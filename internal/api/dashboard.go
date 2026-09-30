package api

import (
	"net/http"
	"strings"
)

// dashboardHandler serves the static dashboard build in dir. Unknown API
// paths get a JSON 404 rather than a page, and directory listings are
// disabled.
func dashboardHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") {
			respondError(w, http.StatusNotFound, "no such API endpoint: "+path)
			return
		}
		if path != "/" && strings.HasSuffix(path, "/") {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
