package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClientFromKubeconfig builds a Kubernetes clientset from a kubeconfig file.
func NewClientFromKubeconfig(kubeconfigPath string) (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config from %q: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// GetServerEndpoint loads a kubeconfig file and returns the API server URL
// of the first cluster defined in it.
func GetServerEndpoint(kubeconfigPath string) (string, error) {
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig from %q: %w", kubeconfigPath, err)
	}

	for _, cluster := range config.Clusters {
		return cluster.Server, nil
	}

	return "", fmt.Errorf("no clusters found in kubeconfig %q", kubeconfigPath)
}
