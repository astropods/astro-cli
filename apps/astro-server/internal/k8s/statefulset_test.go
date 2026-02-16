package k8s

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestBuildStatefulSet verifies BuildStatefulSet across provider-specific
// configurations: qdrant (port 6333, extra gRPC 6334, mount /qdrant/storage,
// default PVC 10Gi), redis (port 6379, mount /data), postgres (port 5432,
// mount /var/lib/postgresql/data), custom storage size (20Gi), provider-aware
// healthchecks (qdrant→HTTPGet /healthz, redis→Exec "redis-cli ping"), and
// ConfigMap+Secret envFrom refs.
func TestBuildStatefulSet(t *testing.T) {
	tests := []struct {
		name string
		cfg  StatefulSetConfig
	}{
		{
			name: "qdrant provider",
			cfg: StatefulSetConfig{
				Name:      "agent-knowledge-vectors",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "knowledge-vectors",
				Container: spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
				Provider:  "qdrant",
			},
		},
		{
			name: "redis provider",
			cfg: StatefulSetConfig{
				Name:      "agent-knowledge-cache",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "knowledge-cache",
				Container: spec.ContainerConfig{Image: "redis:latest"},
				Provider:  "redis",
			},
		},
		{
			name: "postgres provider",
			cfg: StatefulSetConfig{
				Name:      "agent-knowledge-db",
				Namespace: "default",
				AgentName: "my-agent",
				BuildID:   "1.0",
				Component: "knowledge-db",
				Container: spec.ContainerConfig{Image: "postgres:latest"},
				Provider:  "postgres",
			},
		},
		{
			name: "custom storage 20Gi",
			cfg: StatefulSetConfig{
				Name:        "agent-knowledge-big",
				Namespace:   "default",
				AgentName:   "my-agent",
				BuildID:     "1.0",
				Component:   "knowledge-big",
				Container:   spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
				Provider:    "qdrant",
				StorageSize: "20Gi",
			},
		},
		{
			name: "with healthcheck qdrant",
			cfg: StatefulSetConfig{
				Name:        "agent-knowledge-hc",
				Namespace:   "default",
				AgentName:   "my-agent",
				BuildID:     "1.0",
				Component:   "knowledge-hc",
				Container:   spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
				Provider:    "qdrant",
				Healthcheck: &spec.Healthcheck{},
			},
		},
		{
			name: "with healthcheck redis",
			cfg: StatefulSetConfig{
				Name:        "agent-knowledge-hcr",
				Namespace:   "default",
				AgentName:   "my-agent",
				BuildID:     "1.0",
				Component:   "knowledge-hcr",
				Container:   spec.ContainerConfig{Image: "redis:latest"},
				Provider:    "redis",
				Healthcheck: &spec.Healthcheck{},
			},
		},
		{
			name: "with ConfigMap and Secret",
			cfg: StatefulSetConfig{
				Name:          "agent-knowledge-refs",
				Namespace:     "default",
				AgentName:     "my-agent",
				BuildID:       "1.0",
				Component:     "knowledge-refs",
				Container:     spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
				Provider:      "qdrant",
				ConfigMapName: "my-config",
				SecretName:    "my-secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := BuildStatefulSet(tt.cfg)

			if ss.Name != tt.cfg.Name {
				t.Errorf("name: expected %s, got %s", tt.cfg.Name, ss.Name)
			}
			if ss.Kind != "StatefulSet" {
				t.Errorf("kind: expected StatefulSet, got %s", ss.Kind)
			}
			if *ss.Spec.Replicas != 1 {
				t.Errorf("replicas: expected 1, got %d", *ss.Spec.Replicas)
			}
			if ss.Spec.ServiceName != tt.cfg.Name {
				t.Errorf("serviceName: expected %s, got %s", tt.cfg.Name, ss.Spec.ServiceName)
			}
		})
	}

	t.Run("qdrant ports and mount", func(t *testing.T) {
		ss := BuildStatefulSet(tests[0].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

		// Primary port 6333
		if container.Ports[0].ContainerPort != 6333 {
			t.Errorf("expected port 6333, got %d", container.Ports[0].ContainerPort)
		}
		// Extra gRPC port 6334
		if len(container.Ports) < 2 || container.Ports[1].ContainerPort != 6334 {
			t.Errorf("expected extra grpc port 6334, got %v", container.Ports)
		}
		// Mount path
		if container.VolumeMounts[0].MountPath != "/qdrant/storage" {
			t.Errorf("expected mount /qdrant/storage, got %s", container.VolumeMounts[0].MountPath)
		}
		// Default PVC 10Gi
		pvcStorage := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
		if pvcStorage.Cmp(resource.MustParse("10Gi")) != 0 {
			t.Errorf("expected PVC 10Gi, got %s", pvcStorage.String())
		}
	})

	t.Run("redis port and mount", func(t *testing.T) {
		ss := BuildStatefulSet(tests[1].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

		if container.Ports[0].ContainerPort != 6379 {
			t.Errorf("expected port 6379, got %d", container.Ports[0].ContainerPort)
		}
		if container.VolumeMounts[0].MountPath != "/data" {
			t.Errorf("expected mount /data, got %s", container.VolumeMounts[0].MountPath)
		}
	})

	t.Run("postgres port and mount", func(t *testing.T) {
		ss := BuildStatefulSet(tests[2].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

		if container.Ports[0].ContainerPort != 5432 {
			t.Errorf("expected port 5432, got %d", container.Ports[0].ContainerPort)
		}
		if container.VolumeMounts[0].MountPath != "/var/lib/postgresql/data" {
			t.Errorf("expected mount /var/lib/postgresql/data, got %s", container.VolumeMounts[0].MountPath)
		}
	})

	t.Run("custom storage 20Gi", func(t *testing.T) {
		ss := BuildStatefulSet(tests[3].cfg)

		pvcStorage := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
		if pvcStorage.Cmp(resource.MustParse("20Gi")) != 0 {
			t.Errorf("expected PVC 20Gi, got %s", pvcStorage.String())
		}
	})

	t.Run("healthcheck qdrant - HTTPGet", func(t *testing.T) {
		ss := BuildStatefulSet(tests[4].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

		if container.LivenessProbe == nil {
			t.Fatal("expected liveness probe")
		}
		if container.LivenessProbe.HTTPGet == nil {
			t.Fatal("expected HTTPGet probe for qdrant")
		}
		if container.LivenessProbe.HTTPGet.Path != "/healthz" {
			t.Errorf("expected /healthz, got %s", container.LivenessProbe.HTTPGet.Path)
		}
	})

	t.Run("healthcheck redis - Exec", func(t *testing.T) {
		ss := BuildStatefulSet(tests[5].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

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

	t.Run("ConfigMap and Secret envFrom", func(t *testing.T) {
		ss := BuildStatefulSet(tests[6].cfg)
		container := ss.Spec.Template.Spec.Containers[0]

		if len(container.EnvFrom) != 2 {
			t.Fatalf("expected 2 envFrom, got %d", len(container.EnvFrom))
		}
		if container.EnvFrom[0].ConfigMapRef == nil || container.EnvFrom[0].ConfigMapRef.Name != "my-config" {
			t.Errorf("expected ConfigMapRef my-config")
		}
		if container.EnvFrom[1].SecretRef == nil || container.EnvFrom[1].SecretRef.Name != "my-secret" {
			t.Errorf("expected SecretRef my-secret")
		}
	})
}
