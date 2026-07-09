package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDockerConfigJSON(t *testing.T) {
	raw, err := dockerConfigJSON("registry.example.com", "astrocp_primary_secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("output is not valid json: %v", err)
	}
	entry, ok := parsed.Auths["registry.example.com"]
	if !ok {
		t.Fatalf("no auth entry for host, got %s", raw)
	}
	if entry.Username != "token" || entry.Password != "astrocp_primary_secret" {
		t.Errorf("unexpected creds: %+v", entry)
	}
	want := base64.StdEncoding.EncodeToString([]byte("token:astrocp_primary_secret"))
	if entry.Auth != want {
		t.Errorf("auth = %q, want %q", entry.Auth, want)
	}
}

func TestEnsureRegistryPullSecret(t *testing.T) {
	client := fake.NewClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "tenant-ns"},
	})
	a := &Applier{
		clientset:              client,
		namespace:              "tenant-ns",
		proxyRegistryHost:      "registry.example.com",
		registryPullCredential: "astrocp_primary_secret",
	}

	if err := a.ensureRegistryPullSecret(context.Background()); err != nil {
		t.Fatalf("ensureRegistryPullSecret: %v", err)
	}

	// Secret created, dockerconfigjson type.
	sec, err := client.CoreV1().Secrets("tenant-ns").Get(context.Background(), registryPullSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pull secret not created: %v", err)
	}
	if sec.Type != corev1.SecretTypeDockerConfigJson || len(sec.Data[corev1.DockerConfigJsonKey]) == 0 {
		t.Errorf("unexpected secret shape: type=%s dataKeys=%v", sec.Type, sec.Data)
	}

	// Default SA links the pull secret.
	sa, err := client.CoreV1().ServiceAccounts("tenant-ns").Get(context.Background(), "default", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("default SA get: %v", err)
	}
	if len(sa.ImagePullSecrets) != 1 || sa.ImagePullSecrets[0].Name != registryPullSecretName {
		t.Errorf("default SA imagePullSecrets = %+v, want [%s]", sa.ImagePullSecrets, registryPullSecretName)
	}

	// Idempotent: a second apply neither errors nor duplicates the SA link.
	if err := a.ensureRegistryPullSecret(context.Background()); err != nil {
		t.Fatalf("second ensureRegistryPullSecret: %v", err)
	}
	sa, _ = client.CoreV1().ServiceAccounts("tenant-ns").Get(context.Background(), "default", metav1.GetOptions{})
	if len(sa.ImagePullSecrets) != 1 {
		t.Errorf("SA link not idempotent, got %+v", sa.ImagePullSecrets)
	}
}

func TestEnsureRegistryPullSecret_NoopWhenUnset(t *testing.T) {
	client := fake.NewClientset()
	a := &Applier{clientset: client, namespace: "tenant-ns"} // no credential/host

	if err := a.ensureRegistryPullSecret(context.Background()); err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
	if _, err := client.CoreV1().Secrets("tenant-ns").Get(context.Background(), registryPullSecretName, metav1.GetOptions{}); err == nil {
		t.Error("pull secret should not exist when credential is unset")
	}
}
