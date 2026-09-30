package propagation

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const minimalKubeconfig = `apiVersion: v1
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
    token: fake-token
`

const nginxManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
`

// testEnv wires a registry and engine to one fake clientset per cluster.
type testEnv struct {
	registry *registry.Registry
	engine   *Engine
	clients  map[string]*fake.Clientset // keyed by cluster name
}

// newTestEnv registers the named clusters, each backed by its own fake
// clientset, and returns an engine that talks to them.
func newTestEnv(t *testing.T, clusterNames ...string) *testEnv {
	t.Helper()

	env := &testEnv{clients: map[string]*fake.Clientset{}}
	byPath := map[string]*fake.Clientset{}
	factory := func(path string) (kubernetes.Interface, error) {
		if c, ok := byPath[path]; ok {
			return c, nil
		}
		return nil, errors.New("unknown kubeconfig " + path)
	}

	env.registry = registry.NewRegistry(factory)
	dir := t.TempDir()
	for _, name := range clusterNames {
		path := filepath.Join(dir, name+".kubeconfig")
		if err := os.WriteFile(path, []byte(minimalKubeconfig), 0600); err != nil {
			t.Fatalf("failed to write kubeconfig: %v", err)
		}
		client := fake.NewSimpleClientset()
		byPath[path] = client
		env.clients[name] = client
		if _, err := env.registry.Register(name, path); err != nil {
			t.Fatalf("failed to register %s: %v", name, err)
		}
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	env.engine = NewEngine(env.registry, factory, logger)
	return env
}

func mustParse(t *testing.T) *appsv1.Deployment {
	t.Helper()
	d, err := ParseDeployment([]byte(nginxManifest))
	if err != nil {
		t.Fatalf("ParseDeployment failed: %v", err)
	}
	return d
}

func TestParseDeployment(t *testing.T) {
	d := mustParse(t)
	if d.Name != "nginx" {
		t.Errorf("expected name nginx, got %q", d.Name)
	}
	if d.Namespace != "default" {
		t.Errorf("expected default namespace, got %q", d.Namespace)
	}
	if *d.Spec.Replicas != 2 {
		t.Errorf("expected 2 replicas, got %d", *d.Spec.Replicas)
	}
}

func TestParseDeploymentRejectsInvalid(t *testing.T) {
	tests := map[string]string{
		"not yaml":   "{{{",
		"wrong kind": "apiVersion: v1\nkind: Service\nmetadata:\n  name: svc\n",
		"no name":    "apiVersion: apps/v1\nkind: Deployment\nmetadata: {}\n",
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDeployment([]byte(manifest)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestPropagateCreatesOnAllClusters(t *testing.T) {
	env := newTestEnv(t, "cluster-a", "cluster-b")
	deployment := mustParse(t)

	results, err := env.engine.Propagate(context.Background(), deployment, []string{"cluster-a", "cluster-b"})
	if err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	for i, name := range []string{"cluster-a", "cluster-b"} {
		if results[i].Cluster != name || results[i].Action != ActionCreated {
			t.Errorf("result %d: expected %s Created, got %+v", i, name, results[i])
		}

		got, err := env.clients[name].AppsV1().Deployments("default").Get(context.Background(), "nginx", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("deployment missing on %s: %v", name, err)
		}
		if got.Labels[ManagedByLabel] != managedByValue {
			t.Errorf("%s: expected managed-by label, got %v", name, got.Labels)
		}
	}

	if deployment.Labels[ManagedByLabel] != "" {
		t.Error("Propagate mutated the caller's deployment")
	}
}

func TestPropagateUpdatesExisting(t *testing.T) {
	env := newTestEnv(t, "cluster-a")
	deployment := mustParse(t)

	if _, err := env.engine.Propagate(context.Background(), deployment, []string{"cluster-a"}); err != nil {
		t.Fatalf("first Propagate failed: %v", err)
	}

	replicas := int32(5)
	deployment.Spec.Replicas = &replicas
	results, err := env.engine.Propagate(context.Background(), deployment, []string{"cluster-a"})
	if err != nil {
		t.Fatalf("second Propagate failed: %v", err)
	}
	if results[0].Action != ActionUpdated {
		t.Errorf("expected Updated, got %+v", results[0])
	}

	got, _ := env.clients["cluster-a"].AppsV1().Deployments("default").Get(context.Background(), "nginx", metav1.GetOptions{})
	if *got.Spec.Replicas != 5 {
		t.Errorf("expected 5 replicas after update, got %d", *got.Spec.Replicas)
	}
}

func TestPropagateUnknownClusterAppliesNothing(t *testing.T) {
	env := newTestEnv(t, "cluster-a")

	_, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a", "missing"})
	if err == nil {
		t.Fatal("expected error for unknown cluster")
	}

	list, _ := env.clients["cluster-a"].AppsV1().Deployments("default").List(context.Background(), metav1.ListOptions{})
	if len(list.Items) != 0 {
		t.Errorf("expected no deployments applied, got %d", len(list.Items))
	}
}

func TestPropagateRejectsBadTargets(t *testing.T) {
	env := newTestEnv(t, "cluster-a")

	if _, err := env.engine.Propagate(context.Background(), mustParse(t), nil); err == nil {
		t.Error("expected error for empty cluster list")
	}
	if _, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a", "cluster-a"}); err == nil {
		t.Error("expected error for duplicate cluster")
	}
}

func TestPropagatePartialFailure(t *testing.T) {
	env := newTestEnv(t, "cluster-a", "cluster-b")
	env.clients["cluster-b"].PrependReactor("create", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api server unavailable")
	})

	results, err := env.engine.Propagate(context.Background(), mustParse(t), []string{"cluster-a", "cluster-b"})
	if err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	if results[0].Action != ActionCreated {
		t.Errorf("cluster-a: expected Created, got %+v", results[0])
	}
	if results[1].Action != ActionFailed || results[1].Error == "" {
		t.Errorf("cluster-b: expected Failed with error, got %+v", results[1])
	}
}
