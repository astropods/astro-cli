package k8s

import (
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildConfigMap creates a Kubernetes ConfigMap manifest
func BuildConfigMap(
	namespace string,
	accountName string,
	agentName string,
	buildID string,
	data map[string]string,
) *corev1.ConfigMap {
	configMapName := deployment.GenerateConfigMapName(agentName, buildID)
	labels := deployment.GenerateLabels(accountName, agentName, buildID, "config")

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}

	return configMap
}
