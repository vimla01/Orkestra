package health

import (
	"context"
	"fmt"
	"time"

	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Aggregator continuously monitors the health of registered Kubernetes clusters
// by polling node status and updating the registry with health information.
type Aggregator struct {
	registry      *registry.Registry
	clientFactory func(string) (kubernetes.Interface, error)
	interval      time.Duration
	logger        *logrus.Logger
}

// NewAggregator creates a new Aggregator with the given dependencies.
func NewAggregator(
	registry *registry.Registry,
	clientFactory func(string) (kubernetes.Interface, error),
	interval time.Duration,
	logger *logrus.Logger,
) *Aggregator {
	return &Aggregator{
		registry:      registry,
		clientFactory: clientFactory,
		interval:      interval,
		logger:        logger,
	}
}

// Start begins the periodic health polling loop. It runs in a goroutine and
// polls all registered clusters on each tick of the configured interval.
// The loop stops when the context is cancelled.
func (a *Aggregator) Start(ctx context.Context) {
	a.logger.WithField("interval", a.interval).Info("Starting health aggregator")

	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				a.logger.Info("Stopping health aggregator")
				return
			case <-ticker.C:
				a.pollAll(ctx)
			}
		}
	}()
}

// pollAll iterates over all registered clusters and checks their health.
func (a *Aggregator) pollAll(ctx context.Context) {
	clusters := a.registry.List()
	for _, cluster := range clusters {
		a.checkCluster(ctx, cluster)
	}
}

// CheckCluster performs an on-demand health check for the named cluster.
// It retrieves the cluster from the registry, builds a clientset, lists nodes,
// counts total and Ready nodes, and updates the registry with the result.
func (a *Aggregator) CheckCluster(ctx context.Context, name string) error {
	cluster, err := a.registry.Get(name)
	if err != nil {
		return fmt.Errorf("cluster %q not found in registry: %w", name, err)
	}

	a.checkCluster(ctx, cluster)
	return nil
}

// checkCluster performs the actual health check for a single cluster.
func (a *Aggregator) checkCluster(ctx context.Context, cluster *registry.ClusterInfo) {
	logger := a.logger.WithField("cluster", cluster.Name)

	clientset, err := a.clientFactory(cluster.KubeconfigPath)
	if err != nil {
		logger.WithError(err).Error("Failed to create client for cluster")
		if updateErr := a.registry.UpdateHealth(cluster.Name, "Unhealthy", 0, 0); updateErr != nil {
			logger.WithError(updateErr).Error("Failed to update health status")
		}
		return
	}

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.WithError(err).Error("Failed to list nodes for cluster")
		if updateErr := a.registry.UpdateHealth(cluster.Name, "Unhealthy", 0, 0); updateErr != nil {
			logger.WithError(updateErr).Error("Failed to update health status")
		}
		return
	}

	totalNodes := len(nodeList.Items)
	readyNodes := countReadyNodes(nodeList.Items)

	status := "Healthy"
	if readyNodes == 0 {
		status = "Unhealthy"
	}

	if err := a.registry.UpdateHealth(cluster.Name, status, totalNodes, readyNodes); err != nil {
		logger.WithError(err).Error("Failed to update health status")
		return
	}

	logger.WithFields(logrus.Fields{
		"totalNodes": totalNodes,
		"readyNodes": readyNodes,
		"status":     status,
	}).Debug("Health check completed")
}

// countReadyNodes counts the number of nodes that have the Ready condition
// set to True.
func countReadyNodes(nodes []corev1.Node) int {
	ready := 0
	for _, node := range nodes {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready++
				break
			}
		}
	}
	return ready
}
