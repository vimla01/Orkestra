package health

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// newTestLogger creates a logrus logger that discards output during tests.
func newTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.DebugLevel)
	return logger
}

// createTempKubeconfig writes a minimal kubeconfig file to a temp directory
// and returns its path.
func createTempKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "kubeconfig")

	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
		t.Fatalf("Failed to write temp kubeconfig: %v", err)
	}
	return kubeconfigPath
}

// buildFakeNodes creates a slice of corev1.Node objects: count nodes where
// readyCount are Ready and the remainder are NotReady.
func buildFakeNodes(readyCount, notReadyCount int) []corev1.Node {
	var nodes []corev1.Node
	for i := 0; i < readyCount; i++ {
		nodes = append(nodes, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("ready-node-%d", i),
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		})
	}
	for i := 0; i < notReadyCount; i++ {
		nodes = append(nodes, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("notready-node-%d", i),
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		})
	}
	return nodes
}

func TestCheckCluster_HealthyNodes(t *testing.T) {
	kubeconfigPath := createTempKubeconfig(t)
	logger := newTestLogger()

	// Build fake nodes: 2 Ready, 1 NotReady.
	nodes := buildFakeNodes(2, 1)

	// Create a fake clientset pre-populated with the nodes.
	objects := make([]interface{}, 0, len(nodes))
	for i := range nodes {
		objects = append(objects, &nodes[i])
	}

	// Use fake.NewSimpleClientset with runtime.Object arguments.
	fakeClientset := fake.NewSimpleClientset(&nodes[0], &nodes[1], &nodes[2])

	// Client factory returns the pre-built fake clientset.
	clientFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
		return fakeClientset, nil
	}

	// Create registry with a fake client factory that supports ServerVersion.
	reg := registry.NewRegistry(clientFactory)

	// Register the cluster directly by manipulating the registry.
	// We use the same clientFactory which will pass the ServerVersion check.
	_, err := reg.Register("test-cluster", kubeconfigPath)
	if err != nil {
		t.Fatalf("Failed to register cluster: %v", err)
	}

	// Create the aggregator and run CheckCluster.
	agg := NewAggregator(reg, clientFactory, 30*time.Second, logger)
	if err := agg.CheckCluster(context.Background(), "test-cluster"); err != nil {
		t.Fatalf("CheckCluster returned error: %v", err)
	}

	// Verify the registry was updated with the correct health info.
	info, err := reg.Get("test-cluster")
	if err != nil {
		t.Fatalf("Failed to get cluster info: %v", err)
	}

	if info.NodeCount != 3 {
		t.Errorf("Expected nodeCount=3, got %d", info.NodeCount)
	}
	if info.ReadyNodes != 2 {
		t.Errorf("Expected readyNodes=2, got %d", info.ReadyNodes)
	}
	if info.Status != "Healthy" {
		t.Errorf("Expected status=Healthy, got %s", info.Status)
	}
	if info.LastHealthCheck.IsZero() {
		t.Error("Expected LastHealthCheck to be set")
	}
}

func TestCheckCluster_ConnectionError(t *testing.T) {
	kubeconfigPath := createTempKubeconfig(t)
	logger := newTestLogger()

	// Create a working factory for registration (needs ServerVersion to pass).
	workingFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(), nil
	}

	// Create registry and register the cluster with the working factory.
	reg := registry.NewRegistry(workingFactory)
	_, err := reg.Register("failing-cluster", kubeconfigPath)
	if err != nil {
		t.Fatalf("Failed to register cluster: %v", err)
	}

	// Create an error-returning factory for the aggregator.
	errorFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
		return nil, fmt.Errorf("connection refused")
	}

	// Create the aggregator with the error factory.
	agg := NewAggregator(reg, errorFactory, 30*time.Second, logger)
	if err := agg.CheckCluster(context.Background(), "failing-cluster"); err != nil {
		t.Fatalf("CheckCluster returned error: %v", err)
	}

	// Verify the registry shows Unhealthy with 0/0 nodes.
	info, err := reg.Get("failing-cluster")
	if err != nil {
		t.Fatalf("Failed to get cluster info: %v", err)
	}

	if info.Status != "Unhealthy" {
		t.Errorf("Expected status=Unhealthy, got %s", info.Status)
	}
	if info.NodeCount != 0 {
		t.Errorf("Expected nodeCount=0, got %d", info.NodeCount)
	}
	if info.ReadyNodes != 0 {
		t.Errorf("Expected readyNodes=0, got %d", info.ReadyNodes)
	}
}

func TestCheckCluster_NotFound(t *testing.T) {
	logger := newTestLogger()

	clientFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(), nil
	}

	reg := registry.NewRegistry(clientFactory)
	agg := NewAggregator(reg, clientFactory, 30*time.Second, logger)

	err := agg.CheckCluster(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent cluster, got nil")
	}
}
