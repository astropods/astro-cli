package k8s

import (
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressConfig holds configuration for building an Ingress
type IngressConfig struct {
	Name              string
	Namespace         string
	AgentName         string
	Version           string
	Component         string
	ServiceName       string
	ServicePort       int32
	Host              string // Full hostname (e.g., agent-name-namespace.agents.odesdaz.com)
	ACMCertificateARN string
	ALBGroupName      string
}

// BuildIngress creates a Kubernetes Ingress manifest for AWS ALB
func BuildIngress(cfg IngressConfig) *networkingv1.Ingress {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	pathType := networkingv1.PathTypePrefix

	annotations := map[string]string{
		// ALB Ingress Controller annotations
		"alb.ingress.kubernetes.io/scheme":        "internet-facing",
		"alb.ingress.kubernetes.io/target-type":   "ip",
		"alb.ingress.kubernetes.io/listen-ports":  `[{"HTTPS":443}]`,
		"alb.ingress.kubernetes.io/ssl-redirect":  "443",
		// external-dns annotation for automatic DNS record creation
		"external-dns.alpha.kubernetes.io/hostname": cfg.Host,
	}

	// Add certificate ARN if provided
	if cfg.ACMCertificateARN != "" {
		annotations["alb.ingress.kubernetes.io/certificate-arn"] = cfg.ACMCertificateARN
	}

	// Add group name to share ALB across ingresses
	if cfg.ALBGroupName != "" {
		annotations["alb.ingress.kubernetes.io/group.name"] = cfg.ALBGroupName
	}

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cfg.Name,
			Namespace:   cfg.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("alb"),
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
// Format: <agent-name>-<namespace>.domain
func GenerateIngressHost(agentName, namespace, domain string) string {
	return fmt.Sprintf("%s-%s.%s", agentName, namespace, domain)
}

// GenerateExternalURL generates the full external URL for an agent
func GenerateExternalURL(agentName, namespace, domain string) string {
	host := GenerateIngressHost(agentName, namespace, domain)
	return fmt.Sprintf("https://%s", host)
}

func stringPtr(s string) *string {
	return &s
}
