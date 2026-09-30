package propagation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/orkestra/internal/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// setHealth records a health check result for a cluster in the registry.
func setHealth(t *testing.T, env *testEnv, cluster, status string, readyNodes int) {
	t.Helper()
	if err := env.registry.UpdateHealth(cluster, status, readyNodes, readyNodes); err != nil {
		t.Fatalf("UpdateHealth(%s) failed: %v", cluster, err)
	}
}

// hasDeployment reports whether nginx exists on the named fake cluster.
func hasDeployment(env *testEnv, cluster string) bool {
	_, err := env.clients[cluster].AppsV1().Deployments("default").Get(context.Background(), "nginx", metav1.GetOptions{})
	return err == nil
}

// propagateTo propagates nginx to the given clusters or fails the test.
func propagateTo(t *testing.T, env *testEnv, clusters ...string) {
	t.Helper()
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), clusters); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}
}

func TestFailoverMovesToBestHealthyCluster(t *testing.T) {
	env := newTestEnv(t, "a", "b", "small", "big")
	propagateTo(t, env, "a", "b")

	setHealth(t, env, "a", registry.StatusUnhealthy, 0)
	setHealth(t, env, "b", registry.StatusHealthy, 3)
	setHealth(t, env, "small", registry.StatusHealthy, 1)
	setHealth(t, env, "big", registry.StatusHealthy, 5)

	events := env.engine.ReconcileFailover(context.Background(), 0)
	if len(events) != 1 || events[0].From != "a" || events[0].To != "big" {
		t.Fatalf("expected one failover a -> big, got %+v", events)
	}
	if events[0].CleanupError != "" {
		t.Errorf("unexpected cleanup error: %s", events[0].CleanupError)
	}

	if !hasDeployment(env, "big") {
		t.Error("expected deployment on the replacement cluster")
	}
	if hasDeployment(env, "a") {
		t.Error("expected deployment removed from the unhealthy cluster")
	}

	record, _ := env.engine.Get("default", "nginx")
	if record.Clusters[0] != "big" || record.Clusters[1] != "b" {
		t.Errorf("expected targets [big b], got %v", record.Clusters)
	}
	if record.Results[0].Cluster != "big" || record.Results[0].Action != ActionCreated {
		t.Errorf("expected result for big to be Created, got %+v", record.Results[0])
	}
	if len(record.Failovers) != 1 {
		t.Errorf("expected failover history on the record, got %+v", record.Failovers)
	}
}

func TestFailoverWaitsForGracePeriod(t *testing.T) {
	env := newTestEnv(t, "a", "b")
	propagateTo(t, env, "a")

	setHealth(t, env, "a", registry.StatusUnhealthy, 0)
	setHealth(t, env, "b", registry.StatusHealthy, 1)

	if events := env.engine.ReconcileFailover(context.Background(), time.Hour); len(events) != 0 {
		t.Errorf("expected no failover within grace period, got %+v", events)
	}
	if hasDeployment(env, "b") {
		t.Error("deployment should not have moved yet")
	}
}

func TestFailoverSkipsUnknownAndUnhealthyCandidates(t *testing.T) {
	env := newTestEnv(t, "a", "unknown", "down")
	propagateTo(t, env, "a")

	setHealth(t, env, "a", registry.StatusUnhealthy, 0)
	setHealth(t, env, "down", registry.StatusUnhealthy, 0)
	// "unknown" has never been health-checked.

	if events := env.engine.ReconcileFailover(context.Background(), 0); len(events) != 0 {
		t.Errorf("expected no failover without a healthy candidate, got %+v", events)
	}

	record, _ := env.engine.Get("default", "nginx")
	if record.Clusters[0] != "a" {
		t.Errorf("targets should be unchanged, got %v", record.Clusters)
	}
}

func TestFailoverRetriesWhenReplacementApplyFails(t *testing.T) {
	env := newTestEnv(t, "a", "b")
	propagateTo(t, env, "a")

	setHealth(t, env, "a", registry.StatusUnhealthy, 0)
	setHealth(t, env, "b", registry.StatusHealthy, 1)
	env.clients["b"].PrependReactor("create", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("quota exceeded")
	})

	if events := env.engine.ReconcileFailover(context.Background(), 0); len(events) != 0 {
		t.Errorf("expected no failover when apply fails, got %+v", events)
	}
	if !hasDeployment(env, "a") {
		t.Error("deployment must not be removed from the old cluster if the move failed")
	}

	record, _ := env.engine.Get("default", "nginx")
	if record.Clusters[0] != "a" {
		t.Errorf("targets should be unchanged, got %v", record.Clusters)
	}
}

func TestFailoverRecordsCleanupError(t *testing.T) {
	env := newTestEnv(t, "a", "b")
	propagateTo(t, env, "a")

	setHealth(t, env, "a", registry.StatusUnhealthy, 0)
	setHealth(t, env, "b", registry.StatusHealthy, 1)
	env.clients["a"].PrependReactor("delete", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	events := env.engine.ReconcileFailover(context.Background(), 0)
	if len(events) != 1 || events[0].To != "b" {
		t.Fatalf("expected failover to b despite cleanup failure, got %+v", events)
	}
	if events[0].CleanupError == "" {
		t.Error("expected cleanup error to be recorded")
	}
}

func TestFailoverDoesNotOverwriteNewerPropagation(t *testing.T) {
	env := newTestEnv(t, "a", "b")
	propagateTo(t, env, "a")
	stale, _ := env.engine.Get("default", "nginx")

	// The deployment is re-propagated while a failover is in flight.
	propagateTo(t, env, "a", "b")

	if env.engine.commitFailover(stale, []string{"b"}, nil, []FailoverEvent{{From: "a", To: "b"}}) {
		t.Fatal("expected commit to be rejected for a stale record")
	}

	record, _ := env.engine.Get("default", "nginx")
	if len(record.Clusters) != 2 || len(record.Failovers) != 0 {
		t.Errorf("newer record was modified: %+v", record)
	}
}
