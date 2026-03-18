package k8s

import (
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// TestBuildCronJob verifies that BuildCronJob produces a CronJob with the
// correct schedule, ConcurrencyPolicy=Forbid, history limits (success=3,
// failed=1), RestartPolicy=OnFailure, container name "ingestion-worker",
// ingestion environment variables, and ConfigMap+Secret envFrom refs.
func TestBuildCronJob(t *testing.T) {
	cfg := CronJobConfig{
		Name:          "agent-ingestion-sync",
		Namespace:     "default",
		AgentName:     "my-agent",
		BuildID:       "1.0",
		Component:     "ingestion-sync",
		Schedule:      "*/5 * * * *",
		SecretName:    "my-secret",
		ConfigMapName: "my-config",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{
				Image: "ingest:latest",
				Environment: map[string]string{
					"SOURCE": "s3://bucket",
				},
			},
			Trigger: spec.IngestionTrigger{Type: "schedule"},
		},
	}

	cj := BuildCronJob(cfg)

	if cj.Kind != "CronJob" {
		t.Errorf("kind: expected CronJob, got %s", cj.Kind)
	}
	if cj.Name != cfg.Name {
		t.Errorf("name: expected %s, got %s", cfg.Name, cj.Name)
	}
	if cj.Spec.Schedule != cfg.Schedule {
		t.Errorf("schedule: expected %s, got %s", cfg.Schedule, cj.Spec.Schedule)
	}
	if cj.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("concurrency: expected Forbid, got %s", cj.Spec.ConcurrencyPolicy)
	}
	if *cj.Spec.SuccessfulJobsHistoryLimit != 3 {
		t.Errorf("success history: expected 3, got %d", *cj.Spec.SuccessfulJobsHistoryLimit)
	}
	if *cj.Spec.FailedJobsHistoryLimit != 1 {
		t.Errorf("failed history: expected 1, got %d", *cj.Spec.FailedJobsHistoryLimit)
	}

	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Errorf("restart policy: expected OnFailure, got %s", podSpec.RestartPolicy)
	}

	container := podSpec.Containers[0]
	if container.Name != "ingestion-worker" {
		t.Errorf("container name: expected ingestion-worker, got %s", container.Name)
	}

	// Check env vars
	envMap := make(map[string]string)
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["SOURCE"] != "s3://bucket" {
		t.Errorf("expected SOURCE=s3://bucket, got %q", envMap["SOURCE"])
	}

	// Check envFrom refs
	if len(container.EnvFrom) != 2 {
		t.Fatalf("expected 2 envFrom, got %d", len(container.EnvFrom))
	}
	if container.EnvFrom[0].ConfigMapRef == nil || container.EnvFrom[0].ConfigMapRef.Name != "my-config" {
		t.Error("expected ConfigMapRef my-config")
	}
	if container.EnvFrom[1].SecretRef == nil || container.EnvFrom[1].SecretRef.Name != "my-secret" {
		t.Error("expected SecretRef my-secret")
	}
}

func TestBuildCronJob_NoSecretOrConfigMap(t *testing.T) {
	cfg := CronJobConfig{
		Name: "agent-ingestion-sync", Namespace: "default",
		AgentName: "my-agent", BuildID: "1.0", Component: "ingestion-sync",
		Schedule: "0 * * * *",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "ingest:latest"},
			Trigger:   spec.IngestionTrigger{Type: "schedule"},
		},
	}

	cj := BuildCronJob(cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	if len(container.EnvFrom) != 0 {
		t.Errorf("expected 0 envFrom without secret/configmap, got %d", len(container.EnvFrom))
	}
}

func TestBuildCronJob_ImageAndResources(t *testing.T) {
	cfg := CronJobConfig{
		Name: "agent-ingestion-sync", Namespace: "default",
		AgentName: "my-agent", BuildID: "1.0", Component: "ingestion-sync",
		Schedule: "0 * * * *",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "my-ingest:v2"},
			Trigger:   spec.IngestionTrigger{Type: "schedule"},
		},
	}

	cj := BuildCronJob(cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	if container.Image != "my-ingest:v2" {
		t.Errorf("image: expected my-ingest:v2, got %s", container.Image)
	}
	if container.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("pull policy: expected Always, got %s", container.ImagePullPolicy)
	}

	cpuReq := container.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "50m" {
		t.Errorf("CPU request: expected 50m, got %s", cpuReq.String())
	}
	memLim := container.Resources.Limits[corev1.ResourceMemory]
	if memLim.String() != "256Mi" {
		t.Errorf("memory limit: expected 256Mi, got %s", memLim.String())
	}
}

// TestBuildJob verifies that BuildJob produces a one-shot Job with
// BackoffLimit=3, RestartPolicy=OnFailure, and container name "ingestion-worker".
func TestBuildJob(t *testing.T) {
	cfg := JobConfig{
		Name:          "agent-ingestion-boot",
		Namespace:     "default",
		AgentName:     "my-agent",
		BuildID:       "1.0",
		Component:     "ingestion-boot",
		SecretName:    "my-secret",
		ConfigMapName: "my-config",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{
				Image: "ingest:latest",
			},
			Trigger: spec.IngestionTrigger{Type: "startup"},
		},
	}

	job := BuildJob(cfg)

	if job.Kind != "Job" {
		t.Errorf("kind: expected Job, got %s", job.Kind)
	}
	if job.Name != cfg.Name {
		t.Errorf("name: expected %s, got %s", cfg.Name, job.Name)
	}
	if *job.Spec.BackoffLimit != 3 {
		t.Errorf("backoff limit: expected 3, got %d", *job.Spec.BackoffLimit)
	}

	podSpec := job.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Errorf("restart policy: expected OnFailure, got %s", podSpec.RestartPolicy)
	}

	container := podSpec.Containers[0]
	if container.Name != "ingestion-worker" {
		t.Errorf("container name: expected ingestion-worker, got %s", container.Name)
	}
}

func TestBuildJob_TTLAndEnv(t *testing.T) {
	cfg := JobConfig{
		Name: "agent-ingestion-boot", Namespace: "default",
		AgentName: "my-agent", BuildID: "1.0", Component: "ingestion-boot",
		SecretName: "my-secret", ConfigMapName: "my-config",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{
				Image:       "ingest:latest",
				Environment: map[string]string{"MODE": "full", "BATCH": "100"},
			},
			Trigger: spec.IngestionTrigger{Type: "startup"},
		},
	}

	job := BuildJob(cfg)

	// TTL
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 86400 {
		t.Errorf("TTL: expected 86400, got %v", job.Spec.TTLSecondsAfterFinished)
	}

	// Labels
	if job.Labels["astro.dev/agent"] != "my-agent" {
		t.Errorf("agent label: expected my-agent, got %s", job.Labels["astro.dev/agent"])
	}

	// Env vars
	container := job.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["MODE"] != "full" {
		t.Errorf("expected MODE=full, got %q", envMap["MODE"])
	}
	if envMap["BATCH"] != "100" {
		t.Errorf("expected BATCH=100, got %q", envMap["BATCH"])
	}

	// EnvFrom
	if len(container.EnvFrom) != 2 {
		t.Fatalf("expected 2 envFrom, got %d", len(container.EnvFrom))
	}
}

func TestBuildJob_NoSecretOrConfigMap(t *testing.T) {
	cfg := JobConfig{
		Name: "agent-ingestion-boot", Namespace: "default",
		AgentName: "my-agent", BuildID: "1.0", Component: "ingestion-boot",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "ingest:latest"},
			Trigger:   spec.IngestionTrigger{Type: "startup"},
		},
	}

	job := BuildJob(cfg)
	container := job.Spec.Template.Spec.Containers[0]

	if len(container.EnvFrom) != 0 {
		t.Errorf("expected 0 envFrom without secret/configmap, got %d", len(container.EnvFrom))
	}
}

// TestBuildIngestionDeployment verifies that BuildIngestionDeployment produces a
// long-running Deployment for webhook-triggered ingestion with replicas=1,
// RestartPolicy=Always, the specified container port, and container name
// "ingestion-worker". Also checks that a custom port is correctly applied.
func TestBuildIngestionDeployment(t *testing.T) {
	cfg := JobConfig{
		Name:          "agent-ingestion-webhook",
		Namespace:     "default",
		AgentName:     "my-agent",
		BuildID:       "1.0",
		Component:     "ingestion-webhook",
		SecretName:    "my-secret",
		ConfigMapName: "my-config",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{
				Image: "ingest:latest",
			},
			Trigger: spec.IngestionTrigger{Type: "webhook"},
		},
	}

	cfg.ImagePullPolicy = corev1.PullAlways
	d := BuildIngestionDeployment(cfg, 8080)

	if d.Kind != "Deployment" {
		t.Errorf("kind: expected Deployment, got %s", d.Kind)
	}
	if *d.Spec.Replicas != 1 {
		t.Errorf("replicas: expected 1, got %d", *d.Spec.Replicas)
	}

	podSpec := d.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Errorf("restart policy: expected Always, got %s", podSpec.RestartPolicy)
	}

	container := podSpec.Containers[0]
	if container.Name != "ingestion-worker" {
		t.Errorf("container name: expected ingestion-worker, got %s", container.Name)
	}
	if len(container.Ports) == 0 || container.Ports[0].ContainerPort != 8080 {
		t.Errorf("expected port 8080, got %v", container.Ports)
	}

	// Custom port
	d2 := BuildIngestionDeployment(cfg, 9090)
	container2 := d2.Spec.Template.Spec.Containers[0]
	if container2.Ports[0].ContainerPort != 9090 {
		t.Errorf("expected custom port 9090, got %d", container2.Ports[0].ContainerPort)
	}
}

func TestBuildIngestionDeployment_EnvAndLabels(t *testing.T) {
	cfg := JobConfig{
		Name: "agent-ingestion-webhook", Namespace: "prod",
		AgentName: "my-agent", BuildID: "2.0", Component: "ingestion-webhook",
		SecretName: "my-secret", ConfigMapName: "my-config",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{
				Image:       "ingest:latest",
				Environment: map[string]string{"ENDPOINT": "/hooks/jira"},
			},
			Trigger: spec.IngestionTrigger{Type: "webhook"},
		},
	}

	cfg.ImagePullPolicy = corev1.PullIfNotPresent
	d := BuildIngestionDeployment(cfg, 3001)

	// Labels
	if d.Labels["astro.dev/agent"] != "my-agent" {
		t.Errorf("agent label: expected my-agent, got %s", d.Labels["astro.dev/agent"])
	}

	// Selector
	if d.Spec.Selector.MatchLabels["astro.dev/agent"] != "my-agent" {
		t.Error("expected agent selector label")
	}

	container := d.Spec.Template.Spec.Containers[0]

	// ImagePullPolicy override
	if container.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("pull policy: expected IfNotPresent, got %s", container.ImagePullPolicy)
	}

	// Env vars from ingestion
	envMap := make(map[string]string)
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["ENDPOINT"] != "/hooks/jira" {
		t.Errorf("expected ENDPOINT=/hooks/jira, got %q", envMap["ENDPOINT"])
	}

	// EnvFrom
	if len(container.EnvFrom) != 2 {
		t.Fatalf("expected 2 envFrom, got %d", len(container.EnvFrom))
	}
	if container.EnvFrom[0].ConfigMapRef == nil || container.EnvFrom[0].ConfigMapRef.Name != "my-config" {
		t.Error("expected ConfigMapRef my-config")
	}
	if container.EnvFrom[1].SecretRef == nil || container.EnvFrom[1].SecretRef.Name != "my-secret" {
		t.Error("expected SecretRef my-secret")
	}

	// Resources
	cpuReq := container.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "50m" {
		t.Errorf("CPU request: expected 50m, got %s", cpuReq.String())
	}
}
