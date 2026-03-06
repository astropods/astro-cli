package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// TestBuildService verifies that BuildService produces a Service with the
// correct defaults (ClusterIP type, port 8080) and respects overrides (custom
// port, LoadBalancer type). Also checks that labels include astro.dev/agent
// and that the selector includes both agent and component labels.
func TestBuildService(t *testing.T) {
	tests := []struct {
		name string
		cfg  ServiceConfig
	}{
		{
			name: "default ClusterIP port 8080",
			cfg: ServiceConfig{
				Name:      "my-agent-agent",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
			},
		},
		{
			name: "custom port and LoadBalancer",
			cfg: ServiceConfig{
				Name:        "my-agent-agent",
				Namespace:   "default",
				AgentName:   "my-agent",
				BuildID:     "1.0",
				Component:   "agent",
				Port:        3000,
				ServiceType: corev1.ServiceTypeLoadBalancer,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := BuildService(tt.cfg)

			if svc.Kind != "Service" {
				t.Errorf("kind: expected Service, got %s", svc.Kind)
			}
			if svc.Name != tt.cfg.Name {
				t.Errorf("name: expected %s, got %s", tt.cfg.Name, svc.Name)
			}
			if svc.Namespace != tt.cfg.Namespace {
				t.Errorf("namespace: expected %s, got %s", tt.cfg.Namespace, svc.Namespace)
			}

			// Labels
			if svc.Labels["astro.dev/agent"] != tt.cfg.AgentName {
				t.Errorf("agent label: expected %s, got %s", tt.cfg.AgentName, svc.Labels["astro.dev/agent"])
			}

			// Selector
			if svc.Spec.Selector["astro.dev/agent"] != tt.cfg.AgentName {
				t.Errorf("selector: expected agent %s, got %s", tt.cfg.AgentName, svc.Spec.Selector["astro.dev/agent"])
			}
			if tt.cfg.Component != "" {
				if svc.Spec.Selector["app.kubernetes.io/component"] != tt.cfg.Component {
					t.Errorf("selector component: expected %s, got %s", tt.cfg.Component, svc.Spec.Selector["app.kubernetes.io/component"])
				}
			}
		})
	}

	t.Run("default values", func(t *testing.T) {
		svc := BuildService(tests[0].cfg)

		if svc.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Errorf("type: expected ClusterIP, got %s", svc.Spec.Type)
		}
		if svc.Spec.Ports[0].Port != 8080 {
			t.Errorf("port: expected 8080, got %d", svc.Spec.Ports[0].Port)
		}
	})

	t.Run("custom port and type", func(t *testing.T) {
		svc := BuildService(tests[1].cfg)

		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			t.Errorf("type: expected LoadBalancer, got %s", svc.Spec.Type)
		}
		if svc.Spec.Ports[0].Port != 3000 {
			t.Errorf("port: expected 3000, got %d", svc.Spec.Ports[0].Port)
		}
		if svc.Spec.Ports[0].TargetPort.IntValue() != 3000 {
			t.Errorf("target port: expected 3000, got %d", svc.Spec.Ports[0].TargetPort.IntValue())
		}
	})

	t.Run("port name and protocol", func(t *testing.T) {
		svc := BuildService(tests[0].cfg)

		p := svc.Spec.Ports[0]
		if p.Name != "http" {
			t.Errorf("port name: expected http, got %s", p.Name)
		}
		if p.Protocol != corev1.ProtocolTCP {
			t.Errorf("protocol: expected TCP, got %s", p.Protocol)
		}
	})
}
