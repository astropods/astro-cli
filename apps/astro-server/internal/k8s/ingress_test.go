package k8s

import (
	"strings"
	"testing"
)

func TestGenerateIngressHost(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		namespace string
		domain    string
	}{
		{
			name:      "short agent name",
			agentName: "my-agent",
			namespace: "default",
			domain:    "example.com",
		},
		{
			name:      "long agent name",
			agentName: strings.Repeat("a", 60),
			namespace: "default",
			domain:    "example.com",
		},
		{
			name:      "very long agent name - needs truncation",
			agentName: "very-long-agent-name-that-exceeds-normal-limits-and-needs-truncation",
			namespace: "production",
			domain:    "example.com",
		},
		{
			name:      "names with special characters",
			agentName: "agent_with_underscores",
			namespace: "namespace.with.dots",
			domain:    "example.com",
		},
		{
			name:      "different namespaces same agent",
			agentName: "test-agent",
			namespace: "prod",
			domain:    "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateIngressHost(tt.agentName, tt.namespace, tt.domain)

			// Verify result has correct format: label.domain
			parts := strings.Split(result, ".")
			if len(parts) < 2 {
				t.Errorf("Expected hostname to have format label.domain, got: %s", result)
				return
			}

			// Extract the label (everything before the domain)
			label := parts[0]

			// Verify label doesn't exceed 59 characters
			if len(label) > 59 {
				t.Errorf("Label exceeds 59 characters: %d chars in %s", len(label), label)
			}

			// Verify domain is preserved
			domain := strings.Join(parts[1:], ".")
			if domain != tt.domain {
				t.Errorf("Expected domain %s, got %s", tt.domain, domain)
			}

			// Verify hash is present (should always have a hash)
			if !strings.Contains(label, "-") {
				t.Errorf("Expected label to contain hash suffix, got: %s", label)
			}

			// Verify the hash suffix format (should end with -[16 hex chars])
			lastHyphen := strings.LastIndex(label, "-")
			if lastHyphen == -1 {
				t.Errorf("Expected label to contain hyphen, got: %s", label)
			} else {
				hashPart := label[lastHyphen+1:]
				if len(hashPart) != 16 {
					t.Errorf("Expected 16 character hash, got %d chars: %s", len(hashPart), hashPart)
				}
				// Verify hash is hex
				for _, c := range hashPart {
					if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
						t.Errorf("Hash contains non-hex character: %c in %s", c, hashPart)
					}
				}
			}

			// Verify namespace is NOT in the label (privacy)
			if strings.Contains(label, tt.namespace) {
				t.Errorf("Label should not contain namespace for privacy, but found %s in %s", tt.namespace, label)
			}

			// Verify label starts and ends with alphanumeric (DNS requirement)
			if len(label) > 0 {
				first := label[0]
				last := label[len(label)-1]
				if !isAlphanumeric(first) || !isAlphanumeric(last) {
					t.Errorf("Label must start and end with alphanumeric, got: %s", label)
				}
			}

			t.Logf("Result: %s (label length: %d)", result, len(label))
		})
	}
}

func TestGenerateIngressHostUniqueness(t *testing.T) {
	// Test that different inputs produce different hostnames
	host1 := GenerateIngressHost("my-agent", "namespace1", "example.com")
	host2 := GenerateIngressHost("my-agent", "namespace2", "example.com")

	if host1 == host2 {
		t.Errorf("Different namespaces should produce different hostnames, got: %s = %s", host1, host2)
	}

	// Test that same inputs produce same hostname (deterministic)
	host3 := GenerateIngressHost("my-agent", "namespace1", "example.com")
	if host1 != host3 {
		t.Errorf("Same inputs should produce same hostname, got: %s != %s", host1, host3)
	}

	// Test different agents
	host4 := GenerateIngressHost("agent1", "namespace1", "example.com")
	host5 := GenerateIngressHost("agent2", "namespace1", "example.com")

	if host4 == host5 {
		t.Errorf("Different agents should produce different hostnames, got: %s = %s", host4, host5)
	}
}

func TestGenerateIngressHostMaxLength(t *testing.T) {
	// Test with extremely long inputs
	longAgent := strings.Repeat("a", 100)
	longNamespace := strings.Repeat("b", 100)

	result := GenerateIngressHost(longAgent, longNamespace, "example.com")
	parts := strings.Split(result, ".")
	label := parts[0]

	if len(label) > 59 {
		t.Errorf("Label should never exceed 59 characters, got %d: %s", len(label), label)
	}

	t.Logf("Max length test result: %s (label length: %d)", result, len(label))
}

func TestGenerateExternalURL(t *testing.T) {
	url := GenerateExternalURL("my-agent", "default", "example.com")

	if !strings.HasPrefix(url, "https://") {
		t.Errorf("Expected URL to start with https://, got: %s", url)
	}

	if !strings.Contains(url, "example.com") {
		t.Errorf("Expected URL to contain domain example.com, got: %s", url)
	}
}

func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
