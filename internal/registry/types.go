package registry

import "time"

// ClusterInfo represents a registered Kubernetes cluster and its health state.
type ClusterInfo struct {
	// Name is the unique identifier for this cluster.
	Name string `json:"name"`

	// KubeconfigPath is the filesystem path to the kubeconfig file.
	// Excluded from API responses for security.
	KubeconfigPath string `json:"-"`

	// Status is the current health status: "Healthy", "Unhealthy", or "Unknown".
	Status string `json:"status"`

	// RegisteredAt is the timestamp when the cluster was first registered.
	RegisteredAt time.Time `json:"registeredAt"`

	// LastHealthCheck is the timestamp of the most recent health check.
	LastHealthCheck time.Time `json:"lastHealthCheck"`

	// NodeCount is the total number of nodes in the cluster.
	NodeCount int `json:"nodeCount"`

	// ReadyNodes is the number of nodes in a Ready condition.
	ReadyNodes int `json:"readyNodes"`

	// Endpoint is the Kubernetes API server URL.
	Endpoint string `json:"endpoint"`
}
