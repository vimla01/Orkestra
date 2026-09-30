package propagation

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

// defaultNamespace is used when a manifest does not specify a namespace.
const defaultNamespace = "default"

// ParseDeployment decodes a YAML or JSON manifest into a Deployment. It
// rejects manifests of any other kind, requires a name, and defaults the
// namespace. Server-assigned metadata is stripped so the object can be
// applied to any cluster.
func ParseDeployment(data []byte) (*appsv1.Deployment, error) {
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil, fmt.Errorf("manifest must be an apps/v1 Deployment, got %T", obj)
	}

	if deployment.Name == "" {
		return nil, fmt.Errorf("deployment manifest must set metadata.name")
	}
	if deployment.Namespace == "" {
		deployment.Namespace = defaultNamespace
	}

	deployment.ResourceVersion = ""
	deployment.UID = ""
	deployment.Status = appsv1.DeploymentStatus{}

	return deployment, nil
}
