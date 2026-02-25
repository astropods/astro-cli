package k8s

import (
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	kedaNamespace      = "keda"
	interceptorService = "keda-add-ons-http-interceptor"
	interceptorPort    = int64(8080)
)

var (
	httpScaledObjectGVR = schema.GroupVersionResource{Group: "http.keda.sh", Version: "v1alpha1", Resource: "httpscaledobjects"}
	scaledObjectGVR     = schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"}
)

// HTTPScaledObjectConfig holds config for a KEDA HTTPScaledObject.
type HTTPScaledObjectConfig struct {
	Name           string
	Namespace      string
	AgentName      string
	BuildID        string
	Component      string
	Host           string
	DeploymentName string
	ServiceName    string
	ServicePort    int32
}

// BuildHTTPScaledObject builds a KEDA HTTPScaledObject targeting the given Deployment.
// It scales to 0 when the KEDA interceptor receives no traffic for the given host.
func BuildHTTPScaledObject(cfg HTTPScaledObjectConfig) *unstructured.Unstructured {
	labels := labelsToInterface(deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component))
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "http.keda.sh/v1alpha1",
			"kind":       "HTTPScaledObject",
			"metadata": map[string]any{
				"name":      cfg.Name,
				"namespace": cfg.Namespace,
				"labels":    labels,
			},
			"spec": map[string]any{
				"hosts": []any{cfg.Host},
				"scaleTargetRef": map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       cfg.DeploymentName,
					"service":    cfg.ServiceName,
					"port":       int64(cfg.ServicePort), //nolint:gosec
				},
				"replicas": map[string]any{
					"min": int64(0),
					"max": int64(1),
				},
			},
		},
	}
}

// WorkloadScaledObjectConfig holds config for a KEDA ScaledObject using the
// kubernetes-workload trigger. It scales the target Deployment to 0 when no
// pods matching podSelector are running, and back to 1 when they appear.
type WorkloadScaledObjectConfig struct {
	Name           string
	Namespace      string
	AgentName      string
	BuildID        string
	Component      string
	DeploymentName string
	PodSelector    string
}

// BuildWorkloadScaledObject builds a KEDA ScaledObject that mirrors the replica count
// of another workload. Used to keep the agent in sync with its messaging adapter.
func BuildWorkloadScaledObject(cfg WorkloadScaledObjectConfig) *unstructured.Unstructured {
	labels := labelsToInterface(deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component))
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata": map[string]any{
				"name":      cfg.Name,
				"namespace": cfg.Namespace,
				"labels":    labels,
			},
			"spec": map[string]any{
				"scaleTargetRef": map[string]any{
					"name": cfg.DeploymentName,
				},
				"minReplicaCount": int64(0),
				"maxReplicaCount": int64(1),
				"triggers": []any{
					map[string]any{
						"type": "kubernetes-workload",
						"metadata": map[string]any{
							"podSelector": cfg.PodSelector,
							"value":       "1",
						},
					},
				},
			},
		},
	}
}

// messagingPodSelector returns the KEDA pod selector string for the messaging
// adapter pods of the given agent.
func messagingPodSelector(agentName string) string {
	return fmt.Sprintf("app.kubernetes.io/name=%s,app.kubernetes.io/component=messaging",
		deployment.SanitizeName(agentName),
	)
}

// labelsToInterface converts map[string]string to map[string]any for unstructured objects.
func labelsToInterface(labels map[string]string) map[string]any {
	out := make(map[string]any, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out
}
