package k8s

import (
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildSecret creates the agent's main credentials Secret (one per deployment,
// containing every secret variable in the spec).
func BuildSecret(
	namespace string,
	accountName string,
	agentName string,
	buildID string,
	secretValues map[string]string,
) *corev1.Secret {
	return BuildNamedSecret(
		namespace,
		deployment.GenerateSecretName(agentName, buildID),
		accountName, agentName, buildID,
		"variables",
		secretValues,
	)
}

// BuildNamedSecret creates a Kubernetes Secret with an explicit name and
// component label. Used by the messaging sidecar to get its own narrower
// credentials bundle (only the keys it references in interfaces.environment),
// independent of the agent's main credentials Secret.
func BuildNamedSecret(
	namespace string,
	secretName string,
	accountName string,
	agentName string,
	buildID string,
	component string,
	secretValues map[string]string,
) *corev1.Secret {
	labels := deployment.GenerateLabels(accountName, agentName, buildID, component)

	// Encode values - convert keys to uppercase to match env-var conventions.
	data := make(map[string][]byte)
	for key, value := range secretValues {
		upperKey := strings.ToUpper(key)
		data[upperKey] = []byte(value)
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}
