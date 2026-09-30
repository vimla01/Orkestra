package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/orkestra/internal/propagation"
)

// maxDeployBodyBytes caps the size of a propagation request body.
const maxDeployBodyBytes = 1 << 20

// PropagateRequest is the JSON body for propagating a deployment.
type PropagateRequest struct {
	// Manifest is a Deployment manifest in YAML or JSON.
	Manifest string `json:"manifest"`

	// Clusters lists the registered clusters to propagate to.
	Clusters []string `json:"clusters"`
}

// PropagateResponse reports the outcome of a propagation on each cluster.
type PropagateResponse struct {
	Namespace string                      `json:"namespace"`
	Name      string                      `json:"name"`
	Results   []propagation.ClusterResult `json:"results"`
}

// handlePropagateDeployment applies a deployment to the requested clusters.
// It responds 200 once every cluster has been attempted; callers must check
// the per-cluster results, since some clusters may have failed.
func (s *Server) handlePropagateDeployment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDeployBodyBytes)

	var req PropagateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Manifest == "" {
		respondError(w, http.StatusBadRequest, "manifest is required")
		return
	}

	deployment, err := propagation.ParseDeployment([]byte(req.Manifest))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Detach from the inbound request so a client disconnect can't abort a
	// rollout halfway; the engine bounds each cluster with its own timeout.
	ctx := context.WithoutCancel(r.Context())
	results, err := s.engine.Propagate(ctx, deployment, req.Clusters)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, PropagateResponse{
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
		Results:   results,
	})
}

// handleListDeployments returns every recorded propagation.
func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, s.engine.List())
}

// handleGetDeploymentStatus returns a propagated deployment's live rollout
// state on each of its target clusters.
func (s *Server) handleGetDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	namespace, name := vars["namespace"], vars["name"]

	status, err := s.engine.Status(r.Context(), namespace, name)
	if errors.Is(err, propagation.ErrDeploymentNotFound) {
		respondError(w, http.StatusNotFound, "deployment not found: "+namespace+"/"+name)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}
