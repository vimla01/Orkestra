package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/orkestra/internal/health"
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

// setupTestServer creates a test server with a real registry backed by a fake clientset.
func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	// Create temp kubeconfig
	tmpDir := t.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(testKubeconfig), 0644); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}

	// Create fake client factory
	fakeFactory := func(path string) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(), nil
	}

	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.DebugLevel)

	reg := registry.NewRegistry(fakeFactory)
	agg := health.NewAggregator(reg, fakeFactory, 0, logger)
	srv := NewServer(0, reg, agg, logger)

	return srv, kubeconfigPath
}

// newRouter creates a mux router with all routes registered for testing.
func newRouter(s *Server) *mux.Router {
	r := mux.NewRouter()
	registerRoutes(s, r)
	return r
}

func TestHandleHealth(t *testing.T) {
	srv, _ := setupTestServer(t)
	router := newRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestHandleRegisterCluster(t *testing.T) {
	srv, kubeconfigPath := setupTestServer(t)
	router := newRouter(srv)

	body := RegisterClusterRequest{
		Name:           "test-cluster",
		KubeconfigPath: kubeconfigPath,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var cluster registry.ClusterInfo
	if err := json.NewDecoder(rec.Body).Decode(&cluster); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if cluster.Name != "test-cluster" {
		t.Errorf("expected cluster name 'test-cluster', got '%s'", cluster.Name)
	}
}

func TestHandleRegisterClusterDuplicate(t *testing.T) {
	srv, kubeconfigPath := setupTestServer(t)
	router := newRouter(srv)

	body := RegisterClusterRequest{
		Name:           "dup-cluster",
		KubeconfigPath: kubeconfigPath,
	}
	jsonBody, _ := json.Marshal(body)

	// First registration
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first registration failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Second registration — should fail with 409
	jsonBody, _ = json.Marshal(body)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListClusters(t *testing.T) {
	srv, kubeconfigPath := setupTestServer(t)
	router := newRouter(srv)

	// Register 2 clusters
	for _, name := range []string{"cluster-a", "cluster-b"} {
		body := RegisterClusterRequest{Name: name, KubeconfigPath: kubeconfigPath}
		jsonBody, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("failed to register %s: %d", name, rec.Code)
		}
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var clusters []registry.ClusterInfo
	if err := json.NewDecoder(rec.Body).Decode(&clusters); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}
}

func TestHandleGetCluster(t *testing.T) {
	srv, kubeconfigPath := setupTestServer(t)
	router := newRouter(srv)

	// Register
	body := RegisterClusterRequest{Name: "get-test", KubeconfigPath: kubeconfigPath}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Get
	req = httptest.NewRequest(http.MethodGet, "/api/v1/clusters/get-test", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var cluster registry.ClusterInfo
	if err := json.NewDecoder(rec.Body).Decode(&cluster); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if cluster.Name != "get-test" {
		t.Errorf("expected 'get-test', got '%s'", cluster.Name)
	}
}

func TestHandleGetClusterNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	router := newRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleDeregisterCluster(t *testing.T) {
	srv, kubeconfigPath := setupTestServer(t)
	router := newRouter(srv)

	// Register
	body := RegisterClusterRequest{Name: "del-test", KubeconfigPath: kubeconfigPath}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/del-test", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rec.Code)
	}

	// Verify it's gone
	req = httptest.NewRequest(http.MethodGet, "/api/v1/clusters/del-test", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 after delete, got %d", rec.Code)
	}
}
