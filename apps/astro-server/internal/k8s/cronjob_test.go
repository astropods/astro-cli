package k8s

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
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
		Version:       "1.0",
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
			Trigger: spec.IngestionTrigger{Type: "schedule", Schedule: "*/5 * * * *"},
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

// TestBuildJob verifies that BuildJob produces a one-shot Job with
// BackoffLimit=3, RestartPolicy=OnFailure, and container name "ingestion-worker".
func TestBuildJob(t *testing.T) {
	cfg := JobConfig{
		Name:          "agent-ingestion-boot",
		Namespace:     "default",
		AgentName:     "my-agent",
		Version:       "1.0",
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

// TestBuildIngestionDeployment verifies that BuildIngestionDeployment produces a
// long-running Deployment for webhook-triggered ingestion with replicas=1,
// RestartPolicy=Always, the specified container port, and container name
// "ingestion-worker". Also checks that a custom port is correctly applied.
func TestBuildIngestionDeployment(t *testing.T) {
	cfg := JobConfig{
		Name:          "agent-ingestion-webhook",
		Namespace:     "default",
		AgentName:     "my-agent",
		Version:       "1.0",
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

	d := BuildIngestionDeployment(cfg, 8080, corev1.PullAlways)

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
	d2 := BuildIngestionDeployment(cfg, 9090, corev1.PullAlways)
	container2 := d2.Spec.Template.Spec.Containers[0]
	if container2.Ports[0].ContainerPort != 9090 {
		t.Errorf("expected custom port 9090, got %d", container2.Ports[0].ContainerPort)
	}
}
