package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupDashboard returns a server handler serving a minimal dashboard build.
func setupDashboard(t *testing.T) http.Handler {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>dashboard</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0644); err != nil {
		t.Fatal(err)
	}

	srv, _ := setupTestServer(t)
	srv.ServeDashboard(dir)
	return srv.Handler()
}

func TestDashboardServesFiles(t *testing.T) {
	h := setupDashboard(t)

	for path, want := range map[string]string{
		"/":              "<h1>dashboard</h1>",
		"/assets/app.js": "console.log(1)",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s: got %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestDashboardDoesNotShadowAPI(t *testing.T) {
	h := setupDashboard(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("API route broken by dashboard: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Errorf("unknown API path should be a JSON 404, got %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestDashboardHidesDirectoryListings(t *testing.T) {
	h := setupDashboard(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for directory listing, got %d", rec.Code)
	}
}
