package k8s

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressConfig holds configuration for building an Ingress.
type IngressConfig struct {
	Name        string
	Namespace   string
	AccountID   string
	AgentName   string
	BuildID     string
	Component   string
	ServiceName string
	ServicePort int32
	Host        string // Full hostname (e.g., agent-name-namespace.agents.example.com)
}

// BuildIngress creates a Kubernetes Ingress manifest for the tenant-router
// data plane (Contour + front-door ALB managed in astro-infra).
//
// The Ingress carries only the host + backend rule and the tenant-router
// class. TLS termination, WorkOS OIDC, and DNS (wildcard *.agents.<domain>
// + *.ingestion.<domain>) are owned by the front-door ALB in astro-infra.
//
// See docs/plans/tenant-router-migration.md in astro-infra.
func BuildIngress(cfg IngressConfig) *networkingv1.Ingress {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	pathType := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("tenant-router"),
			Rules: []networkingv1.IngressRule{
				{
					Host: cfg.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: cfg.ServiceName,
											Port: networkingv1.ServiceBackendPort{
												Number: cfg.ServicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GenerateIngressHost generates the hostname for an agent's ingress
// Format: <agent-name>-<hash>.domain
// Always uses a hash to avoid leaking namespace information
// Ensures the hostname label doesn't exceed 59 characters
func GenerateIngressHost(agentName, namespace, domain string) string {
	const maxLabelLength = 59
	const hashLength = 16 // 16 character hex hash

	// Sanitize inputs
	sanitizedAgent := deployment.SanitizeName(agentName)
	sanitizedNs := deployment.SanitizeName(namespace)

	// Generate hash from agent + namespace for uniqueness and privacy
	combined := fmt.Sprintf("%s-%s", sanitizedAgent, sanitizedNs)
	hash := sha256.Sum256([]byte(combined))
	hashSuffix := fmt.Sprintf("%x", hash[:8]) // 8 bytes = 16 hex chars

	// Format: {agent}-{hash}
	// Calculate max agent name length: maxLabelLength - hashLength - 1 (for hyphen)
	maxAgentLength := maxLabelLength - hashLength - 1

	// Truncate agent name if needed
	agentPart := sanitizedAgent
	if len(agentPart) > maxAgentLength {
		agentPart = agentPart[:maxAgentLength]
		// Ensure doesn't end with hyphen after truncation
		agentPart = strings.TrimRight(agentPart, "-")
	}

	// Combine: agent-hash
	label := fmt.Sprintf("%s-%s", agentPart, hashSuffix)

	return fmt.Sprintf("%s.%s", label, domain)
}

// GenerateIngestionIngressHost generates a unique hostname for an ingestion webhook
// Format: <agent>-<ingestion>-<hash>.domain
// Each part gets a fair share of the 63-char DNS label limit so neither is lost.
// The hash includes agent + namespace + ingestion name for uniqueness.
func GenerateIngestionIngressHost(agentName, namespace, ingestionName, domain string) string {
	const maxLabelLength = 63
	const hashLength = 8 // 4 bytes = 8 hex chars — sufficient uniqueness within one agent
	const separators = 2 // two hyphens: agent-ingestion-hash

	sanitizedAgent := deployment.SanitizeName(agentName)
	sanitizedNs := deployment.SanitizeName(namespace)
	sanitizedIngestion := deployment.SanitizeName(ingestionName)

	// Hash includes ingestion name for per-webhook uniqueness
	combined := fmt.Sprintf("%s-%s-%s", sanitizedAgent, sanitizedNs, sanitizedIngestion)
	hash := sha256.Sum256([]byte(combined))
	hashSuffix := fmt.Sprintf("%x", hash[:4]) // 4 bytes = 8 hex chars

	// Budget available for agent + ingestion names
	budget := maxLabelLength - hashLength - separators // 63 - 8 - 2 = 53

	agentPart := sanitizedAgent
	ingestionPart := sanitizedIngestion

	// If both fit, use as-is
	totalLen := len(agentPart) + len(ingestionPart)
	if totalLen > budget {
		half := budget / 2 // 26 each

		// If one part is short, give its unused space to the other
		if len(agentPart) <= half {
			ingestionPart = truncateLabel(ingestionPart, budget-len(agentPart))
		} else if len(ingestionPart) <= half {
			agentPart = truncateLabel(agentPart, budget-len(ingestionPart))
		} else {
			// Both are long — split evenly
			agentPart = truncateLabel(agentPart, half)
			ingestionPart = truncateLabel(ingestionPart, budget-len(agentPart))
		}
	}

	label := fmt.Sprintf("%s-%s-%s", agentPart, ingestionPart, hashSuffix)
	return fmt.Sprintf("%s.%s", label, domain)
}

// truncateLabel truncates s to max chars and trims trailing hyphens
func truncateLabel(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimRight(s[:max], "-")
}

// GenerateMessagingIngressHost generates the hostname for the messaging (web adapter) ingress.
// Uses "chat" as a fixed component so the hostname is distinct from the agent ingress even
// when both use the same server-default domain.
func GenerateMessagingIngressHost(agentName, namespace, domain string) string {
	return GenerateIngestionIngressHost(agentName, namespace, "chat", domain)
}

// GenerateIngestionExternalURL generates the full external URL for an ingestion webhook
func GenerateIngestionExternalURL(agentName, namespace, ingestionName, domain string) string {
	host := GenerateIngestionIngressHost(agentName, namespace, ingestionName, domain)
	return fmt.Sprintf("https://%s", host)
}

// GenerateExternalURL generates the full external URL for an agent
func GenerateExternalURL(agentName, namespace, domain string) string {
	host := GenerateIngressHost(agentName, namespace, domain)
	return fmt.Sprintf("https://%s", host)
}

func stringPtr(s string) *string {
	return &s
}
