package client

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orkestra/internal/api"
	"github.com/orkestra/internal/health"
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

// newTestClient starts a real API server backed by one fake cluster client
// and returns a Client pointed at it, plus a kubeconfig path to register.
func newTestClient(t *testing.T) (*Client, string) {
	t.Helper()

	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(testKubeconfig), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}

	cs := fake.NewSimpleClientset()
	factory := func(string) (kubernetes.Interface, error) { return cs, nil }

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	reg := registry.NewRegistry(factory)
	srv := api.NewServer(0, reg,
		health.NewAggregator(reg, factory, 0, logger),
		propagation.NewEngine(reg, factory, logger),
		logger)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return New(ts.URL + "/"), kubeconfigPath
}

func TestClusterRoundTrip(t *testing.T) {
	c, kubeconfig := newTestClient(t)

	cluster, err := c.RegisterCluster("cluster-a", kubeconfig)
	if err != nil {
		t.Fatalf("RegisterCluster failed: %v", err)
	}
	if cluster.Endpoint != "https://localhost:6443" {
		t.Errorf("unexpected endpoint %q", cluster.Endpoint)
	}

	clusters, err := c.ListClusters()
	if err != nil {
		t.Fatalf("ListClusters failed: %v", err)
	}
	if len(clusters) != 1 || clusters[0].Name != "cluster-a" {
		t.Errorf("expected [cluster-a], got %+v", clusters)
	}
}

func TestServerErrorMessageIsReturned(t *testing.T) {
	c, kubeconfig := newTestClient(t)

	if _, err := c.RegisterCluster("dup", kubeconfig); err != nil {
		t.Fatalf("RegisterCluster failed: %v", err)
	}
	_, err := c.RegisterCluster("dup", kubeconfig)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected the server's conflict message, got %v", err)
	}
}

func TestDeploymentRoundTrip(t *testing.T) {
	c, kubeconfig := newTestClient(t)
	if _, err := c.RegisterCluster("cluster-a", kubeconfig); err != nil {
		t.Fatalf("RegisterCluster failed: %v", err)
	}

	resp, err := c.Propagate(testManifest, []string{"cluster-a"})
	if err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}
	if resp.Name != "web" || len(resp.Results) != 1 || resp.Results[0].Action != propagation.ActionCreated {
		t.Errorf("unexpected propagate response: %+v", resp)
	}

	records, err := c.ListDeployments()
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	status, err := c.DeploymentStatus("default", "web")
	if err != nil {
		t.Fatalf("DeploymentStatus failed: %v", err)
	}
	if len(status.Statuses) != 1 || status.Statuses[0].Cluster != "cluster-a" {
		t.Errorf("unexpected status: %+v", status)
	}

	if _, err := c.DeploymentStatus("default", "missing"); err == nil {
		t.Error("expected error for unknown deployment")
	}
}

func TestUnreachableServer(t *testing.T) {
	c := New("http://127.0.0.1:1")
	_, err := c.ListClusters()
	if err == nil || !strings.Contains(err.Error(), "cannot reach Orkestra server") {
		t.Errorf("expected a helpful connection error, got %v", err)
	}
}
