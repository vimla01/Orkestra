// Package store persists control plane state (registered clusters and
// propagation records) to a JSON file so it survives restarts.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
	appsv1 "k8s.io/api/apps/v1"
)

// stateVersion is the on-disk format version, bumped on incompatible changes.
const stateVersion = 1

// State is the persisted form of the control plane.
type State struct {
	Version     int               `json:"version"`
	Clusters    []ClusterState    `json:"clusters"`
	Deployments []DeploymentState `json:"deployments"`
}

// ClusterState is a registered cluster as saved to disk. Health fields are
// deliberately left out: they are stale after a restart and re-checked.
type ClusterState struct {
	Name           string    `json:"name"`
	KubeconfigPath string    `json:"kubeconfigPath"`
	Endpoint       string    `json:"endpoint"`
	RegisteredAt   time.Time `json:"registeredAt"`
}

// DeploymentState is a propagation record as saved to disk, including the
// applied manifest so failover can re-propagate it after a restart.
type DeploymentState struct {
	Namespace    string                      `json:"namespace"`
	Name         string                      `json:"name"`
	Clusters     []string                    `json:"clusters"`
	Results      []propagation.ClusterResult `json:"results"`
	PropagatedAt time.Time                   `json:"propagatedAt"`
	Failovers    []propagation.FailoverEvent `json:"failovers,omitempty"`
	Deployment   *appsv1.Deployment          `json:"deployment"`
}

// FileStore reads and writes State as a JSON file.
type FileStore struct {
	path string
	mu   sync.Mutex // serializes snapshot+write so the newest state lands last
}

// NewFileStore creates a FileStore backed by the file at path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load reads the saved state. A missing file is not an error: it returns an
// empty state, as on first start.
func (s *FileStore) Load() (*State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{Version: stateVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %q: %w", s.path, err)
	}
	if state.Version != stateVersion {
		return nil, fmt.Errorf("state file %q has unsupported version %d (want %d)", s.path, state.Version, stateVersion)
	}
	for _, d := range state.Deployments {
		if d.Deployment == nil {
			return nil, fmt.Errorf("state file %q: deployment %s/%s has no manifest", s.path, d.Namespace, d.Name)
		}
	}
	return &state, nil
}

// Persist snapshots the registry and engine and writes them to disk.
// Snapshotting under the store's lock guarantees that when saves race, the
// last write holds the latest state.
func (s *FileStore) Persist(reg *registry.Registry, engine *propagation.Engine) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(Snapshot(reg, engine))
}

// write saves state atomically: it writes a temp file in the same directory,
// syncs it, and renames it over the target, so a crash mid-write never
// leaves a truncated state file.
func (s *FileStore) write(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close state file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("failed to replace state file: %w", err)
	}
	return nil
}

// Snapshot captures the current registry and engine contents as a State.
func Snapshot(reg *registry.Registry, engine *propagation.Engine) *State {
	state := &State{
		Version:     stateVersion,
		Clusters:    []ClusterState{},
		Deployments: []DeploymentState{},
	}

	for _, c := range reg.List() {
		state.Clusters = append(state.Clusters, ClusterState{
			Name:           c.Name,
			KubeconfigPath: c.KubeconfigPath,
			Endpoint:       c.Endpoint,
			RegisteredAt:   c.RegisteredAt,
		})
	}

	for _, r := range engine.List() {
		state.Deployments = append(state.Deployments, DeploymentState{
			Namespace:    r.Namespace,
			Name:         r.Name,
			Clusters:     r.Clusters,
			Results:      r.Results,
			PropagatedAt: r.PropagatedAt,
			Failovers:    r.Failovers,
			Deployment:   r.Deployment,
		})
	}

	return state
}

// Restore loads a saved State into the registry and engine.
func Restore(state *State, reg *registry.Registry, engine *propagation.Engine) {
	clusters := make([]*registry.ClusterInfo, 0, len(state.Clusters))
	for _, c := range state.Clusters {
		clusters = append(clusters, &registry.ClusterInfo{
			Name:           c.Name,
			KubeconfigPath: c.KubeconfigPath,
			Endpoint:       c.Endpoint,
			RegisteredAt:   c.RegisteredAt,
		})
	}
	reg.Restore(clusters)

	records := make([]propagation.Record, 0, len(state.Deployments))
	for _, d := range state.Deployments {
		records = append(records, propagation.Record{
			Namespace:    d.Namespace,
			Name:         d.Name,
			Clusters:     d.Clusters,
			Results:      d.Results,
			PropagatedAt: d.PropagatedAt,
			Failovers:    d.Failovers,
			Deployment:   d.Deployment,
		})
	}
	engine.Restore(records)
}
