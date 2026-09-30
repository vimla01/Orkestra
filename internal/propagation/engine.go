package propagation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// clusterTimeout bounds how long a call to a single cluster may take, so one
// unresponsive cluster can't stall the rest. It stays below the API server's
// 15s write timeout so a propagation request can still respond in time.
const clusterTimeout = 10 * time.Second

// ManagedByLabel marks resources that Orkestra propagated to member clusters.
const ManagedByLabel = "app.kubernetes.io/managed-by"

// managedByValue is the value of ManagedByLabel on propagated resources.
const managedByValue = "orkestra"

// Action values reported in ClusterResult.
const (
	ActionCreated = "Created"
	ActionUpdated = "Updated"
	ActionFailed  = "Failed"
)

// ClusterResult is the outcome of applying a deployment to one cluster.
type ClusterResult struct {
	Cluster string `json:"cluster"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

// Engine propagates Deployments from the control plane to member clusters
// and remembers each propagation so its status can be queried later.
type Engine struct {
	registry      *registry.Registry
	clientFactory func(string) (kubernetes.Interface, error)
	logger        *logrus.Logger

	mu      sync.RWMutex
	records map[string]*Record // keyed by namespace/name

	// onChange, if set, is called after a record is saved or changed by
	// failover, outside the lock.
	onChange func()
}

// SetOnChange sets a callback invoked after every change to the
// propagation records, e.g. to persist them. It must be called before the
// engine is used concurrently.
func (e *Engine) SetOnChange(fn func()) {
	e.onChange = fn
}

// notifyChange invokes the onChange callback if one is set. Callers must
// not hold e.mu, since the callback may read the records.
func (e *Engine) notifyChange() {
	if e.onChange != nil {
		e.onChange()
	}
}

// Restore loads previously saved propagation records, replacing any with
// the same namespace and name.
func (e *Engine) Restore(records []Record) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, r := range records {
		c := copyRecord(&r)
		e.records[recordKey(c.Namespace, c.Name)] = &c
	}
}

// NewEngine creates a new propagation Engine with the given dependencies.
func NewEngine(
	registry *registry.Registry,
	clientFactory func(string) (kubernetes.Interface, error),
	logger *logrus.Logger,
) *Engine {
	return &Engine{
		registry:      registry,
		clientFactory: clientFactory,
		logger:        logger,
		records:       make(map[string]*Record),
	}
}

// Propagate applies the deployment to every named cluster in parallel,
// creating it where it does not exist and updating it where it does.
//
// All clusters are validated against the registry before anything is
// applied, so an unknown cluster name fails the whole request rather than
// leaving a partial rollout. Once applying starts, a failure on one cluster
// does not stop the others; per-cluster outcomes are returned in the same
// order as clusterNames, and the propagation is recorded (replacing any
// earlier record for the same deployment).
func (e *Engine) Propagate(ctx context.Context, deployment *appsv1.Deployment, clusterNames []string) ([]ClusterResult, error) {
	if len(clusterNames) == 0 {
		return nil, fmt.Errorf("at least one target cluster is required")
	}

	clusters := make([]*registry.ClusterInfo, len(clusterNames))
	seen := make(map[string]bool, len(clusterNames))
	for i, name := range clusterNames {
		if seen[name] {
			return nil, fmt.Errorf("cluster %q listed more than once", name)
		}
		seen[name] = true

		cluster, err := e.registry.Get(name)
		if err != nil {
			return nil, err
		}
		clusters[i] = cluster
	}

	results := make([]ClusterResult, len(clusters))
	var wg sync.WaitGroup
	for i, cluster := range clusters {
		wg.Add(1)
		go func(i int, cluster *registry.ClusterInfo) {
			defer wg.Done()
			results[i] = e.applyToCluster(ctx, deployment, cluster)
		}(i, cluster)
	}
	wg.Wait()

	e.saveRecord(deployment, clusterNames, results)
	return results, nil
}

// applyToCluster creates or updates the deployment on a single cluster.
func (e *Engine) applyToCluster(ctx context.Context, deployment *appsv1.Deployment, cluster *registry.ClusterInfo) ClusterResult {
	logger := e.logger.WithFields(logrus.Fields{
		"cluster":    cluster.Name,
		"deployment": deployment.Namespace + "/" + deployment.Name,
	})

	ctx, cancel := context.WithTimeout(ctx, clusterTimeout)
	defer cancel()

	result := ClusterResult{Cluster: cluster.Name}

	clientset, err := e.clientFactory(cluster.KubeconfigPath)
	if err != nil {
		logger.WithError(err).Error("Failed to create client for cluster")
		result.Action = ActionFailed
		result.Error = err.Error()
		return result
	}

	action, err := applyDeployment(ctx, clientset, deployment)
	if err != nil {
		logger.WithError(err).Error("Failed to apply deployment")
		result.Action = ActionFailed
		result.Error = err.Error()
		return result
	}

	logger.WithField("action", action).Info("Deployment propagated")
	result.Action = action
	return result
}

// applyDeployment creates the deployment if it is absent, otherwise updates
// the existing object in place. It works on a copy so the caller's object is
// never mutated.
func applyDeployment(ctx context.Context, clientset kubernetes.Interface, deployment *appsv1.Deployment) (string, error) {
	desired := deployment.DeepCopy()
	if desired.Labels == nil {
		desired.Labels = map[string]string{}
	}
	desired.Labels[ManagedByLabel] = managedByValue

	deployments := clientset.AppsV1().Deployments(desired.Namespace)

	existing, err := deployments.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := deployments.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return "", fmt.Errorf("create failed: %w", err)
		}
		return ActionCreated, nil
	}
	if err != nil {
		return "", fmt.Errorf("get failed: %w", err)
	}

	desired.ResourceVersion = existing.ResourceVersion
	if _, err := deployments.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("update failed: %w", err)
	}
	return ActionUpdated, nil
}
