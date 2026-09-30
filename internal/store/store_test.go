package store

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
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
    token: test-token
`

const testManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginx:1.25
`

// newControlPlane returns a fresh registry and engine backed by one shared
// fake cluster client.
func newControlPlane() (*registry.Registry, *propagation.Engine) {
	cs := fake.NewSimpleClientset()
	factory := func(string) (kubernetes.Interface, error) { return cs, nil }
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	reg := registry.NewRegistry(factory)
	return reg, propagation.NewEngine(reg, factory, logger)
}

func writeKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	return path
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	s := NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	state, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(state.Clusters) != 0 || len(state.Deployments) != 0 {
		t.Errorf("expected empty state, got %+v", state)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	tests := map[string]string{
		"corrupt":         "{not json",
		"future version":  `{"version": 99}`,
		"missing payload": `{"version": 1, "deployments": [{"namespace": "default", "name": "web"}]}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewFileStore(path).Load(); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestPersistAndRestoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	s := NewFileStore(path)
	kubeconfig := writeKubeconfig(t)

	// First "run": register, propagate, save.
	reg, engine := newControlPlane()
	if _, err := reg.Register("cluster-a", kubeconfig); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := reg.UpdateHealth("cluster-a", registry.StatusHealthy, 2, 2); err != nil {
		t.Fatal(err)
	}
	deployment, err := propagation.ParseDeployment([]byte(testManifest))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Propagate(context.Background(), deployment, []string{"cluster-a"}); err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}
	if err := s.Persist(reg, engine); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Second "run": load into an empty control plane.
	state, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	reg2, engine2 := newControlPlane()
	Restore(state, reg2, engine2)

	cluster, err := reg2.Get("cluster-a")
	if err != nil {
		t.Fatalf("cluster not restored: %v", err)
	}
	if cluster.KubeconfigPath != kubeconfig || cluster.Endpoint != "https://localhost:6443" {
		t.Errorf("cluster fields not restored: %+v", cluster)
	}
	if cluster.Status != registry.StatusUnknown || cluster.ReadyNodes != 0 {
		t.Errorf("health should reset to Unknown after restore, got %+v", cluster)
	}

	record, err := engine2.Get("default", "web")
	if err != nil {
		t.Fatalf("deployment not restored: %v", err)
	}
	if len(record.Clusters) != 1 || record.Clusters[0] != "cluster-a" {
		t.Errorf("targets not restored: %v", record.Clusters)
	}
	if record.Deployment == nil || *record.Deployment.Spec.Replicas != 3 {
		t.Error("manifest not restored")
	}
}

func TestPersistLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(filepath.Join(dir, "state.json"))
	reg, engine := newControlPlane()

	for i := 0; i < 3; i++ {
		if err := s.Persist(reg, engine); err != nil {
			t.Fatalf("Persist failed: %v", err)
		}
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only state.json, got %v", names)
	}
}

func TestChangeHooksPersistAutomatically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewFileStore(path)
	kubeconfig := writeKubeconfig(t)

	reg, engine := newControlPlane()
	persist := func() {
		if err := s.Persist(reg, engine); err != nil {
			t.Errorf("Persist failed: %v", err)
		}
	}
	reg.SetOnChange(persist)
	engine.SetOnChange(persist)

	if _, err := reg.Register("cluster-a", kubeconfig); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Load()
	if len(state.Clusters) != 1 {
		t.Fatalf("expected registration to be persisted, got %+v", state.Clusters)
	}

	deployment, _ := propagation.ParseDeployment([]byte(testManifest))
	if _, err := engine.Propagate(context.Background(), deployment, []string{"cluster-a"}); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Load()
	if len(state.Deployments) != 1 {
		t.Fatalf("expected propagation to be persisted, got %+v", state.Deployments)
	}

	if err := reg.Deregister("cluster-a"); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Load()
	if len(state.Clusters) != 0 {
		t.Errorf("expected deregistration to be persisted, got %+v", state.Clusters)
	}
}
