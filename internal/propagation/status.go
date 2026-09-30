package propagation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrDeploymentNotFound is returned when no propagation has been recorded
// for the requested deployment.
var ErrDeploymentNotFound = errors.New("deployment not found")

// Record is the control plane's memory of the last propagation of a
// deployment: what was applied, where, and how each cluster responded.
type Record struct {
	Namespace    string          `json:"namespace"`
	Name         string          `json:"name"`
	Clusters     []string        `json:"clusters"`
	Results      []ClusterResult `json:"results"`
	PropagatedAt time.Time       `json:"propagatedAt"`

	// Failovers lists every time the deployment was moved off an unhealthy
	// cluster since it was last propagated.
	Failovers []FailoverEvent `json:"failovers,omitempty"`

	// Deployment is the manifest that was applied. It is kept so the
	// deployment can be re-propagated later; it is omitted from API output.
	Deployment *appsv1.Deployment `json:"-"`
}

// ClusterStatus is the live rollout state of a deployment on one cluster.
type ClusterStatus struct {
	Cluster           string `json:"cluster"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	ReadyReplicas     int32  `json:"readyReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	UpdatedReplicas   int32  `json:"updatedReplicas"`
	Ready             bool   `json:"ready"`
	Error             string `json:"error,omitempty"`
}

// DeploymentStatus combines a propagation record with live per-cluster state.
type DeploymentStatus struct {
	Record
	Ready    bool            `json:"ready"`
	Statuses []ClusterStatus `json:"statuses"`
}

// recordKey builds the lookup key for a deployment's record.
func recordKey(namespace, name string) string {
	return namespace + "/" + name
}

// saveRecord stores the outcome of a propagation, replacing any earlier one.
func (e *Engine) saveRecord(deployment *appsv1.Deployment, clusterNames []string, results []ClusterResult) {
	record := &Record{
		Namespace:    deployment.Namespace,
		Name:         deployment.Name,
		Clusters:     append([]string(nil), clusterNames...),
		Results:      results,
		PropagatedAt: time.Now(),
		Deployment:   deployment.DeepCopy(),
	}

	e.mu.Lock()
	e.records[recordKey(record.Namespace, record.Name)] = record
	e.mu.Unlock()

	e.notifyChange()
}

// copyRecord returns a copy of r whose slices and deployment are not shared.
func copyRecord(r *Record) Record {
	c := *r
	c.Clusters = append([]string(nil), r.Clusters...)
	c.Results = append([]ClusterResult(nil), r.Results...)
	c.Failovers = append([]FailoverEvent(nil), r.Failovers...)
	c.Deployment = r.Deployment.DeepCopy()
	return c
}

// List returns a snapshot of all propagation records, sorted by namespace
// and name.
func (e *Engine) List() []Record {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Record, 0, len(e.records))
	for _, r := range e.records {
		result = append(result, copyRecord(r))
	}
	sort.Slice(result, func(i, j int) bool {
		return recordKey(result[i].Namespace, result[i].Name) < recordKey(result[j].Namespace, result[j].Name)
	})
	return result
}

// Get returns a snapshot of the propagation record for a deployment.
func (e *Engine) Get(namespace, name string) (Record, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	r, ok := e.records[recordKey(namespace, name)]
	if !ok {
		return Record{}, fmt.Errorf("%s/%s: %w", namespace, name, ErrDeploymentNotFound)
	}
	return copyRecord(r), nil
}

// Status queries every target cluster in parallel for the live rollout state
// of a propagated deployment. The deployment is Ready only when it is fully
// rolled out on every target cluster.
func (e *Engine) Status(ctx context.Context, namespace, name string) (*DeploymentStatus, error) {
	record, err := e.Get(namespace, name)
	if err != nil {
		return nil, err
	}

	statuses := make([]ClusterStatus, len(record.Clusters))
	var wg sync.WaitGroup
	for i, clusterName := range record.Clusters {
		wg.Add(1)
		go func(i int, clusterName string) {
			defer wg.Done()
			statuses[i] = e.clusterStatus(ctx, clusterName, namespace, name)
		}(i, clusterName)
	}
	wg.Wait()

	ready := true
	for _, s := range statuses {
		if !s.Ready {
			ready = false
			break
		}
	}

	return &DeploymentStatus{Record: record, Ready: ready, Statuses: statuses}, nil
}

// clusterStatus reads the deployment's rollout state from a single cluster.
func (e *Engine) clusterStatus(ctx context.Context, clusterName, namespace, name string) ClusterStatus {
	status := ClusterStatus{Cluster: clusterName}

	ctx, cancel := context.WithTimeout(ctx, clusterTimeout)
	defer cancel()

	cluster, err := e.registry.Get(clusterName)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	clientset, err := e.clientFactory(cluster.KubeconfigPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.DesiredReplicas = 1
	if deployment.Spec.Replicas != nil {
		status.DesiredReplicas = *deployment.Spec.Replicas
	}
	status.ReadyReplicas = deployment.Status.ReadyReplicas
	status.AvailableReplicas = deployment.Status.AvailableReplicas
	status.UpdatedReplicas = deployment.Status.UpdatedReplicas
	status.Ready = isRolledOut(deployment, status.DesiredReplicas)
	return status
}

// isRolledOut reports whether the deployment controller has observed the
// latest spec and every desired replica is updated and available.
func isRolledOut(d *appsv1.Deployment, desired int32) bool {
	return d.Status.ObservedGeneration >= d.Generation &&
		d.Status.UpdatedReplicas == desired &&
		d.Status.AvailableReplicas == desired
}
