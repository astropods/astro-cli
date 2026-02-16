package k8s

import (
	"fmt"
	"testing"

	"github.com/postman/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestBuildDeployment verifies BuildDeployment across a range of configurations:
// minimal defaults (port 8080, PullAlways, standard resources 100m/256Mi, replicas=1,
// no probes), GPU enabled (nvidia.com/gpu:1, scaled CPU/memory 2/8Gi→4/16Gi,
// node selector), ConfigMap+Secret envFrom refs, container environment variables,
// custom exec healthcheck command, HTTP path healthcheck without provider,
// provider-specific healthchecks (redis→Exec "redis-cli ping", qdrant→HTTPGet
// /healthz:6333), custom probe timing (interval/timeout/retries), and custom port.
func TestBuildDeployment(t *testing.T) {
	tests := []struct {
		name  string
		cfg   DeploymentConfig
		check func(t *testing.T, d *DeploymentConfig)
	}{
		{
			name: "minimal config - defaults",
			cfg: DeploymentConfig{
				Name:      "test-deploy",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{
					Image: "my-agent:latest",
				},
			},
		},
		{
			name: "GPU enabled",
			cfg: DeploymentConfig{
				Name:      "gpu-deploy",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "model",
				Container: spec.ContainerConfig{
					Image: "model:latest",
					GPU:   &spec.GPUConfig{VRAM: "24Gi", Runtime: "cuda"},
				},
			},
		},
		{
			name: "with ConfigMap and Secret",
			cfg: DeploymentConfig{
				Name:          "ref-deploy",
				Namespace:     "default",
				AgentName:     "my-agent",
				BuildID:       "1.0",
				Component:     "agent",
				Container:     spec.ContainerConfig{Image: "agent:latest"},
				ConfigMapName: "my-agent-1-0-config",
				SecretName:    "my-agent-1-0-credentials",
			},
		},
		{
			name: "with container env vars",
			cfg: DeploymentConfig{
				Name:      "env-deploy",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{
					Image: "agent:latest",
					Environment: map[string]string{
						"FOO": "bar",
						"BAZ": "qux",
					},
				},
			},
		},
		{
			name: "healthcheck custom test command",
			cfg: DeploymentConfig{
				Name:      "hc-exec",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{Image: "agent:latest"},
				Healthcheck: &spec.Healthcheck{
					Test: []string{"CMD", "curl", "-f", "http://localhost:8080/health"},
				},
			},
		},
		{
			name: "healthcheck HTTP path no provider",
			cfg: DeploymentConfig{
				Name:      "hc-http",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{Image: "agent:latest"},
				Healthcheck: &spec.Healthcheck{
					Path: "/health",
				},
			},
		},
		{
			name: "healthcheck provider redis",
			cfg: DeploymentConfig{
				Name:      "hc-redis",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "knowledge-cache",
				Container: spec.ContainerConfig{Image: "redis:latest"},
				Provider:  "redis",
				Healthcheck: &spec.Healthcheck{},
			},
		},
		{
			name: "healthcheck provider qdrant",
			cfg: DeploymentConfig{
				Name:      "hc-qdrant",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "knowledge-vectors",
				Container: spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
				Provider:  "qdrant",
				Port:      6333,
				Healthcheck: &spec.Healthcheck{},
			},
		},
		{
			name: "healthcheck custom interval/timeout/retries",
			cfg: DeploymentConfig{
				Name:      "hc-custom-timing",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{Image: "agent:latest"},
				Healthcheck: &spec.Healthcheck{
					Test:     []string{"CMD", "true"},
					Interval: "30s",
					Timeout:  "10s",
					Retries:  5,
				},
			},
		},
		{
			name: "custom port",
			cfg: DeploymentConfig{
				Name:      "custom-port",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "agent",
				Container: spec.ContainerConfig{Image: "agent:latest"},
				Port:      3000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := BuildDeployment(tt.cfg)

			// Common checks
			if d.Name != tt.cfg.Name {
				t.Errorf("name: expected %s, got %s", tt.cfg.Name, d.Name)
			}
			if d.Namespace != tt.cfg.Namespace {
				t.Errorf("namespace: expected %s, got %s", tt.cfg.Namespace, d.Namespace)
			}
			if d.Kind != "Deployment" {
				t.Errorf("kind: expected Deployment, got %s", d.Kind)
			}
			if *d.Spec.Replicas != 1 {
				t.Errorf("replicas: expected 1, got %d", *d.Spec.Replicas)
			}

			container := d.Spec.Template.Spec.Containers[0]
			if container.Name != "app" {
				t.Errorf("container name: expected app, got %s", container.Name)
			}
			if container.Image != tt.cfg.Container.Image {
				t.Errorf("image: expected %s, got %s", tt.cfg.Container.Image, container.Image)
			}
			if container.ImagePullPolicy != corev1.PullAlways {
				t.Errorf("pull policy: expected Always, got %s", container.ImagePullPolicy)
			}

			// Labels should be present
			if d.Labels == nil {
				t.Fatal("expected labels on deployment")
			}
			if d.Labels["astro.dev/agent"] != tt.cfg.AgentName {
				t.Errorf("agent label: expected %s, got %s", tt.cfg.AgentName, d.Labels["astro.dev/agent"])
			}
		})
	}

	// Specific assertions per case
	t.Run("minimal defaults check", func(t *testing.T) {
		cfg := tests[0].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		// Default port 8080
		if len(container.Ports) == 0 || container.Ports[0].ContainerPort != 8080 {
			t.Errorf("expected default port 8080, got %v", container.Ports)
		}

		// Standard resources
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		if cpuReq.Cmp(resource.MustParse("100m")) != 0 {
			t.Errorf("expected CPU request 100m, got %s", cpuReq.String())
		}
		memReq := container.Resources.Requests[corev1.ResourceMemory]
		if memReq.Cmp(resource.MustParse("256Mi")) != 0 {
			t.Errorf("expected memory request 256Mi, got %s", memReq.String())
		}

		// No probes when no healthcheck
		if container.LivenessProbe != nil {
			t.Error("expected no liveness probe without healthcheck")
		}
	})

	t.Run("GPU resources check", func(t *testing.T) {
		cfg := tests[1].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		gpuReq := container.Resources.Requests["nvidia.com/gpu"]
		if gpuReq.Cmp(resource.MustParse("1")) != 0 {
			t.Errorf("expected GPU request 1, got %s", gpuReq.String())
		}

		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		if cpuReq.Cmp(resource.MustParse("2")) != 0 {
			t.Errorf("expected GPU CPU request 2, got %s", cpuReq.String())
		}
		memLim := container.Resources.Limits[corev1.ResourceMemory]
		if memLim.Cmp(resource.MustParse("16Gi")) != 0 {
			t.Errorf("expected GPU memory limit 16Gi, got %s", memLim.String())
		}

		// GPU node selector derived from runtime
		ns := d.Spec.Template.Spec.NodeSelector
		if ns == nil || ns["accelerator"] != "nvidia-gpu" {
			t.Errorf("expected GPU node selector nvidia-gpu, got %v", ns)
		}
	})

	t.Run("ConfigMap and Secret refs check", func(t *testing.T) {
		cfg := tests[2].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if len(container.EnvFrom) != 2 {
			t.Fatalf("expected 2 envFrom entries, got %d", len(container.EnvFrom))
		}
		if container.EnvFrom[0].ConfigMapRef == nil || container.EnvFrom[0].ConfigMapRef.Name != cfg.ConfigMapName {
			t.Errorf("expected ConfigMapRef %s", cfg.ConfigMapName)
		}
		if container.EnvFrom[1].SecretRef == nil || container.EnvFrom[1].SecretRef.Name != cfg.SecretName {
			t.Errorf("expected SecretRef %s", cfg.SecretName)
		}
	})

	t.Run("container env vars check", func(t *testing.T) {
		cfg := tests[3].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		envMap := make(map[string]string)
		for _, e := range container.Env {
			envMap[e.Name] = e.Value
		}
		for k, v := range cfg.Container.Environment {
			if envMap[k] != v {
				t.Errorf("env %s: expected %s, got %s", k, v, envMap[k])
			}
		}
	})

	t.Run("healthcheck exec probe check", func(t *testing.T) {
		cfg := tests[4].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.Exec == nil {
			t.Fatal("expected exec probe")
		}
		if len(container.LivenessProbe.Exec.Command) != len(cfg.Healthcheck.Test) {
			t.Errorf("expected %d commands, got %d", len(cfg.Healthcheck.Test), len(container.LivenessProbe.Exec.Command))
		}
		// Defaults
		if container.LivenessProbe.InitialDelaySeconds != 10 {
			t.Errorf("expected initial delay 10, got %d", container.LivenessProbe.InitialDelaySeconds)
		}
	})

	t.Run("healthcheck HTTP path check", func(t *testing.T) {
		cfg := tests[5].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("expected HTTPGet probe")
		}
		if container.LivenessProbe.HTTPGet.Path != "/health" {
			t.Errorf("expected path /health, got %s", container.LivenessProbe.HTTPGet.Path)
		}
	})

	t.Run("healthcheck redis provider check", func(t *testing.T) {
		cfg := tests[6].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.Exec == nil {
			t.Fatal("expected exec probe for redis")
		}
		cmd := container.LivenessProbe.Exec.Command
		if len(cmd) < 2 || cmd[0] != "redis-cli" || cmd[1] != "ping" {
			t.Errorf("expected [redis-cli ping], got %v", cmd)
		}
	})

	t.Run("healthcheck qdrant provider check", func(t *testing.T) {
		cfg := tests[7].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("expected HTTPGet probe for qdrant")
		}
		if container.LivenessProbe.HTTPGet.Path != "/healthz" {
			t.Errorf("expected path /healthz, got %s", container.LivenessProbe.HTTPGet.Path)
		}
		if container.LivenessProbe.HTTPGet.Port.IntValue() != 6333 {
			t.Errorf("expected port 6333, got %d", container.LivenessProbe.HTTPGet.Port.IntValue())
		}
	})

	t.Run("custom timing probe check", func(t *testing.T) {
		cfg := tests[8].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.PeriodSeconds != 30 {
			t.Errorf("expected period 30s, got %d", container.LivenessProbe.PeriodSeconds)
		}
		if container.LivenessProbe.TimeoutSeconds != 10 {
			t.Errorf("expected timeout 10s, got %d", container.LivenessProbe.TimeoutSeconds)
		}
		if container.LivenessProbe.FailureThreshold != 5 {
			t.Errorf("expected failure threshold 5, got %d", container.LivenessProbe.FailureThreshold)
		}
	})

	t.Run("custom port check", func(t *testing.T) {
		cfg := tests[9].cfg
		d := BuildDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.Ports[0].ContainerPort != 3000 {
			t.Errorf("expected port 3000, got %d", container.Ports[0].ContainerPort)
		}
	})
}

// TestBuildMessagingDeployment verifies BuildMessagingDeployment for messaging
// sidecar configurations: slack interface (port 9090, SLACK_ENABLED/GRPC_ENABLED
// env vars, secret envFrom), web adapter enabled (adds http/8080 port,
// WEB_ENABLED=true), and without secret (no envFrom entries).
func TestBuildMessagingDeployment(t *testing.T) {
	tests := []struct {
		name string
		cfg  MessagingDeploymentConfig
	}{
		{
			name: "slack interface",
			cfg: MessagingDeploymentConfig{
				Name:          "msg-slack",
				Namespace:     "default",
				AgentName:     "my-agent",
				BuildID:       "1.0",
				Component:     "messaging-slack",
				Image:         "messaging:latest",
				InterfaceType: "slack",
				SecretName:    "my-secret",
			},
		},
		{
			name: "web enabled",
			cfg: MessagingDeploymentConfig{
				Name:          "msg-web",
				Namespace:     "default",
				AgentName:     "my-agent",
				BuildID:       "1.0",
				Component:     "messaging-web",
				Image:         "messaging:latest",
				InterfaceType: "slack",
				WebEnabled:    true,
			},
		},
		{
			name: "without secret",
			cfg: MessagingDeploymentConfig{
				Name:          "msg-nosecret",
				Namespace:     "default",
				AgentName:     "my-agent",
				BuildID:       "1.0",
				Component:     "messaging",
				Image:         "messaging:latest",
				InterfaceType: "slack",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := BuildMessagingDeployment(tt.cfg)

			if d.Name != tt.cfg.Name {
				t.Errorf("name: expected %s, got %s", tt.cfg.Name, d.Name)
			}
			if d.Kind != "Deployment" {
				t.Errorf("kind: expected Deployment, got %s", d.Kind)
			}

			container := d.Spec.Template.Spec.Containers[0]
			if container.Name != "messaging" {
				t.Errorf("container name: expected messaging, got %s", container.Name)
			}
		})
	}

	t.Run("slack env vars", func(t *testing.T) {
		d := BuildMessagingDeployment(tests[0].cfg)
		container := d.Spec.Template.Spec.Containers[0]

		// Default port 9090
		if container.Ports[0].ContainerPort != 9090 {
			t.Errorf("expected port 9090, got %d", container.Ports[0].ContainerPort)
		}

		envMap := make(map[string]string)
		for _, e := range container.Env {
			envMap[e.Name] = e.Value
		}

		for _, key := range []string{"SLACK_ENABLED", "GRPC_ENABLED", "SLACK_SOCKET_MODE"} {
			if envMap[key] != "true" {
				t.Errorf("expected %s=true, got %q", key, envMap[key])
			}
		}

		// Secret ref should be present
		if len(container.EnvFrom) == 0 || container.EnvFrom[0].SecretRef == nil {
			t.Error("expected secret envFrom")
		}
	})

	t.Run("web enabled adds http port", func(t *testing.T) {
		d := BuildMessagingDeployment(tests[1].cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if len(container.Ports) != 2 {
			t.Fatalf("expected 2 ports (grpc + http), got %d", len(container.Ports))
		}

		envMap := make(map[string]string)
		for _, e := range container.Env {
			envMap[e.Name] = e.Value
		}
		if envMap["WEB_ENABLED"] != "true" {
			t.Errorf("expected WEB_ENABLED=true, got %q", envMap["WEB_ENABLED"])
		}

		// Find the http port
		foundHTTP := false
		for _, p := range container.Ports {
			if p.Name == "http" && p.ContainerPort == 8080 {
				foundHTTP = true
			}
		}
		if !foundHTTP {
			t.Error("expected http port 8080")
		}
	})

	t.Run("without secret - no envFrom", func(t *testing.T) {
		d := BuildMessagingDeployment(tests[2].cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if len(container.EnvFrom) != 0 {
			t.Errorf("expected no envFrom without secret, got %d", len(container.EnvFrom))
		}
	})
}

// TestBuildCollectorDeployment verifies that BuildCollectorDeployment produces
// a Deployment with the collector container exposing OTLP gRPC (4317) and HTTP
// (4318) ports, ConfigMap envFrom, Galileo API key and project env vars, and
// lightweight resource limits (50m/128Mi → 250m/256Mi).
func TestBuildCollectorDeployment(t *testing.T) {
	t.Run("full config", func(t *testing.T) {
		cfg := CollectorDeploymentConfig{
			Name:            "my-agent-collector",
			Namespace:       "default",
			AgentName:       "my-agent",
			BuildID:         "1.0",
			Component:       "collector",
			Image:           "collector:latest",
			ConfigMapName:   "my-agent-1-0-config",
			GalileoAPIKey:   "gal-key-123",
			GalileoProject:  "my-project",
		}

		d := BuildCollectorDeployment(cfg)

		if d.Kind != "Deployment" {
			t.Errorf("kind: expected Deployment, got %s", d.Kind)
		}
		if d.Name != cfg.Name {
			t.Errorf("name: expected %s, got %s", cfg.Name, d.Name)
		}
		if d.Namespace != cfg.Namespace {
			t.Errorf("namespace: expected %s, got %s", cfg.Namespace, d.Namespace)
		}
		if *d.Spec.Replicas != 1 {
			t.Errorf("replicas: expected 1, got %d", *d.Spec.Replicas)
		}

		container := d.Spec.Template.Spec.Containers[0]
		if container.Name != "collector" {
			t.Errorf("container name: expected collector, got %s", container.Name)
		}
		if container.Image != cfg.Image {
			t.Errorf("image: expected %s, got %s", cfg.Image, container.Image)
		}
		if container.ImagePullPolicy != corev1.PullAlways {
			t.Errorf("pull policy: expected Always, got %s", container.ImagePullPolicy)
		}

		// Ports: OTLP gRPC (4317) and HTTP (4318)
		if len(container.Ports) != 2 {
			t.Fatalf("expected 2 ports, got %d", len(container.Ports))
		}
		portMap := make(map[string]int32)
		for _, p := range container.Ports {
			portMap[p.Name] = p.ContainerPort
		}
		if portMap["otlp-grpc"] != 4317 {
			t.Errorf("expected otlp-grpc port 4317, got %d", portMap["otlp-grpc"])
		}
		if portMap["otlp-http"] != 4318 {
			t.Errorf("expected otlp-http port 4318, got %d", portMap["otlp-http"])
		}

		// ConfigMap envFrom
		if len(container.EnvFrom) != 1 || container.EnvFrom[0].ConfigMapRef == nil {
			t.Fatal("expected ConfigMapRef in envFrom")
		}
		if container.EnvFrom[0].ConfigMapRef.Name != cfg.ConfigMapName {
			t.Errorf("configmap ref: expected %s, got %s", cfg.ConfigMapName, container.EnvFrom[0].ConfigMapRef.Name)
		}

		// Galileo env vars
		envMap := make(map[string]string)
		for _, e := range container.Env {
			envMap[e.Name] = e.Value
		}
		if envMap["GALILEO_API_KEY"] != cfg.GalileoAPIKey {
			t.Errorf("expected GALILEO_API_KEY=%s, got %s", cfg.GalileoAPIKey, envMap["GALILEO_API_KEY"])
		}
		if envMap["GALILEO_PROJECT"] != cfg.GalileoProject {
			t.Errorf("expected GALILEO_PROJECT=%s, got %s", cfg.GalileoProject, envMap["GALILEO_PROJECT"])
		}

		// Resources
		cpuReq := container.Resources.Requests[corev1.ResourceCPU]
		if cpuReq.Cmp(resource.MustParse("50m")) != 0 {
			t.Errorf("expected CPU request 50m, got %s", cpuReq.String())
		}
		memLim := container.Resources.Limits[corev1.ResourceMemory]
		if memLim.Cmp(resource.MustParse("256Mi")) != 0 {
			t.Errorf("expected memory limit 256Mi, got %s", memLim.String())
		}
	})

	t.Run("without optional fields", func(t *testing.T) {
		cfg := CollectorDeploymentConfig{
			Name:      "collector-minimal",
			Namespace: "default",
			AgentName: "my-agent",
			BuildID:   "1.0",
			Component: "collector",
			Image:     "collector:latest",
		}

		d := BuildCollectorDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		// No configmap envFrom
		if len(container.EnvFrom) != 0 {
			t.Errorf("expected no envFrom, got %d", len(container.EnvFrom))
		}

		// No Galileo env vars
		if len(container.Env) != 0 {
			t.Errorf("expected no env vars, got %d", len(container.Env))
		}
	})

	t.Run("custom ImagePullPolicy", func(t *testing.T) {
		cfg := CollectorDeploymentConfig{
			Name:            "collector-ifnotpresent",
			Namespace:       "default",
			AgentName:       "my-agent",
			BuildID:         "1.0",
			Component:       "collector",
			Image:           "collector:v1.2.3",
			ImagePullPolicy: corev1.PullIfNotPresent,
		}

		d := BuildCollectorDeployment(cfg)
		container := d.Spec.Template.Spec.Containers[0]

		if container.ImagePullPolicy != corev1.PullIfNotPresent {
			t.Errorf("expected IfNotPresent, got %s", container.ImagePullPolicy)
		}
	})
}

// TestParsePort verifies that ParsePort correctly parses int, int32, int64,
// and string port values, and returns errors for invalid strings and unsupported types.
func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    int32
		wantErr bool
	}{
		{"int", 8080, 8080, false},
		{"int32", int32(3000), 3000, false},
		{"int64", int64(9090), 9090, false},
		{"string", "8080", 8080, false},
		{"invalid string", "not-a-port", 0, true},
		{"unsupported type float64", float64(8080), 0, true},
		{"unsupported type bool", true, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePort(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %v (%T)", tt.input, tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ParsePort(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestBuildProbeHandlerEdgeCases verifies buildProbeHandler for edge cases:
// unknown provider with no path returns nil, provider with health path and
// port=0 uses provider default port, and path-based fallback with port=0
// defaults to 8080.
func TestBuildProbeHandlerEdgeCases(t *testing.T) {
	t.Run("unknown provider no path returns nil", func(t *testing.T) {
		handler := buildProbeHandler("unknown", 8080, "")
		if handler != nil {
			t.Errorf("expected nil handler for unknown provider with no path, got %+v", handler)
		}
	})

	t.Run("qdrant provider port 0 uses default", func(t *testing.T) {
		handler := buildProbeHandler("qdrant", 0, "")
		if handler == nil {
			t.Fatal("expected handler for qdrant provider")
		}
		if handler.HTTPGet == nil {
			t.Fatal("expected HTTPGet handler for qdrant")
		}
		if handler.HTTPGet.Port.IntValue() != 6333 {
			t.Errorf("expected default port 6333, got %d", handler.HTTPGet.Port.IntValue())
		}
	})

	t.Run("path fallback port 0 defaults to 8080", func(t *testing.T) {
		handler := buildProbeHandler("", 0, "/health")
		if handler == nil {
			t.Fatal("expected handler for path fallback")
		}
		if handler.HTTPGet == nil {
			t.Fatal("expected HTTPGet handler for path fallback")
		}
		if handler.HTTPGet.Port.IntValue() != 8080 {
			t.Errorf("expected default port 8080, got %d", handler.HTTPGet.Port.IntValue())
		}
		if handler.HTTPGet.Path != "/health" {
			t.Errorf("expected path /health, got %s", handler.HTTPGet.Path)
		}
	})
}

// TestEncodeSecretData verifies that EncodeSecretData base64-encodes values correctly.
func TestEncodeSecretData(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-ant-test", "c2stYW50LXRlc3Q="},
		{"", ""},
		{"hello world", "aGVsbG8gd29ybGQ="},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("encode %q", tt.input), func(t *testing.T) {
			got := EncodeSecretData(tt.input)
			if got != tt.want {
				t.Errorf("EncodeSecretData(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
