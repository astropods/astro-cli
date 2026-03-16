package k8s

import (
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
)

// assertHardenedPodSpec checks that a PodSpec has the restricted security defaults applied.
func assertHardenedPodSpec(t *testing.T, ps corev1.PodSpec) {
	t.Helper()

	// SecurityContext
	if ps.SecurityContext == nil {
		t.Fatal("expected pod SecurityContext")
	}
	if ps.SecurityContext.RunAsNonRoot == nil || !*ps.SecurityContext.RunAsNonRoot {
		t.Error("expected runAsNonRoot=true")
	}
	if ps.SecurityContext.RunAsUser == nil || *ps.SecurityContext.RunAsUser != 1000 {
		t.Error("expected runAsUser=1000")
	}
	if ps.SecurityContext.FSGroup == nil || *ps.SecurityContext.FSGroup != 1000 {
		t.Error("expected fsGroup=1000")
	}
	if ps.SecurityContext.SeccompProfile == nil || ps.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("expected seccompProfile RuntimeDefault")
	}

	// automountServiceAccountToken
	if ps.AutomountServiceAccountToken == nil || *ps.AutomountServiceAccountToken {
		t.Error("expected automountServiceAccountToken=false")
	}
}

// assertHardenedContainer checks that a Container has the restricted security defaults applied.
func assertHardenedContainer(t *testing.T, c corev1.Container) {
	t.Helper()

	sc := c.SecurityContext
	if sc == nil {
		t.Fatalf("container %q: expected SecurityContext", c.Name)
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Errorf("container %q: expected runAsNonRoot=true", c.Name)
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 1000 {
		t.Errorf("container %q: expected runAsUser=1000", c.Name)
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Errorf("container %q: expected allowPrivilegeEscalation=false", c.Name)
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Errorf("container %q: expected capabilities.drop=[ALL]", c.Name)
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("container %q: expected seccompProfile RuntimeDefault", c.Name)
	}
}

func TestDeploymentSecurityHardening(t *testing.T) {
	cfg := DeploymentConfig{
		Name:      "test-deploy",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "agent",
		Container: spec.ContainerConfig{Image: "agent:latest"},
	}

	d := BuildDeployment(cfg)
	ps := d.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}

func TestDeploymentSecurityHardening_AllSidecars(t *testing.T) {
	cfg := DeploymentConfig{
		Name:      "full-agent",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "agent",
		Container: spec.ContainerConfig{Image: "agent:latest"},
		Messaging: &MessagingDeploymentConfig{
			Image:        "messaging:latest",
			SlackEnabled: true,
		},
		Collector: &CollectorDeploymentConfig{
			Image: "collector:latest",
		},
	}

	d := BuildDeployment(cfg)
	ps := d.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	for _, c := range ps.Containers {
		assertHardenedContainer(t, c)
	}
}

func TestStatefulSetSecurityHardening(t *testing.T) {
	cfg := StatefulSetConfig{
		Name:            "agent-knowledge-vectors",
		Namespace:       "default",
		AgentName:       "my-agent",
		BuildID:         "1.0",
		Component:       "knowledge-vectors",
		Container:       spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
		Provider:        "qdrant",
		ProviderSection: "knowledge",
	}

	ss, err := BuildStatefulSet(cfg)
	if err != nil {
		t.Fatalf("BuildStatefulSet: %v", err)
	}

	ps := ss.Spec.Template.Spec
	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}

func TestCronJobSecurityHardening(t *testing.T) {
	cfg := CronJobConfig{
		Name:      "agent-ingestion-sync",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "ingestion-sync",
		Schedule:  "*/5 * * * *",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "ingest:latest"},
			Trigger:   spec.IngestionTrigger{Type: "schedule"},
		},
	}

	cj := BuildCronJob(cfg)
	ps := cj.Spec.JobTemplate.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}

func TestJobSecurityHardening(t *testing.T) {
	cfg := JobConfig{
		Name:      "agent-ingestion-boot",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "ingestion-boot",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "ingest:latest"},
			Trigger:   spec.IngestionTrigger{Type: "startup"},
		},
	}

	job := BuildJob(cfg)
	ps := job.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}

func TestStatefulSetSecurityHardening_PreservesDataMount(t *testing.T) {
	cfg := StatefulSetConfig{
		Name:            "agent-knowledge-vectors",
		Namespace:       "default",
		AgentName:       "my-agent",
		BuildID:         "1.0",
		Component:       "knowledge-vectors",
		Container:       spec.ContainerConfig{Image: "qdrant/qdrant:latest"},
		Provider:        "qdrant",
		ProviderSection: "knowledge",
	}

	ss, err := BuildStatefulSet(cfg)
	if err != nil {
		t.Fatalf("BuildStatefulSet: %v", err)
	}

	container := ss.Spec.Template.Spec.Containers[0]

	mountMap := make(map[string]string)
	for _, vm := range container.VolumeMounts {
		mountMap[vm.Name] = vm.MountPath
	}
	if mountMap["data"] != "/qdrant" {
		t.Errorf("expected data mount at /qdrant, got %q", mountMap["data"])
	}
}

func TestNoProviderNoExtraMounts(t *testing.T) {
	cfg := DeploymentConfig{
		Name:      "agent-app",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "agent",
		Container: spec.ContainerConfig{Image: "app:latest"},
		Port:      8080,
	}
	depl := BuildDeployment(cfg)
	container := depl.Spec.Template.Spec.Containers[0]

	if len(container.VolumeMounts) != 0 {
		t.Errorf("expected no volume mounts for provider-less deployment, got %d", len(container.VolumeMounts))
	}
}

func TestIngestionDeploymentSecurityHardening(t *testing.T) {
	cfg := JobConfig{
		Name:      "agent-ingestion-webhook",
		Namespace: "default",
		AgentName: "my-agent",
		BuildID:   "1.0",
		Component: "ingestion-webhook",
		Ingestion: spec.Ingestion{
			Container: spec.ContainerConfig{Image: "ingest:latest"},
			Trigger:   spec.IngestionTrigger{Type: "webhook"},
		},
	}

	d := BuildIngestionDeployment(cfg, 8080, corev1.PullAlways)
	ps := d.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}
