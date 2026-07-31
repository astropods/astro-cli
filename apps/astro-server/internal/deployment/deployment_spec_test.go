package deployment

import (
	"testing"
)

func TestDefaultUpdateStrategy(t *testing.T) {
	s := DefaultUpdateStrategy()
	if s.Strategy != "rolling" {
		t.Errorf("expected rolling, got %s", s.Strategy)
	}
	if s.MaxUnavailable != "25%" {
		t.Errorf("expected 25%%, got %s", s.MaxUnavailable)
	}
	if s.MaxSurge != "25%" {
		t.Errorf("expected 25%%, got %s", s.MaxSurge)
	}
}

func TestDefaultStorageConfig(t *testing.T) {
	s := DefaultStorageConfig()
	if s.Size != "10Gi" {
		t.Errorf("expected 10Gi, got %s", s.Size)
	}
	if s.AccessMode != "ReadWriteOnce" {
		t.Errorf("expected ReadWriteOnce, got %s", s.AccessMode)
	}
	if s.Class != "" {
		t.Errorf("expected empty class, got %s", s.Class)
	}
}

func TestStandardResources(t *testing.T) {
	r := StandardResources
	if r.CPU != "100m" {
		t.Errorf("expected 100m, got %s", r.CPU)
	}
	if r.Memory != "1Gi" {
		t.Errorf("expected 1Gi, got %s", r.Memory)
	}
	if r.CPULimit != "100m" {
		t.Errorf("expected 100m, got %s", r.CPULimit)
	}
	if r.MemoryLimit != "1Gi" {
		t.Errorf("expected 1Gi, got %s", r.MemoryLimit)
	}
}

func TestGPUResources(t *testing.T) {
	r := GPUResources
	if r.CPU != "2" {
		t.Errorf("expected 2, got %s", r.CPU)
	}
	if r.Memory != "8Gi" {
		t.Errorf("expected 8Gi, got %s", r.Memory)
	}
}

func TestMessagingResources(t *testing.T) {
	r := MessagingResources
	if r.CPU != "100m" || r.CPULimit != "100m" {
		t.Errorf("expected request==limit at 100m, got %s/%s", r.CPU, r.CPULimit)
	}
	if r.Memory != "256Mi" || r.MemoryLimit != "256Mi" {
		t.Errorf("expected request==limit at 256Mi, got %s/%s", r.Memory, r.MemoryLimit)
	}
}

func TestCollectorResources(t *testing.T) {
	r := CollectorResources
	if r.CPU != "50m" || r.CPULimit != "50m" {
		t.Errorf("expected request==limit at 50m, got %s/%s", r.CPU, r.CPULimit)
	}
	if r.Memory != "128Mi" || r.MemoryLimit != "128Mi" {
		t.Errorf("expected request==limit at 128Mi, got %s/%s", r.Memory, r.MemoryLimit)
	}
}

func TestDeploymentInterfacesAuth_ParseYAML(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  account: acme
  name: my-agent
  build: abc123
  registry: registry.example.com
target:
  runtime: kubernetes
agent:
  image: registry.example.com/my-agent:abc123
  endpoints:
    http:
      port: 8080
  replicas: 1
  update:
    strategy: rolling
interfaces:
  adapters: [web]
  image: registry.example.com/messaging:latest
  endpoints:
    grpc:
      port: 9090
      protocol: grpc
    http:
      port: 8081
      protocol: http
      expose:
        enabled: true
  resources:
    cpu: "100m"
    memory: "128Mi"
  auth:
    web:
      type: oidc
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if ds.Interfaces == nil {
		t.Fatal("expected interfaces to be set")
	}
	if ds.Interfaces.Auth == nil {
		t.Fatal("expected auth to be set")
	}
	if ds.Interfaces.Auth.Web == nil {
		t.Fatal("expected auth.web to be set")
	}
	if ds.Interfaces.Auth.Web.Type != "oidc" {
		t.Errorf("expected auth.web.type oidc, got %s", ds.Interfaces.Auth.Web.Type)
	}
}

func TestDeploymentInterfacesAuth_NilWhenAbsent(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  account: acme
  name: my-agent
  build: abc123
  registry: registry.example.com
target:
  runtime: kubernetes
agent:
  image: registry.example.com/my-agent:abc123
  endpoints:
    http:
      port: 8080
  replicas: 1
  update:
    strategy: rolling
interfaces:
  adapters: [web]
  image: registry.example.com/messaging:latest
  endpoints:
    grpc:
      port: 9090
      protocol: grpc
    http:
      port: 8081
      protocol: http
      expose:
        enabled: true
  resources:
    cpu: "100m"
    memory: "128Mi"
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if ds.Interfaces == nil {
		t.Fatal("expected interfaces to be set")
	}
	if ds.Interfaces.Auth != nil {
		t.Errorf("expected auth to be nil when not specified, got %+v", ds.Interfaces.Auth)
	}
}
