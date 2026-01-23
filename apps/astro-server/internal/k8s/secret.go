package k8s

import (
	"encoding/base64"
	"strings"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildSecret creates a Kubernetes Secret manifest from user credentials
func BuildSecret(
	namespace string,
	agentName string,
	version string,
	userCredentials map[string]string,
) *corev1.Secret {
	secretName := deployment.GenerateCredentialSecretName(agentName, version)
	labels := deployment.GenerateLabels(agentName, version, "credentials")

	// Encode credentials - convert keys to uppercase
	data := make(map[string][]byte)
	for key, value := range userCredentials {
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

// EncodeSecretData encodes a string to base64 for use in Kubernetes secrets
func EncodeSecretData(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
