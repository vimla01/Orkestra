package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/orkestra/internal/health"
	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const testManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: apps
spec:
  replicas: 1
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

// setupDeployServer returns a router whose single registered cluster,
// "cluster-a", is backed by one fake clientset that persists across calls.
func setupDeployServer(t *testing.T) *mux.Router {
	t.Helper()

	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(testKubeconfig), 0600); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}

	client := fake.NewSimpleClientset()
	factory := func(string) (kubernetes.Interface, error) { return client, nil }

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	reg := registry.NewRegistry(factory)
	if _, err := reg.Register("cluster-a", kubeconfigPath); err != nil {
		t.Fatalf("failed to register cluster: %v", err)
	}
	agg := health.NewAggregator(reg, factory, 0, logger)
	engine := propagation.NewEngine(reg, factory, logger)

	return newRouter(NewServer(0, reg, agg, engine, logger))
}

func postDeployment(t *testing.T, router *mux.Router, body PropagateRequest) *httptest.ResponseRecorder {
	t.Helper()
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandlePropagateDeployment(t *testing.T) {
	router := setupDeployServer(t)

	rec := postDeployment(t, router, PropagateRequest{Manifest: testManifest, Clusters: []string{"cluster-a"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp PropagateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Namespace != "apps" || resp.Name != "web" {
		t.Errorf("expected apps/web, got %s/%s", resp.Namespace, resp.Name)
	}
	if len(resp.Results) != 1 || resp.Results[0].Action != propagation.ActionCreated {
		t.Errorf("expected one Created result, got %+v", resp.Results)
	}
}

func TestHandlePropagateDeploymentBadRequests(t *testing.T) {
	router := setupDeployServer(t)

	tests := map[string]PropagateRequest{
		"missing manifest": {Clusters: []string{"cluster-a"}},
		"invalid manifest": {Manifest: "kind: Service", Clusters: []string{"cluster-a"}},
		"no clusters":      {Manifest: testManifest},
		"unknown cluster":  {Manifest: testManifest, Clusters: []string{"nope"}},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if rec := postDeployment(t, router, body); rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleListDeployments(t *testing.T) {
	router := setupDeployServer(t)
	postDeployment(t, router, PropagateRequest{Manifest: testManifest, Clusters: []string{"cluster-a"}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var records []propagation.Record
	if err := json.NewDecoder(rec.Body).Decode(&records); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(records) != 1 || records[0].Name != "web" {
		t.Errorf("expected one record for web, got %+v", records)
	}
}

func TestHandleGetDeploymentStatus(t *testing.T) {
	router := setupDeployServer(t)
	postDeployment(t, router, PropagateRequest{Manifest: testManifest, Clusters: []string{"cluster-a"}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/apps/web", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status propagation.DeploymentStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(status.Statuses) != 1 || status.Statuses[0].Cluster != "cluster-a" {
		t.Errorf("expected status for cluster-a, got %+v", status.Statuses)
	}
	if status.Statuses[0].Error != "" {
		t.Errorf("unexpected error: %s", status.Statuses[0].Error)
	}
}

func TestHandleGetDeploymentStatusNotFound(t *testing.T) {
	router := setupDeployServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/apps/missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}
