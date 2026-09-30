package registry

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/orkestra/internal/k8s"
	"k8s.io/client-go/kubernetes"
)

// ErrClusterExists is returned by Register when a cluster with the given
// name is already present in the registry.
var ErrClusterExists = errors.New("cluster already registered")

// Registry is a thread-safe in-memory store of registered Kubernetes clusters.
type Registry struct {
	mu            sync.RWMutex
	clusters      map[string]*ClusterInfo
	clientFactory func(string) (kubernetes.Interface, error)

	// onChange, if set, is called after a cluster is registered or
	// deregistered, outside the lock.
	onChange func()
}

// NewRegistry creates a new Registry with the given client factory.
// The clientFactory function receives a kubeconfig path and returns a
// kubernetes.Interface that can communicate with the target cluster.
func NewRegistry(clientFactory func(string) (kubernetes.Interface, error)) *Registry {
	return &Registry{
		clusters:      make(map[string]*ClusterInfo),
		clientFactory: clientFactory,
	}
}

// SetOnChange sets a callback invoked after every registration or
// deregistration, e.g. to persist the registry. Health updates do not
// trigger it. It must be called before the registry is used concurrently.
func (r *Registry) SetOnChange(fn func()) {
	r.onChange = fn
}

// notifyChange invokes the onChange callback if one is set. Callers must
// not hold r.mu, since the callback may read the registry.
func (r *Registry) notifyChange() {
	if r.onChange != nil {
		r.onChange()
	}
}

// Restore loads previously registered clusters without contacting them, so
// the control plane can start even while some clusters are down. Health
// state is reset to Unknown until the next health check. Clusters whose
// name is already registered are skipped.
func (r *Registry) Restore(clusters []*ClusterInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, c := range clusters {
		if _, exists := r.clusters[c.Name]; exists {
			continue
		}
		r.clusters[c.Name] = &ClusterInfo{
			Name:           c.Name,
			KubeconfigPath: c.KubeconfigPath,
			Status:         StatusUnknown,
			RegisteredAt:   c.RegisteredAt,
			Endpoint:       c.Endpoint,
		}
	}
}

// Register adds a new cluster to the registry.
//
// It validates that the kubeconfig file exists, uses the client factory to
// verify basic connectivity (ServerVersion), and extracts the API server
// endpoint from the kubeconfig. The initial status is set to "Unknown".
func (r *Registry) Register(name, kubeconfigPath string) (*ClusterInfo, error) {
	// Check for duplicate registration up front, without holding the lock
	// across the network calls below.
	r.mu.RLock()
	_, exists := r.clusters[name]
	r.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("cluster %q: %w", name, ErrClusterExists)
	}

	// Validate that the kubeconfig file exists on disk.
	if _, err := os.Stat(kubeconfigPath); err != nil {
		return nil, fmt.Errorf("kubeconfig file not found: %w", err)
	}

	// Verify connectivity by building a clientset and calling ServerVersion.
	client, err := r.clientFactory(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for cluster %q: %w", name, err)
	}
	if _, err := client.Discovery().ServerVersion(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %q: %w", name, err)
	}

	// Extract the API server endpoint from the kubeconfig.
	endpoint, err := k8s.GetServerEndpoint(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract endpoint for cluster %q: %w", name, err)
	}

	info := &ClusterInfo{
		Name:           name,
		KubeconfigPath: kubeconfigPath,
		Status:         StatusUnknown,
		RegisteredAt:   time.Now(),
		Endpoint:       endpoint,
	}

	r.mu.Lock()
	if _, exists := r.clusters[name]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("cluster %q: %w", name, ErrClusterExists)
	}
	r.clusters[name] = info
	infoCopy := *info
	r.mu.Unlock()

	r.notifyChange()
	return &infoCopy, nil
}

// Deregister removes a cluster from the registry.
// Returns an error if the cluster is not found.
func (r *Registry) Deregister(name string) error {
	r.mu.Lock()
	if _, exists := r.clusters[name]; !exists {
		r.mu.Unlock()
		return fmt.Errorf("cluster %q not found", name)
	}
	delete(r.clusters, name)
	r.mu.Unlock()

	r.notifyChange()
	return nil
}

// Get returns the ClusterInfo for the named cluster.
// Returns an error if the cluster is not found.
func (r *Registry) Get(name string) (*ClusterInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.clusters[name]
	if !exists {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	infoCopy := *info
	return &infoCopy, nil
}

// List returns a snapshot copy of all registered clusters.
func (r *Registry) List() []*ClusterInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ClusterInfo, 0, len(r.clusters))
	for _, info := range r.clusters {
		infoCopy := *info
		result = append(result, &infoCopy)
	}
	return result
}

// UpdateHealth updates the health status and node counts for a registered
// cluster. The LastHealthCheck timestamp is set to the current time, and
// UnhealthySince records when the cluster entered the Unhealthy state.
func (r *Registry) UpdateHealth(name string, status string, nodeCount, readyNodes int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.clusters[name]
	if !exists {
		return fmt.Errorf("cluster %q not found", name)
	}

	switch {
	case status != StatusUnhealthy:
		info.UnhealthySince = time.Time{}
	case info.Status != StatusUnhealthy:
		info.UnhealthySince = time.Now()
	}

	info.Status = status
	info.NodeCount = nodeCount
	info.ReadyNodes = readyNodes
	info.LastHealthCheck = time.Now()
	return nil
}
