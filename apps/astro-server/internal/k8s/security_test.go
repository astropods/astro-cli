package k8s

import (
	"net/http"
	"net/http/httptest"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type stubClusterClient struct{ cs *kubernetes.Clientset }

func (s *stubClusterClient) Clientset() *kubernetes.Clientset      { return s.cs }
func (s *stubClusterClient) Config() *rest.Config                  { return nil }
func (s *stubClusterClient) CheckHealth() error                    { return nil }
func (s *stubClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (s *stubClusterClient) DiagnoseConnection() map[string]string { return nil }

func newStubClient(t *testing.T) ClusterClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return &stubClusterClient{cs: cs}
}

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
		Container:       spec.ContainerConfig{Image: "qdrant/qdrant/storage:latest"},
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
		Container:       spec.ContainerConfig{Image: "qdrant/qdrant/storage:latest"},
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
	if mountMap["data"] != "/qdrant/storage" {
		t.Errorf("expected data mount at /qdrant/storage, got %q", mountMap["data"])
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

	cfg.ImagePullPolicy = corev1.PullAlways
	d := BuildIngestionDeployment(cfg, 8080)
	ps := d.Spec.Template.Spec

	assertHardenedPodSpec(t, ps)
	assertHardenedContainer(t, ps.Containers[0])
}

// assertUnhardenedPodSpec checks that a PodSpec does NOT have security hardening applied.
func assertUnhardenedPodSpec(t *testing.T, ps corev1.PodSpec) {
	t.Helper()
	if ps.SecurityContext != nil && ps.SecurityContext.RunAsUser != nil {
		t.Errorf("expected no runAsUser override, got %d", *ps.SecurityContext.RunAsUser)
	}
}

func assertUnhardenedContainer(t *testing.T, c corev1.Container) {
	t.Helper()
	if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
		t.Errorf("container %q: expected no runAsUser override, got %d", c.Name, *c.SecurityContext.RunAsUser)
	}
}

// ---------------------------------------------------------------------------
// LocalMode isolation: exhaustive matrix tests
//
// These verify that security hardening is ONLY relaxed when ALL of:
//   1. LocalMode is true
//   2. The container is a third-party provider (Provider != "")
//
// Production, preview, staging, and default ("") modes must always produce
// fully hardened pods/containers.
// ---------------------------------------------------------------------------

func TestLocalModeIsolation_StatefulSet(t *testing.T) {
	type testCase struct {
		name      string
		localMode bool
		provider  string
		section   string
		wantHard  bool
	}

	cases := []testCase{
		{"prod/qdrant", false, "qdrant", "knowledge", true},
		{"prod/neo4j", false, "neo4j", "knowledge", true},
		{"prod/redis", false, "redis", "knowledge", true},
		{"prod/postgres", false, "postgres", "knowledge", true},
		{"prod/ollama", false, "ollama", "models", true},
		{"default(false)/qdrant", false, "qdrant", "knowledge", true},
		{"local/qdrant", true, "qdrant", "knowledge", false},
		{"local/neo4j", true, "neo4j", "knowledge", false},
		{"local/redis", true, "redis", "knowledge", false},
		{"local/postgres", true, "postgres", "knowledge", false},
		{"local/ollama", true, "ollama", "models", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := StatefulSetConfig{
				Name: "test", Namespace: "default",
				AgentName: "a", BuildID: "1", Component: "c",
				Container:       spec.ContainerConfig{Image: "img:latest"},
				Provider:        tc.provider,
				ProviderSection: tc.section,
				LocalMode:       tc.localMode,
			}

			ss, err := BuildStatefulSet(cfg)
			if err != nil {
				t.Fatalf("BuildStatefulSet: %v", err)
			}
			ps := ss.Spec.Template.Spec

			if tc.wantHard {
				assertHardenedPodSpec(t, ps)
				assertHardenedContainer(t, ps.Containers[0])
			} else {
				assertUnhardenedPodSpec(t, ps)
				assertUnhardenedContainer(t, ps.Containers[0])
			}
		})
	}
}

func TestLocalModeIsolation_Deployment(t *testing.T) {
	type testCase struct {
		name      string
		localMode bool
		provider  string
		section   string
		wantHard  bool
	}

	cases := []testCase{
		// Production: all providers hardened
		{"prod/model-provider", false, "ollama", "models", true},
		{"prod/knowledge-provider", false, "redis", "knowledge", true},
		{"prod/agent-no-provider", false, "", "", true},
		// Default zero-value: hardened (catches accidental true)
		{"zero-value/provider", false, "qdrant", "knowledge", true},
		{"zero-value/agent", false, "", "", true},
		// Local: only provider containers are unhardened
		{"local/provider-redis", true, "redis", "knowledge", false},
		{"local/provider-neo4j", true, "neo4j", "knowledge", false},
		{"local/provider-ollama", true, "ollama", "models", false},
		// Local: non-provider containers are STILL hardened
		{"local/agent-no-provider", true, "", "", true},
		{"local/tool-no-provider", true, "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DeploymentConfig{
				Name: "test", Namespace: "default",
				AgentName: "a", BuildID: "1", Component: "c",
				Container:       spec.ContainerConfig{Image: "img:latest"},
				Provider:        tc.provider,
				ProviderSection: tc.section,
				LocalMode:       tc.localMode,
				Port:            8080,
			}

			d := BuildDeployment(cfg)
			ps := d.Spec.Template.Spec

			if tc.wantHard {
				assertHardenedPodSpec(t, ps)
				assertHardenedContainer(t, ps.Containers[0])
			} else {
				assertUnhardenedPodSpec(t, ps)
				assertUnhardenedContainer(t, ps.Containers[0])
			}
		})
	}
}

// Sidecars (messaging, collector) must ALWAYS be hardened even in local mode
// with a provider present. This catches regressions where local-mode relaxation
// accidentally leaks to non-provider containers in the same pod.
func TestLocalMode_SidecarsAlwaysHardened(t *testing.T) {
	cfg := DeploymentConfig{
		Name: "agent-with-sidecars", Namespace: "default",
		AgentName: "a", BuildID: "1", Component: "agent",
		Container: spec.ContainerConfig{Image: "agent:latest"},
		LocalMode: true,
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

// Ingestion resources (Job, CronJob, IngestionDeployment) are always the user's
// own images, never third-party providers. They have no LocalMode field and must
// ALWAYS be hardened regardless of environment.
func TestLocalModeNeverAffectsIngestion(t *testing.T) {
	t.Run("Job", func(t *testing.T) {
		job := BuildJob(JobConfig{
			Name: "j", Namespace: "default",
			AgentName: "a", BuildID: "1", Component: "c",
			Ingestion: spec.Ingestion{
				Container: spec.ContainerConfig{Image: "ingest:latest"},
				Trigger:   spec.IngestionTrigger{Type: "startup"},
			},
		})
		ps := job.Spec.Template.Spec
		assertHardenedPodSpec(t, ps)
		assertHardenedContainer(t, ps.Containers[0])
	})

	t.Run("CronJob", func(t *testing.T) {
		cj := BuildCronJob(CronJobConfig{
			Name: "cj", Namespace: "default",
			AgentName: "a", BuildID: "1", Component: "c",
			Schedule: "*/5 * * * *",
			Ingestion: spec.Ingestion{
				Container: spec.ContainerConfig{Image: "ingest:latest"},
				Trigger:   spec.IngestionTrigger{Type: "schedule"},
			},
		})
		ps := cj.Spec.JobTemplate.Spec.Template.Spec
		assertHardenedPodSpec(t, ps)
		assertHardenedContainer(t, ps.Containers[0])
	})

	t.Run("IngestionDeployment", func(t *testing.T) {
		d := BuildIngestionDeployment(JobConfig{
			Name: "id", Namespace: "default",
			AgentName: "a", BuildID: "1", Component: "c",
			ImagePullPolicy: corev1.PullAlways,
			Ingestion: spec.Ingestion{
				Container: spec.ContainerConfig{Image: "ingest:latest"},
				Trigger:   spec.IngestionTrigger{Type: "webhook"},
			},
		}, 8080)
		ps := d.Spec.Template.Spec
		assertHardenedPodSpec(t, ps)
		assertHardenedContainer(t, ps.Containers[0])
	})
}

// The zero value of LocalMode (false) must produce hardened resources.
// This catches struct initialization bugs where LocalMode accidentally defaults
// to true.
func TestLocalModeZeroValue_AlwaysHardened(t *testing.T) {
	var cfg StatefulSetConfig
	if cfg.LocalMode {
		t.Fatal("zero-value StatefulSetConfig.LocalMode must be false")
	}

	var dcfg DeploymentConfig
	if dcfg.LocalMode {
		t.Fatal("zero-value DeploymentConfig.LocalMode must be false")
	}

	var acfg ApplierConfig
	if acfg.LocalMode {
		t.Fatal("zero-value ApplierConfig.LocalMode must be false")
	}
}

// NewApplier must propagate LocalMode from config to the internal struct.
// Verifies the wiring doesn't silently drop the field.
func TestNewApplier_LocalModePropagation(t *testing.T) {
	client := newStubClient(t)
	cases := []struct {
		name      string
		localMode bool
	}{
		{"false", false},
		{"true", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApplier(client, ApplierConfig{
				Namespace: "ns",
				LocalMode: tc.localMode,
			})
			if a.localMode != tc.localMode {
				t.Errorf("Applier.localMode = %v, want %v", a.localMode, tc.localMode)
			}
		})
	}
}
