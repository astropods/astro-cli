package deployment

import (
	"strings"
	"testing"
)

// TestSanitizeName verifies that SanitizeName lowercases input, replaces
// underscores and dots with hyphens, strips invalid characters, collapses
// consecutive hyphens, trims leading/trailing hyphens, and truncates to 63
// characters without leaving a trailing hyphen after truncation.
func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already valid",
			input: "my-agent",
			want:  "my-agent",
		},
		{
			name:  "uppercase to lowercase",
			input: "My-Agent",
			want:  "my-agent",
		},
		{
			name:  "underscores to hyphens",
			input: "my_agent_name",
			want:  "my-agent-name",
		},
		{
			name:  "dots to hyphens",
			input: "my.agent.name",
			want:  "my-agent-name",
		},
		{
			name:  "removes invalid characters",
			input: "my@agent!name#1",
			want:  "myagentname1",
		},
		{
			name:  "consecutive hyphens collapsed",
			input: "my---agent",
			want:  "my-agent",
		},
		{
			name:  "leading and trailing hyphens trimmed",
			input: "-my-agent-",
			want:  "my-agent",
		},
		{
			name:  "truncates to 63 characters",
			input: strings.Repeat("a", 70),
			want:  strings.Repeat("a", 63),
		},
		{
			name:  "truncation does not leave trailing hyphen",
			input: strings.Repeat("a", 62) + "-b",
			want:  strings.Repeat("a", 62),
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "mixed special characters",
			input: "Agent_V2.0@prod",
			want:  "agent-v2-0prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify length invariant
			if len(got) > 63 {
				t.Errorf("SanitizeName(%q) produced %d chars, max is 63", tt.input, len(got))
			}
		})
	}
}

// TestGenerateLabels verifies that GenerateLabels produces the correct standard
// Kubernetes labels including app.kubernetes.io/name, instance, version,
// managed-by, astro.dev/agent, and optional component.
func TestGenerateLabels(t *testing.T) {
	t.Run("with component", func(t *testing.T) {
		labels := GenerateLabels("my-agent", "1.0", "agent")

		expected := map[string]string{
			"app.kubernetes.io/name":       "my-agent",
			"app.kubernetes.io/instance":   "my-agent",
			"app.kubernetes.io/version":    "1-0",
			"app.kubernetes.io/managed-by": "astro-server",
			"app.kubernetes.io/component":  "agent",
			"astro.dev/agent":              "my-agent",
		}

		for k, want := range expected {
			if labels[k] != want {
				t.Errorf("label %s: expected %q, got %q", k, want, labels[k])
			}
		}
	})

	t.Run("without component", func(t *testing.T) {
		labels := GenerateLabels("my-agent", "1.0", "")

		if _, ok := labels["app.kubernetes.io/component"]; ok {
			t.Error("expected no component label when component is empty")
		}

		if labels["astro.dev/agent"] != "my-agent" {
			t.Errorf("expected astro.dev/agent=my-agent, got %q", labels["astro.dev/agent"])
		}
	})

	t.Run("sanitizes values", func(t *testing.T) {
		labels := GenerateLabels("My_Agent", "2.0", "Model")

		if labels["app.kubernetes.io/name"] != "my-agent" {
			t.Errorf("expected sanitized name my-agent, got %q", labels["app.kubernetes.io/name"])
		}
		if labels["app.kubernetes.io/component"] != "model" {
			t.Errorf("expected sanitized component model, got %q", labels["app.kubernetes.io/component"])
		}
	})
}

// TestGenerateSelector verifies that GenerateSelector produces the correct
// selector labels (name, instance, astro.dev/agent, optional component) which
// are a subset of the full labels.
func TestGenerateSelector(t *testing.T) {
	t.Run("with component", func(t *testing.T) {
		sel := GenerateSelector("my-agent", "agent")

		expected := map[string]string{
			"app.kubernetes.io/name":      "my-agent",
			"app.kubernetes.io/instance":  "my-agent",
			"app.kubernetes.io/component": "agent",
			"astro.dev/agent":             "my-agent",
		}

		for k, want := range expected {
			if sel[k] != want {
				t.Errorf("selector %s: expected %q, got %q", k, want, sel[k])
			}
		}

		// Selector should NOT have version or managed-by
		if _, ok := sel["app.kubernetes.io/version"]; ok {
			t.Error("selector should not include version")
		}
		if _, ok := sel["app.kubernetes.io/managed-by"]; ok {
			t.Error("selector should not include managed-by")
		}
	})

	t.Run("without component", func(t *testing.T) {
		sel := GenerateSelector("my-agent", "")

		if _, ok := sel["app.kubernetes.io/component"]; ok {
			t.Error("expected no component in selector when component is empty")
		}
	})
}

// TestGenerateResourceName verifies the {agent}-{type}-{name} naming format
// and that all parts are sanitized.
func TestGenerateResourceName(t *testing.T) {
	tests := []struct {
		name         string
		agent        string
		resourceType string
		resourceName string
		want         string
	}{
		{
			name:         "simple",
			agent:        "my-agent",
			resourceType: "knowledge",
			resourceName: "vectors",
			want:         "my-agent-knowledge-vectors",
		},
		{
			name:         "sanitizes parts",
			agent:        "My_Agent",
			resourceType: "Knowledge",
			resourceName: "My.Vectors",
			want:         "my-agent-knowledge-my-vectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateResourceName(tt.agent, tt.resourceType, tt.resourceName)
			if got != tt.want {
				t.Errorf("GenerateResourceName(%q,%q,%q) = %q, want %q",
					tt.agent, tt.resourceType, tt.resourceName, got, tt.want)
			}
		})
	}
}

// TestGenerateAgentResourceName verifies the {agent}-{type} naming format.
func TestGenerateAgentResourceName(t *testing.T) {
	got := GenerateAgentResourceName("my-agent", "agent")
	if got != "my-agent-agent" {
		t.Errorf("expected my-agent-agent, got %q", got)
	}
}

// TestGenerateCredentialSecretName verifies the {agent}-{version}-credentials format
// with dots in version replaced by hyphens.
func TestGenerateCredentialSecretName(t *testing.T) {
	got := GenerateCredentialSecretName("my-agent", "1.0")
	if got != "my-agent-1-0-credentials" {
		t.Errorf("expected my-agent-1-0-credentials, got %q", got)
	}
}

// TestGenerateConfigMapName verifies the {agent}-{version}-config format.
func TestGenerateConfigMapName(t *testing.T) {
	got := GenerateConfigMapName("my-agent", "1.0")
	if got != "my-agent-1-0-config" {
		t.Errorf("expected my-agent-1-0-config, got %q", got)
	}
}

// TestGenerateServiceDNS verifies the {service}.{namespace}.svc.cluster.local format.
func TestGenerateServiceDNS(t *testing.T) {
	got := GenerateServiceDNS("my-agent-agent", "prod-ns")
	want := "my-agent-agent.prod-ns.svc.cluster.local"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
