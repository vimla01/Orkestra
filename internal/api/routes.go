package api

import "github.com/gorilla/mux"

// registerRoutes configures all API routes on the provided mux router.
func registerRoutes(s *Server, r *mux.Router) {
	// Health endpoint.
	r.HandleFunc("/api/v1/health", s.handleHealth).Methods("GET")

	// Cluster management endpoints.
	r.HandleFunc("/api/v1/clusters", s.handleRegisterCluster).Methods("POST")
	r.HandleFunc("/api/v1/clusters", s.handleListClusters).Methods("GET")
	r.HandleFunc("/api/v1/clusters/{name}", s.handleGetCluster).Methods("GET")
	r.HandleFunc("/api/v1/clusters/{name}", s.handleDeregisterCluster).Methods("DELETE")

	// On-demand health check endpoint.
	r.HandleFunc("/api/v1/clusters/{name}/healthcheck", s.handleTriggerHealthCheck).Methods("POST")
}
