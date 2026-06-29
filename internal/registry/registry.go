package registry

import (
	"fmt"
	"os"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Registry is a thread-safe in-memory store of registered Kubernetes clusters.
type Registry struct {
	mu            sync.RWMutex
	clusters      map[string]*ClusterInfo
	clientFactory func(string) (kubernetes.Interface, error)
}

// NewRegistry creates a new Registry with the given client factory.
// The clientFactory function receives a kubeconfig path and returns a
// kubernetes.Interface that can communicate with the target cluster.
func NewRegistry(clientFactory func(string) (kubernetes.Interface, error)) *Registry {
	return &Registry{
		clusters:      make(map[string]*ClusterInfo),
		clientFactory: clientFactory,
	}
}

// Register adds a new cluster to the registry.
//
// It validates that the kubeconfig file exists, uses the client factory to
// verify basic connectivity (ServerVersion), and extracts the API server
// endpoint from the kubeconfig. The initial status is set to "Unknown".
func (r *Registry) Register(name, kubeconfigPath string) (*ClusterInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate registration.
	if _, exists := r.clusters[name]; exists {
		return nil, fmt.Errorf("cluster %q is already registered", name)
	}

	// Validate that the kubeconfig file exists on disk.
	if _, err := os.Stat(kubeconfigPath); err != nil {
		return nil, fmt.Errorf("kubeconfig file not found: %w", err)
	}

	// Verify connectivity by building a clientset and calling ServerVersion.
	client, err := r.clientFactory(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for cluster %q: %w", name, err)
	}
	if _, err := client.Discovery().ServerVersion(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %q: %w", name, err)
	}

	// Extract the API server endpoint from the kubeconfig.
	endpoint, err := extractEndpoint(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract endpoint for cluster %q: %w", name, err)
	}

	info := &ClusterInfo{
		Name:           name,
		KubeconfigPath: kubeconfigPath,
		Status:         "Unknown",
		RegisteredAt:   time.Now(),
		Endpoint:       endpoint,
	}

	r.clusters[name] = info
	return info, nil
}

// Deregister removes a cluster from the registry.
// Returns an error if the cluster is not found.
func (r *Registry) Deregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clusters[name]; !exists {
		return fmt.Errorf("cluster %q not found", name)
	}
	delete(r.clusters, name)
	return nil
}

// Get returns the ClusterInfo for the named cluster.
// Returns an error if the cluster is not found.
func (r *Registry) Get(name string) (*ClusterInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.clusters[name]
	if !exists {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	return info, nil
}

// List returns a snapshot copy of all registered clusters.
func (r *Registry) List() []*ClusterInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ClusterInfo, 0, len(r.clusters))
	for _, info := range r.clusters {
		result = append(result, info)
	}
	return result
}

// UpdateHealth updates the health status and node counts for a registered
// cluster. The LastHealthCheck timestamp is set to the current time.
func (r *Registry) UpdateHealth(name string, status string, nodeCount, readyNodes int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.clusters[name]
	if !exists {
		return fmt.Errorf("cluster %q not found", name)
	}

	info.Status = status
	info.NodeCount = nodeCount
	info.ReadyNodes = readyNodes
	info.LastHealthCheck = time.Now()
	return nil
}

// extractEndpoint loads a kubeconfig file and returns the Server URL of the
// first cluster defined in it.
func extractEndpoint(kubeconfigPath string) (string, error) {
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	for _, cluster := range config.Clusters {
		return cluster.Server, nil
	}

	return "", fmt.Errorf("no clusters found in kubeconfig %q", kubeconfigPath)
}
