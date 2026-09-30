package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/orkestra/internal/health"
	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
	"github.com/sirupsen/logrus"
)

// Server is the REST API server for the Orkestra control plane.
// It exposes endpoints for cluster registration, health status, and management.
type Server struct {
	registry   *registry.Registry
	aggregator *health.Aggregator
	engine     *propagation.Engine
	httpServer *http.Server
	logger     *logrus.Logger
}

// NewServer creates a new API Server on the given port with the provided
// registry, health aggregator, and propagation engine. It configures routing, middleware, and
// HTTP server timeouts.
func NewServer(port int, reg *registry.Registry, agg *health.Aggregator, engine *propagation.Engine, logger *logrus.Logger) *Server {
	s := &Server{
		registry:   reg,
		aggregator: agg,
		engine:     engine,
		logger:     logger,
	}

	r := mux.NewRouter()
	registerRoutes(s, r)

	// Apply middleware: CORS first, then logging.
	r.Use(corsMiddleware)
	r.Use(loggingMiddleware(logger))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Handler returns the server's HTTP handler with all routes and middleware,
// for use in tests or when embedding the API in another server.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start begins listening for HTTP requests. It blocks until the server
// encounters an error or is shut down.
func (s *Server) Start() error {
	s.logger.WithField("addr", s.httpServer.Addr).Info("Starting API server")
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server, waiting for in-flight
// requests to complete or the context to expire.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down API server")
	return s.httpServer.Shutdown(ctx)
}
