package k8s

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// TestBuildSecret verifies that BuildSecret produces a Secret with the
// {agent}-{version}-credentials name format, Opaque type, credential keys
// uppercased in the data map, correct values preserved as bytes, and the
// astro.dev/agent label set.
func TestBuildSecret(t *testing.T) {
	creds := map[string]string{
		"anthropic_api_key": "sk-ant-test",
		"github_token":      "ghp-test",
	}

	secret := BuildSecret("prod-ns", "my-agent", "1.0", creds)

	// Name format: {agent}-{version}-credentials
	if !strings.Contains(secret.Name, "my-agent") || !strings.Contains(secret.Name, "credentials") {
		t.Errorf("name should follow {agent}-{version}-credentials, got %s", secret.Name)
	}

	if secret.Namespace != "prod-ns" {
		t.Errorf("namespace: expected prod-ns, got %s", secret.Namespace)
	}

	if secret.Type != corev1.SecretTypeOpaque {
		t.Errorf("type: expected Opaque, got %s", secret.Type)
	}

	if secret.Kind != "Secret" {
		t.Errorf("kind: expected Secret, got %s", secret.Kind)
	}

	// Keys should be uppercased
	for key := range creds {
		upperKey := strings.ToUpper(key)
		if _, ok := secret.Data[upperKey]; !ok {
			t.Errorf("expected uppercased key %s in secret data", upperKey)
		}
	}

	// Values should match
	if string(secret.Data["ANTHROPIC_API_KEY"]) != "sk-ant-test" {
		t.Errorf("expected ANTHROPIC_API_KEY=sk-ant-test, got %s", string(secret.Data["ANTHROPIC_API_KEY"]))
	}
	if string(secret.Data["GITHUB_TOKEN"]) != "ghp-test" {
		t.Errorf("expected GITHUB_TOKEN=ghp-test, got %s", string(secret.Data["GITHUB_TOKEN"]))
	}

	// Labels
	if secret.Labels["astro.dev/agent"] != "my-agent" {
		t.Errorf("expected agent label my-agent, got %s", secret.Labels["astro.dev/agent"])
	}
}
