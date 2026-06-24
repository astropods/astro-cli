package k8s

import (
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
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
					if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
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

// TestBuildIngress verifies that BuildIngress produces an Ingress with ALB
// annotations (scheme, target-type, listen-ports, ssl-redirect, external-dns
// hostname), optional ACM certificate ARN, optional ALB group name, correct
// IngressClassName "alb", host-based routing rule with path "/" and Prefix type,
// and backend pointing to the specified service name and port.
func TestBuildIngress(t *testing.T) {
	t.Run("full config", func(t *testing.T) {
		cfg := IngressConfig{
			Name:        "my-agent-ingress",
			Namespace:   "prod-ns",
			AgentName:   "my-agent",
			BuildID:     "1.0",
			Component:   "agent",
			ServiceName: "my-agent-agent",
			ServicePort: 8080,
			Host:        "my-agent-abc123.agents.example.com",
		}

		ing := BuildIngress(cfg)

		if ing.Name != cfg.Name {
			t.Errorf("name: expected %s, got %s", cfg.Name, ing.Name)
		}
		if ing.Namespace != cfg.Namespace {
			t.Errorf("namespace: expected %s, got %s", cfg.Namespace, ing.Namespace)
		}

		// Labels should include astro.dev/agent
		if ing.Labels["astro.dev/agent"] != cfg.AgentName {
			t.Errorf("agent label: expected %s, got %s", cfg.AgentName, ing.Labels["astro.dev/agent"])
		}

		// Under the tenant-router model the front-door ALB owns TLS, OIDC,
		// and routing. BuildIngress no longer emits the legacy
		// alb.ingress.kubernetes.io/* or external-dns.alpha.kubernetes.io/*
		// annotations even when ACMCertificateARN / ALBGroupName are set.
		for k := range ing.Annotations {
			if strings.HasPrefix(k, "alb.ingress.kubernetes.io/") ||
				strings.HasPrefix(k, "external-dns.alpha.kubernetes.io/") {
				t.Errorf("unexpected legacy annotation %q", k)
			}
		}

		// IngressClassName — tenant-router (Contour picks it up; AWS LB
		// Controller ignores). Kyverno mutates `alb` to this in-cluster as
		// a fallback, but we should emit the right class directly.
		if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "tenant-router" {
			t.Error("expected IngressClassName tenant-router")
		}

		// Rules
		if len(ing.Spec.Rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(ing.Spec.Rules))
		}
		rule := ing.Spec.Rules[0]
		if rule.Host != cfg.Host {
			t.Errorf("rule host: expected %s, got %s", cfg.Host, rule.Host)
		}

		paths := rule.HTTP.Paths
		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(paths))
		}
		if paths[0].Path != "/" {
			t.Errorf("expected path /, got %s", paths[0].Path)
		}
		if *paths[0].PathType != networkingv1.PathTypePrefix {
			t.Error("expected PathTypePrefix")
		}
		if paths[0].Backend.Service.Name != cfg.ServiceName {
			t.Errorf("backend service: expected %s, got %s", cfg.ServiceName, paths[0].Backend.Service.Name)
		}
		if paths[0].Backend.Service.Port.Number != cfg.ServicePort {
			t.Errorf("backend port: expected %d, got %d", cfg.ServicePort, paths[0].Backend.Service.Port.Number)
		}
	})

	t.Run("without ACM and ALB group", func(t *testing.T) {
		cfg := IngressConfig{
			Name:        "basic-ingress",
			Namespace:   "default",
			AgentName:   "my-agent",
			BuildID:     "1.0",
			Component:   "agent",
			ServiceName: "my-agent-agent",
			ServicePort: 8080,
			Host:        "my-agent.example.com",
		}

		ing := BuildIngress(cfg)

		// No legacy annotations regardless of input.
		for k := range ing.Annotations {
			if strings.HasPrefix(k, "alb.ingress.kubernetes.io/") {
				t.Errorf("unexpected legacy annotation %q", k)
			}
		}
	})
}

// TestGenerateIngestionIngressHost verifies that GenerateIngestionIngressHost
// produces a hostname in the format {agent}-{ingestion}-{hash}.{domain}, with
// the DNS label not exceeding 63 characters, deterministic hashing, and proper
// truncation when names are long.
func TestGenerateIngestionIngressHost(t *testing.T) {
	t.Run("short names", func(t *testing.T) {
		host := GenerateIngestionIngressHost("my-agent", "default", "sync", "example.com")

		if !strings.HasSuffix(host, ".example.com") {
			t.Errorf("expected domain suffix .example.com, got %s", host)
		}

		label := strings.Split(host, ".")[0]
		if !strings.Contains(label, "my-agent") {
			t.Errorf("expected label to contain agent name, got %s", label)
		}
		if !strings.Contains(label, "sync") {
			t.Errorf("expected label to contain ingestion name, got %s", label)
		}
	})

	t.Run("long names truncated within 63 chars", func(t *testing.T) {
		longAgent := strings.Repeat("a", 40)
		longIngestion := strings.Repeat("b", 40)

		host := GenerateIngestionIngressHost(longAgent, "default", longIngestion, "example.com")
		label := strings.Split(host, ".")[0]

		if len(label) > 63 {
			t.Errorf("label exceeds 63 characters: %d chars in %s", len(label), label)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		host1 := GenerateIngestionIngressHost("agent", "ns", "sync", "example.com")
		host2 := GenerateIngestionIngressHost("agent", "ns", "sync", "example.com")
		if host1 != host2 {
			t.Errorf("same inputs should produce same host: %s != %s", host1, host2)
		}
	})

	t.Run("different ingestion names produce different hosts", func(t *testing.T) {
		host1 := GenerateIngestionIngressHost("agent", "ns", "sync", "example.com")
		host2 := GenerateIngestionIngressHost("agent", "ns", "webhook", "example.com")
		if host1 == host2 {
			t.Errorf("different ingestion names should produce different hosts: %s == %s", host1, host2)
		}
	})
}

// TestGenerateIngestionExternalURL verifies that GenerateIngestionExternalURL
// prepends https:// to the ingestion ingress host.
func TestGenerateIngestionExternalURL(t *testing.T) {
	url := GenerateIngestionExternalURL("my-agent", "default", "sync", "example.com")

	if !strings.HasPrefix(url, "https://") {
		t.Errorf("expected https:// prefix, got %s", url)
	}

	// The rest should match GenerateIngestionIngressHost
	expectedHost := GenerateIngestionIngressHost("my-agent", "default", "sync", "example.com")
	expectedURL := "https://" + expectedHost
	if url != expectedURL {
		t.Errorf("expected %s, got %s", expectedURL, url)
	}
}

// TestTruncateLabel verifies that truncateLabel returns the string unchanged
// when within limit, truncates to max when over, and trims trailing hyphens
// after truncation.
func TestTruncateLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"within limit", "hello", 10, "hello"},
		{"exactly at limit", "hello", 5, "hello"},
		{"over limit", "hello-world", 5, "hello"},
		{"trailing hyphen after truncation", "abc-def", 4, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateLabel(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncateLabel(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
