package deployment

import (
	"fmt"
	"regexp"
	"strings"
)

// LabelKeyAgent is the Kubernetes label key used to identify the agent (account.agent format).
const LabelKeyAgent = "astro.dev/agent"

var (
	sanitizeRegex           = regexp.MustCompile(`[^a-z0-9-]`)
	consecutiveHyphensRegex = regexp.MustCompile(`-+`)
)

// SanitizeName sanitizes a name to be Kubernetes-compliant
// - Lowercase
// - Max 63 characters
// - Only alphanumeric and hyphens
// - Cannot start or end with hyphen
func SanitizeName(name string) string {
	// Convert to lowercase
	sanitized := strings.ToLower(name)

	// Replace underscores and dots with hyphens
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")

	// Remove invalid characters
	sanitized = sanitizeRegex.ReplaceAllString(sanitized, "")

	// Replace consecutive hyphens with single hyphen
	sanitized = consecutiveHyphensRegex.ReplaceAllString(sanitized, "-")

	// Trim hyphens from start and end
	sanitized = strings.Trim(sanitized, "-")

	// Truncate to 63 characters (K8s limit)
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		// Ensure it doesn't end with a hyphen after truncation
		sanitized = strings.TrimRight(sanitized, "-")
	}

	return sanitized
}

// GenerateResourceName generates a Kubernetes resource name
// Format: {agent}-{type}-{name}
func GenerateResourceName(agent, resourceType, name string) string {
	parts := []string{
		SanitizeName(agent),
		SanitizeName(resourceType),
		SanitizeName(name),
	}

	fullName := strings.Join(parts, "-")
	return SanitizeName(fullName)
}

// GenerateAgentResourceName generates a resource name for the main agent
// Format: {agent}-{type}
func GenerateAgentResourceName(agent, resourceType string) string {
	parts := []string{
		SanitizeName(agent),
		SanitizeName(resourceType),
	}

	fullName := strings.Join(parts, "-")
	return SanitizeName(fullName)
}

// GenerateSecretName generates the name for the secret that holds secret variable values.
// Format: {agent}-{version}-credentials
func GenerateSecretName(agent, version string) string {
	versionSanitized := strings.ReplaceAll(version, ".", "-")
	parts := []string{
		SanitizeName(agent),
		SanitizeName(versionSanitized),
		"credentials",
	}

	fullName := strings.Join(parts, "-")
	return SanitizeName(fullName)
}

// GenerateMessagingSecretName generates the name for the messaging-only Secret
// that holds the subset of secret values referenced from interfaces.environment.
// Format: {agent}-{version}-messaging-credentials
//
// Why this exists: the agent's main credentials Secret carries every secret in
// the deployment (LLM keys, knowledge store passwords, etc). Mounting it on
// the messaging sidecar via envFrom would leak all of those to the messaging
// process. We build a separate, narrower Secret with only the keys the
// messaging container actually needs (slack tokens, web auth secrets, etc).
func GenerateMessagingSecretName(agent, version string) string {
	versionSanitized := strings.ReplaceAll(version, ".", "-")
	parts := []string{
		SanitizeName(agent),
		SanitizeName(versionSanitized),
		"messaging",
		"credentials",
	}

	fullName := strings.Join(parts, "-")
	return SanitizeName(fullName)
}

// GenerateConfigMapName generates the name for configuration ConfigMap
// Format: {agent}-{version}-config
func GenerateConfigMapName(agent, version string) string {
	versionSanitized := strings.ReplaceAll(version, ".", "-")
	parts := []string{
		SanitizeName(agent),
		SanitizeName(versionSanitized),
		"config",
	}

	fullName := strings.Join(parts, "-")
	return SanitizeName(fullName)
}

// GenerateServiceDNS generates the internal DNS name for a service
// Format: {serviceName}.{namespace}.svc.cluster.local
func GenerateServiceDNS(serviceName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace)
}

// AgentLabelValue returns the account-qualified agent label value (account.agent).
// If account is empty, returns just the sanitized agent name.
func AgentLabelValue(account, agent string) string {
	if account == "" {
		return SanitizeName(agent)
	}
	return SanitizeName(account) + "." + SanitizeName(agent)
}

// GenerateLabels generates standard Kubernetes labels for resources
func GenerateLabels(account, agent, version, component string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       SanitizeName(agent),
		"app.kubernetes.io/instance":   SanitizeName(agent),
		"app.kubernetes.io/version":    SanitizeName(version),
		"app.kubernetes.io/managed-by": "astro-server",
		LabelKeyAgent:                  AgentLabelValue(account, agent),
	}

	if component != "" {
		labels["app.kubernetes.io/component"] = SanitizeName(component)
	}

	return labels
}

// GenerateSelector generates selector labels (subset of full labels)
func GenerateSelector(account, agent, component string) map[string]string {
	selector := map[string]string{
		"app.kubernetes.io/name":     SanitizeName(agent),
		"app.kubernetes.io/instance": SanitizeName(agent),
		LabelKeyAgent:                AgentLabelValue(account, agent),
	}

	if component != "" {
		selector["app.kubernetes.io/component"] = SanitizeName(component)
	}

	return selector
}
