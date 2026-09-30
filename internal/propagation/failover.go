package propagation

import (
	"context"
	"sort"
	"time"

	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FailoverEvent records a deployment being moved off an unhealthy cluster.
type FailoverEvent struct {
	From   string    `json:"from"`
	To     string    `json:"to"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
	// CleanupError is set when the deployment could not be removed from the
	// unhealthy cluster, so a copy may still be running there.
	CleanupError string `json:"cleanupError,omitempty"`
}

// StartFailover runs ReconcileFailover on every tick of interval until ctx is
// cancelled. A cluster must stay Unhealthy for gracePeriod before its
// deployments are moved, so brief blips don't cause workloads to flap.
func (e *Engine) StartFailover(ctx context.Context, interval, gracePeriod time.Duration) {
	e.logger.WithFields(logrus.Fields{
		"interval":    interval,
		"gracePeriod": gracePeriod,
	}).Info("Starting failover controller")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				e.logger.Info("Stopping failover controller")
				return
			case <-ticker.C:
				e.ReconcileFailover(ctx, gracePeriod)
			}
		}
	}()
}

// ReconcileFailover moves every propagated deployment off clusters that have
// been Unhealthy for longer than gracePeriod, onto the healthy cluster with
// the most ready nodes that does not already run it. The number of clusters
// a deployment runs on is preserved. It returns the failovers performed.
func (e *Engine) ReconcileFailover(ctx context.Context, gracePeriod time.Duration) []FailoverEvent {
	clusters := make(map[string]*registry.ClusterInfo)
	for _, c := range e.registry.List() {
		clusters[c.Name] = c
	}

	now := time.Now()
	var events []FailoverEvent
	for _, record := range e.List() {
		events = append(events, e.failoverRecord(ctx, record, clusters, now, gracePeriod)...)
	}
	return events
}

// failoverRecord replaces each failed cluster in one record's targets and
// commits the new target list if anything moved.
func (e *Engine) failoverRecord(
	ctx context.Context,
	record Record,
	clusters map[string]*registry.ClusterInfo,
	now time.Time,
	gracePeriod time.Duration,
) []FailoverEvent {
	logger := e.logger.WithField("deployment", record.Namespace+"/"+record.Name)

	targets := append([]string(nil), record.Clusters...)
	results := append([]ClusterResult(nil), record.Results...)
	inUse := make(map[string]bool, len(targets))
	for _, name := range targets {
		inUse[name] = true
	}

	var events []FailoverEvent
	for i, name := range targets {
		failed, ok := clusters[name]
		if !ok || !needsFailover(failed, now, gracePeriod) {
			continue
		}

		replacement := pickReplacement(clusters, inUse)
		if replacement == nil {
			logger.WithField("cluster", name).Warn("Cluster is unhealthy but no healthy cluster is available for failover")
			continue
		}

		result := e.applyToCluster(ctx, record.Deployment, replacement)
		if result.Action == ActionFailed {
			// Leave the target unchanged; the next reconcile will retry.
			logger.WithFields(logrus.Fields{
				"from": name,
				"to":   replacement.Name,
			}).Warn("Failover apply failed; will retry")
			continue
		}

		event := FailoverEvent{
			From:   name,
			To:     replacement.Name,
			At:     time.Now(),
			Reason: "cluster unhealthy since " + failed.UnhealthySince.Format(time.RFC3339),
		}
		if err := e.removeFromCluster(ctx, record, failed); err != nil {
			event.CleanupError = err.Error()
		}

		logger.WithFields(logrus.Fields{
			"from": name,
			"to":   replacement.Name,
		}).Warn("Deployment failed over")

		targets[i] = replacement.Name
		if i < len(results) {
			results[i] = result
		}
		inUse[replacement.Name] = true
		events = append(events, event)
	}

	if len(events) == 0 {
		return nil
	}

	if !e.commitFailover(record, targets, results, events) {
		logger.Warn("Deployment was re-propagated during failover; keeping the newer record")
		return nil
	}
	return events
}

// needsFailover reports whether a cluster has been Unhealthy for longer
// than the grace period.
func needsFailover(c *registry.ClusterInfo, now time.Time, gracePeriod time.Duration) bool {
	return c.Status == registry.StatusUnhealthy &&
		!c.UnhealthySince.IsZero() &&
		now.Sub(c.UnhealthySince) >= gracePeriod
}

// pickReplacement returns the Healthy cluster not in inUse with the most
// ready nodes, breaking ties by name so the choice is deterministic.
func pickReplacement(clusters map[string]*registry.ClusterInfo, inUse map[string]bool) *registry.ClusterInfo {
	var candidates []*registry.ClusterInfo
	for _, c := range clusters {
		if c.Status == registry.StatusHealthy && !inUse[c.Name] {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ReadyNodes != candidates[j].ReadyNodes {
			return candidates[i].ReadyNodes > candidates[j].ReadyNodes
		}
		return candidates[i].Name < candidates[j].Name
	})
	return candidates[0]
}

// removeFromCluster makes a best-effort attempt to delete the deployment from
// a cluster it has failed away from. The cluster is unhealthy, so this often
// fails; the error is returned for the failover record.
func (e *Engine) removeFromCluster(ctx context.Context, record Record, cluster *registry.ClusterInfo) error {
	ctx, cancel := context.WithTimeout(ctx, clusterTimeout)
	defer cancel()

	clientset, err := e.clientFactory(cluster.KubeconfigPath)
	if err != nil {
		return err
	}

	err = clientset.AppsV1().Deployments(record.Namespace).Delete(ctx, record.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		e.logger.WithFields(logrus.Fields{
			"cluster":    cluster.Name,
			"deployment": record.Namespace + "/" + record.Name,
		}).WithError(err).Warn("Could not remove deployment from unhealthy cluster")
		return err
	}
	return nil
}

// commitFailover saves the new targets unless the deployment was
// re-propagated while failover was running, in which case the newer record
// wins. It reports whether the record was updated.
func (e *Engine) commitFailover(snapshot Record, targets []string, results []ClusterResult, events []FailoverEvent) bool {
	e.mu.Lock()
	current, ok := e.records[recordKey(snapshot.Namespace, snapshot.Name)]
	if !ok || !current.PropagatedAt.Equal(snapshot.PropagatedAt) {
		e.mu.Unlock()
		return false
	}

	current.Clusters = targets
	current.Results = results
	current.Failovers = append(current.Failovers, events...)
	e.mu.Unlock()

	e.notifyChange()
	return true
}
