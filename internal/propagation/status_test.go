package propagation

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setRolloutStatus writes replica counts into the deployment's status on a
// fake cluster, standing in for the real deployment controller.
func setRolloutStatus(t *testing.T, env *testEnv, cluster string, updated, available int32) {
	t.Helper()
	deployments := env.clients[cluster].AppsV1().Deployments("default")
	d, err := deployments.Get(context.Background(), "nginx", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get on %s failed: %v", cluster, err)
	}
	d.Status.UpdatedReplicas = updated
	d.Status.AvailableReplicas = available
	d.Status.ReadyReplicas = available
	if _, err := deployments.UpdateStatus(context.Background(), d, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("status update on %s failed: %v", cluster, err)
	}
}

func TestPropagateRecordsResult(t *testing.T) {
	env := newTestEnv(t, "cluster-a", "cluster-b")
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a", "cluster-b"}); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	record, err := env.engine.Get("default", "nginx")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(record.Clusters) != 2 || len(record.Results) != 2 {
		t.Errorf("expected 2 clusters and results, got %+v", record)
	}
	if record.Deployment == nil || record.Deployment.Name != "nginx" {
		t.Error("expected record to keep the applied deployment")
	}

	if list := env.engine.List(); len(list) != 1 {
		t.Errorf("expected 1 record, got %d", len(list))
	}
}

func TestGetUnknownDeployment(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.engine.Get("default", "missing"); !errors.Is(err, ErrDeploymentNotFound) {
		t.Errorf("expected ErrDeploymentNotFound, got %v", err)
	}
	if _, err := env.engine.Status(context.Background(), "default", "missing"); !errors.Is(err, ErrDeploymentNotFound) {
		t.Errorf("expected ErrDeploymentNotFound from Status, got %v", err)
	}
}

func TestRecordIsSnapshot(t *testing.T) {
	env := newTestEnv(t, "cluster-a")
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a"}); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	record, _ := env.engine.Get("default", "nginx")
	record.Clusters[0] = "tampered"
	record.Deployment.Name = "tampered"

	fresh, _ := env.engine.Get("default", "nginx")
	if fresh.Clusters[0] != "cluster-a" || fresh.Deployment.Name != "nginx" {
		t.Error("mutating a returned record changed the engine's copy")
	}
}

func TestStatusReadyOnlyWhenAllClustersRolledOut(t *testing.T) {
	env := newTestEnv(t, "cluster-a", "cluster-b")
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a", "cluster-b"}); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	setRolloutStatus(t, env, "cluster-a", 2, 2)
	setRolloutStatus(t, env, "cluster-b", 2, 1)

	status, err := env.engine.Status(context.Background(), "default", "nginx")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Ready {
		t.Error("expected not ready while cluster-b is still rolling out")
	}
	if !status.Statuses[0].Ready || status.Statuses[1].Ready {
		t.Errorf("unexpected per-cluster readiness: %+v", status.Statuses)
	}
	if status.Statuses[1].DesiredReplicas != 2 || status.Statuses[1].AvailableReplicas != 1 {
		t.Errorf("unexpected replica counts: %+v", status.Statuses[1])
	}

	setRolloutStatus(t, env, "cluster-b", 2, 2)
	status, _ = env.engine.Status(context.Background(), "default", "nginx")
	if !status.Ready {
		t.Errorf("expected ready once every cluster rolled out: %+v", status.Statuses)
	}
}

func TestStatusReportsMissingDeployment(t *testing.T) {
	env := newTestEnv(t, "cluster-a")
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a"}); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	// Someone deleted it directly on the member cluster.
	if err := env.clients["cluster-a"].AppsV1().Deployments("default").Delete(context.Background(), "nginx", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	status, err := env.engine.Status(context.Background(), "default", "nginx")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Ready || status.Statuses[0].Error == "" {
		t.Errorf("expected an error for the missing deployment, got %+v", status.Statuses[0])
	}
}
