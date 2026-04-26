package k8s

import (
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildSecret creates a Kubernetes Secret manifest from resolved secret variable values.
func BuildSecret(
	namespace string,
	accountName string,
	agentName string,
	buildID string,
	secretValues map[string]string,
) *corev1.Secret {
	secretName := deployment.GenerateSecretName(agentName, buildID)
	labels := deployment.GenerateLabels(accountName, agentName, buildID, "variables")

	// Encode values - convert keys to uppercase
	data := make(map[string][]byte)
	for key, value := range secretValues {
		upperKey := strings.ToUpper(key)
		data[upperKey] = []byte(value)
	}

	secret := &corev1.Secret{
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

	return secret
}
