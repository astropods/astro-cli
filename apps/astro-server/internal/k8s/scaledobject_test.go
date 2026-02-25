package k8s

import (
	"strings"
	"testing"
)

func TestBuildHTTPScaledObject(t *testing.T) {
	cfg := HTTPScaledObjectConfig{
		Name:           "my-agent-scaler-messaging",
		Namespace:      "agent-ns",
		AgentName:      "my-agent",
		BuildID:        "build-123",
		Component:      "messaging",
		Host:           "my-agent-abc.agents.example.com",
		DeploymentName: "my-agent-messaging",
		ServiceName:    "my-agent-messaging",
		ServicePort:    8080,
	}

	obj := BuildHTTPScaledObject(cfg)

	if obj.GetAPIVersion() != "http.keda.sh/v1alpha1" {
		t.Errorf("apiVersion: expected http.keda.sh/v1alpha1, got %s", obj.GetAPIVersion())
	}
	if obj.GetKind() != "HTTPScaledObject" {
		t.Errorf("kind: expected HTTPScaledObject, got %s", obj.GetKind())
	}
	if obj.GetName() != cfg.Name {
		t.Errorf("name: expected %s, got %s", cfg.Name, obj.GetName())
	}
	if obj.GetNamespace() != cfg.Namespace {
		t.Errorf("namespace: expected %s, got %s", cfg.Namespace, obj.GetNamespace())
	}

	// Labels
	labels := obj.GetLabels()
	if labels["astro.dev/agent"] != cfg.AgentName {
		t.Errorf("agent label: expected %s, got %s", cfg.AgentName, labels["astro.dev/agent"])
	}

	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("spec field missing or wrong type")
	}

	// Hosts
	hosts, ok := spec["hosts"].([]any)
	if !ok || len(hosts) != 1 || hosts[0] != cfg.Host {
		t.Errorf("hosts: expected [%s], got %v", cfg.Host, hosts)
	}

	// scaleTargetRef
	ref, ok := spec["scaleTargetRef"].(map[string]any)
	if !ok {
		t.Fatal("scaleTargetRef missing")
	}
	if ref["name"] != cfg.DeploymentName {
		t.Errorf("scaleTargetRef.name: expected %s, got %v", cfg.DeploymentName, ref["name"])
	}
	if ref["service"] != cfg.ServiceName {
		t.Errorf("scaleTargetRef.service: expected %s, got %v", cfg.ServiceName, ref["service"])
	}
	if ref["port"] != int64(cfg.ServicePort) {
		t.Errorf("scaleTargetRef.port: expected %d, got %v", cfg.ServicePort, ref["port"])
	}

	// Replicas: min 0, max 1
	replicas, ok := spec["replicas"].(map[string]any)
	if !ok {
		t.Fatal("replicas missing")
	}
	if replicas["min"] != int64(0) {
		t.Errorf("replicas.min: expected 0, got %v", replicas["min"])
	}
	if replicas["max"] != int64(1) {
		t.Errorf("replicas.max: expected 1, got %v", replicas["max"])
	}
}

func TestBuildWorkloadScaledObject(t *testing.T) {
	cfg := WorkloadScaledObjectConfig{
		Name:           "my-agent-scaler-agent",
		Namespace:      "agent-ns",
		AgentName:      "my-agent",
		BuildID:        "build-123",
		Component:      "agent",
		DeploymentName: "my-agent-agent",
		PodSelector:    "app.kubernetes.io/name=my-agent,app.kubernetes.io/component=messaging",
	}

	obj := BuildWorkloadScaledObject(cfg)

	if obj.GetAPIVersion() != "keda.sh/v1alpha1" {
		t.Errorf("apiVersion: expected keda.sh/v1alpha1, got %s", obj.GetAPIVersion())
	}
	if obj.GetKind() != "ScaledObject" {
		t.Errorf("kind: expected ScaledObject, got %s", obj.GetKind())
	}
	if obj.GetName() != cfg.Name {
		t.Errorf("name: expected %s, got %s", cfg.Name, obj.GetName())
	}

	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("spec missing")
	}

	// scaleTargetRef
	ref, ok := spec["scaleTargetRef"].(map[string]any)
	if !ok {
		t.Fatal("scaleTargetRef missing")
	}
	if ref["name"] != cfg.DeploymentName {
		t.Errorf("scaleTargetRef.name: expected %s, got %v", cfg.DeploymentName, ref["name"])
	}

	if spec["minReplicaCount"] != int64(0) {
		t.Errorf("minReplicaCount: expected 0, got %v", spec["minReplicaCount"])
	}
	if spec["maxReplicaCount"] != int64(1) {
		t.Errorf("maxReplicaCount: expected 1, got %v", spec["maxReplicaCount"])
	}

	// Trigger type and podSelector
	triggers, ok := spec["triggers"].([]any)
	if !ok || len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %v", spec["triggers"])
	}
	trigger := triggers[0].(map[string]any)
	if trigger["type"] != "kubernetes-workload" {
		t.Errorf("trigger type: expected kubernetes-workload, got %v", trigger["type"])
	}
	meta := trigger["metadata"].(map[string]any)
	if meta["podSelector"] != cfg.PodSelector {
		t.Errorf("podSelector: expected %s, got %v", cfg.PodSelector, meta["podSelector"])
	}
	if meta["value"] != "1" {
		t.Errorf("value: expected 1, got %v", meta["value"])
	}
}

func TestMessagingPodSelector(t *testing.T) {
	sel := messagingPodSelector("my-agent")

	if !strings.Contains(sel, "my-agent") {
		t.Errorf("selector should contain agent name, got %s", sel)
	}
	if !strings.Contains(sel, "messaging") {
		t.Errorf("selector should contain component=messaging, got %s", sel)
	}
	if !strings.Contains(sel, "app.kubernetes.io/name") {
		t.Errorf("selector should use app.kubernetes.io/name label, got %s", sel)
	}
}

func TestBuildInterceptorIngress(t *testing.T) {
	cfg := IngressConfig{
		Name:              "my-agent-ingress-messaging",
		Namespace:         "agent-ns",
		AgentName:         "my-agent",
		BuildID:           "build-123",
		Component:         "messaging",
		ServiceName:       "my-agent-messaging",
		ServicePort:       8080,
		Host:              "my-agent-abc.agents.example.com",
		ACMCertificateARN: "arn:aws:acm:us-east-1:123:cert",
		ALBGroupName:      "shared-alb",
	}

	ing := BuildInterceptorIngress(cfg)

	// Must be in keda namespace
	if ing.Namespace != kedaNamespace {
		t.Errorf("namespace: expected %s, got %s", kedaNamespace, ing.Namespace)
	}

	// Backend must point to interceptor, not the original service
	paths := ing.Spec.Rules[0].HTTP.Paths
	if paths[0].Backend.Service.Name != interceptorService {
		t.Errorf("backend service: expected %s, got %s", interceptorService, paths[0].Backend.Service.Name)
	}
	if paths[0].Backend.Service.Port.Number != int32(interceptorPort) {
		t.Errorf("backend port: expected %d, got %d", interceptorPort, paths[0].Backend.Service.Port.Number)
	}

	// Host and ALB config must be preserved from original
	if ing.Spec.Rules[0].Host != cfg.Host {
		t.Errorf("host: expected %s, got %s", cfg.Host, ing.Spec.Rules[0].Host)
	}
	if ing.Annotations["alb.ingress.kubernetes.io/group.name"] != cfg.ALBGroupName {
		t.Errorf("ALB group name not preserved")
	}
	if ing.Annotations["external-dns.alpha.kubernetes.io/hostname"] != cfg.Host {
		t.Errorf("external-dns hostname not preserved")
	}
}
