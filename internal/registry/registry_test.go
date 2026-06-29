package registry

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// minimalKubeconfig is a valid kubeconfig YAML that points at localhost:6443.
const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
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

// writeTempKubeconfig writes a minimal kubeconfig file into a temporary
// directory and returns the file path.
func writeTempKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(minimalKubeconfig), 0600); err != nil {
		t.Fatalf("failed to write temp kubeconfig: %v", err)
	}
	return path
}

// mockClientFactory returns a fake clientset regardless of the kubeconfig path.
func mockClientFactory(_ string) (kubernetes.Interface, error) {
	return fake.NewSimpleClientset(), nil
}

func TestRegister(t *testing.T) {
	r := NewRegistry(mockClientFactory)
	kubeconfig := writeTempKubeconfig(t)

	info, err := r.Register("cluster-a", kubeconfig)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if info.Name != "cluster-a" {
		t.Errorf("expected name %q, got %q", "cluster-a", info.Name)
	}
	if info.Status != "Unknown" {
		t.Errorf("expected status %q, got %q", "Unknown", info.Status)
	}
	if info.Endpoint != "https://localhost:6443" {
		t.Errorf("expected endpoint %q, got %q", "https://localhost:6443", info.Endpoint)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry(mockClientFactory)
	kubeconfig := writeTempKubeconfig(t)

	if _, err := r.Register("dup", kubeconfig); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	_, err := r.Register("dup", kubeconfig)
	if err == nil {
		t.Fatal("expected error when registering duplicate cluster, got nil")
	}
}

func TestDeregister(t *testing.T) {
	r := NewRegistry(mockClientFactory)
	kubeconfig := writeTempKubeconfig(t)

	if _, err := r.Register("to-remove", kubeconfig); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := r.Deregister("to-remove"); err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}

	_, err := r.Get("to-remove")
	if err == nil {
		t.Fatal("expected error after deregistering cluster, got nil")
	}
}

func TestDeregisterNotFound(t *testing.T) {
	r := NewRegistry(mockClientFactory)

	err := r.Deregister("nonexistent")
	if err == nil {
		t.Fatal("expected error when deregistering non-existent cluster, got nil")
	}
}

func TestGet(t *testing.T) {
	r := NewRegistry(mockClientFactory)
	kubeconfig := writeTempKubeconfig(t)

	if _, err := r.Register("get-me", kubeconfig); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	info, err := r.Get("get-me")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if info.Name != "get-me" {
		t.Errorf("expected name %q, got %q", "get-me", info.Name)
	}
	if info.KubeconfigPath != kubeconfig {
		t.Errorf("expected kubeconfigPath %q, got %q", kubeconfig, info.KubeconfigPath)
	}
	if info.Endpoint != "https://localhost:6443" {
		t.Errorf("expected endpoint %q, got %q", "https://localhost:6443", info.Endpoint)
	}
	if info.RegisteredAt.IsZero() {
		t.Error("expected RegisteredAt to be set")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry(mockClientFactory)

	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("expected error when getting non-existent cluster, got nil")
	}
}

func TestList(t *testing.T) {
	r := NewRegistry(mockClientFactory)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		kubeconfig := writeTempKubeconfig(t)
		if _, err := r.Register(name, kubeconfig); err != nil {
			t.Fatalf("Register(%q) failed: %v", name, err)
		}
	}

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(list))
	}

	// Verify all names are present.
	found := make(map[string]bool)
	for _, info := range list {
		found[info.Name] = true
	}
	for _, name := range names {
		if !found[name] {
			t.Errorf("cluster %q not found in list", name)
		}
	}
}

func TestUpdateHealth(t *testing.T) {
	r := NewRegistry(mockClientFactory)
	kubeconfig := writeTempKubeconfig(t)

	if _, err := r.Register("health-test", kubeconfig); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := r.UpdateHealth("health-test", "Healthy", 5, 4); err != nil {
		t.Fatalf("UpdateHealth failed: %v", err)
	}

	info, err := r.Get("health-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if info.Status != "Healthy" {
		t.Errorf("expected status %q, got %q", "Healthy", info.Status)
	}
	if info.NodeCount != 5 {
		t.Errorf("expected nodeCount 5, got %d", info.NodeCount)
	}
	if info.ReadyNodes != 4 {
		t.Errorf("expected readyNodes 4, got %d", info.ReadyNodes)
	}
	if info.LastHealthCheck.IsZero() {
		t.Error("expected LastHealthCheck to be set")
	}
}
