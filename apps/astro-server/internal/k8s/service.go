package k8s

import (
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceConfig holds configuration for building a Service
type ServiceConfig struct {
	Name        string
	Namespace   string
	AgentName   string
	BuildID     string
	Component   string
	Port        int32
	ServiceType corev1.ServiceType // ClusterIP or LoadBalancer
}

// BuildService creates a Kubernetes Service manifest
func BuildService(cfg ServiceConfig) *corev1.Service {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	serviceType := cfg.ServiceType
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}

	ports := []corev1.ServicePort{
		{
			Name:       "http",
			Protocol:   corev1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
		},
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: selector,
			Ports:    ports,
		},
	}

	return service
}
