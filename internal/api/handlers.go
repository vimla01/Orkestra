package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// RegisterClusterRequest is the JSON body for cluster registration.
type RegisterClusterRequest struct {
	Name           string `json:"name"`
	KubeconfigPath string `json:"kubeconfigPath"`
}

// handleHealth returns the control plane liveness status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRegisterCluster registers a new Kubernetes cluster.
func (s *Server) handleRegisterCluster(w http.ResponseWriter, r *http.Request) {
	var req RegisterClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.KubeconfigPath == "" {
		respondError(w, http.StatusBadRequest, "kubeconfigPath is required")
		return
	}

	cluster, err := s.registry.Register(req.Name, req.KubeconfigPath)
	if err != nil {
		// Check if it's a duplicate registration
		if existing, _ := s.registry.Get(req.Name); existing != nil {
			respondError(w, http.StatusConflict, "cluster already registered: "+req.Name)
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.WithField("cluster", req.Name).Info("Cluster registered successfully")
	respondJSON(w, http.StatusCreated, cluster)
}

// handleListClusters returns all registered clusters.
func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	clusters := s.registry.List()
	respondJSON(w, http.StatusOK, clusters)
}

// handleGetCluster returns a single cluster by name.
func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	cluster, err := s.registry.Get(name)
	if err != nil {
		respondError(w, http.StatusNotFound, "cluster not found: "+name)
		return
	}

	respondJSON(w, http.StatusOK, cluster)
}

// handleDeregisterCluster removes a cluster from the registry.
func (s *Server) handleDeregisterCluster(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	if err := s.registry.Deregister(name); err != nil {
		respondError(w, http.StatusNotFound, "cluster not found: "+name)
		return
	}

	s.logger.WithField("cluster", name).Info("Cluster deregistered successfully")
	w.WriteHeader(http.StatusNoContent)
}

// handleTriggerHealthCheck triggers an on-demand health check for a cluster.
func (s *Server) handleTriggerHealthCheck(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	// Verify cluster exists
	if _, err := s.registry.Get(name); err != nil {
		respondError(w, http.StatusNotFound, "cluster not found: "+name)
		return
	}

	// Trigger health check
	if err := s.aggregator.CheckCluster(r.Context(), name); err != nil {
		s.logger.WithField("cluster", name).WithError(err).Warn("Health check failed")
	}

	// Return updated cluster info
	cluster, err := s.registry.Get(name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, cluster)
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
