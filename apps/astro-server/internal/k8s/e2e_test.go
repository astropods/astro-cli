package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro-spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// e2eOpts holds user-provided values that are filled into the deployment template.
type e2eOpts struct {
	Account     string
	BuildID     string
	Namespace   string
	RegistryURL string
	Credentials map[string]string // credential key → value
	Schedules   map[string]string // ingestion name → cron expression
	Interfaces  []string          // adapters to enable (e.g. "slack", "web")

}

// e2eResult holds all outputs from the pipeline for assertions.
type e2eResult struct {
	ApplyResult    *ApplyResult
	DeploymentSpec *spec.AstroDeploymentSpec
	Clientset      kubernetes.Interface
	Namespace      string
}

// runE2E executes the full pipeline: parse YAML → generate template → fill values → resolve env → apply to fake k8s.
func runE2E(t *testing.T, yamlSpec string, opts e2eOpts) *e2eResult {
	t.Helper()

	// Defaults
	if opts.Account == "" {
		opts.Account = "acme"
	}
	if opts.BuildID == "" {
		opts.BuildID = "build-001"
	}
	if opts.Namespace == "" {
		opts.Namespace = "test-ns"
	}
	if opts.RegistryURL == "" {
		opts.RegistryURL = "test-registry.example.com"
	}

	// Step 1: Parse the astro-spec YAML
	astroSpec, err := spec.ParseString(yamlSpec)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	// Step 2: Generate deployment template
	ds, err := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
		Spec:        astroSpec,
		AgentName:   astroSpec.Name,
		Account:     opts.Account,
		BuildID:     opts.BuildID,
		RegistryURL: opts.RegistryURL,
	})
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}

	// Step 3: Fill user-provided values

	// Fill credential/variable values
	for key, val := range opts.Credentials {
		if ds.Variables == nil {
			ds.Variables = make(map[string]spec.Variable)
		}
		v, ok := ds.Variables[key]
		if !ok {
			t.Fatalf("variable %q not in deployment spec (available: %v)", key, variableKeys(ds.Variables))
		}
		v.Value = val
		ds.Variables[key] = v
	}

	// Fill schedules
	for name, sched := range opts.Schedules {
		if ing, ok := ds.Ingestion[name]; ok {
			ing.Trigger.Schedule = sched
			ds.Ingestion[name] = ing
		}
	}

	// Fill interfaces
	if len(opts.Interfaces) > 0 && ds.Interfaces != nil {
		ds.Interfaces.Adapters = opts.Interfaces
	}

	// Step 4: Resolve ${} references
	rctx := deployment.ResolveContext{
		Namespace:  opts.Namespace,
		AgentName:  astroSpec.Name,
		BuildID:    opts.BuildID,
		SecretName: deployment.GenerateSecretName(astroSpec.Name, opts.BuildID),
	}
	deployment.ResolveDeploymentSpecEnv(ds, rctx)

	// Step 5: Apply to fake k8s
	fakeClient := fake.NewClientset()
	applier := &Applier{
		clientset:       fakeClient,
		namespace:       opts.Namespace,
		imagePullPolicy: corev1.PullNever,
	}

	result, err := applier.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("ApplyDeploymentSpec: %v", err)
	}

	return &e2eResult{
		ApplyResult:    result,
		DeploymentSpec: ds,
		Clientset:      fakeClient,
		Namespace:      opts.Namespace,
	}
}

// --- helpers ---

func (r *e2eResult) resourceCount(kind string) int {
	n := 0
	for _, res := range r.ApplyResult.Resources {
		if res.Kind == kind {
			n++
		}
	}
	return n
}

func (r *e2eResult) hasResource(kind, name string) bool {
	for _, res := range r.ApplyResult.Resources {
		if res.Kind == kind && res.Name == name {
			return true
		}
	}
	return false
}

func (r *e2eResult) getSecret(t *testing.T, ns, name string) *corev1.Secret {
	t.Helper()
	s, err := r.Clientset.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret %s/%s: %v", ns, name, err)
	}
	return s
}

func (r *e2eResult) getConfigMap(t *testing.T, ns, name string) *corev1.ConfigMap {
	t.Helper()
	cm, err := r.Clientset.CoreV1().ConfigMaps(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap %s/%s: %v", ns, name, err)
	}
	return cm
}

func requireNoErrors(t *testing.T, r *e2eResult) {
	t.Helper()
	if len(r.ApplyResult.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", r.ApplyResult.Errors)
	}
}

// --- test cases ---

func TestE2E_MinimalAgent(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
`, e2eOpts{})

	requireNoErrors(t, r)

	// Should have agent StatefulSet + Service
	if !r.hasResource("StatefulSet", "my-agent-agent") {
		t.Error("expected agent StatefulSet")
	}
	if !r.hasResource("Service", "my-agent-agent") {
		t.Error("expected agent Service")
	}

	// Verify correct port in the deployment spec
	if spec.PrimaryPort(r.DeploymentSpec.Agent.Endpoints) != 8080 {
		t.Errorf("expected default port 8080, got %d", spec.PrimaryPort(r.DeploymentSpec.Agent.Endpoints))
	}

	// Check agent-http endpoint
	found := false
	for _, ep := range r.ApplyResult.ServiceEndpoints {
		if ep.Name == "agent-http" {
			found = true
			if ep.Port != 8080 {
				t.Errorf("expected endpoint port 8080, got %d", ep.Port)
			}
		}
	}
	if !found {
		t.Error("expected agent-http service endpoint")
	}
}

func TestE2E_SelfHostedModel_Container(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    container:
      image: my-model:latest
      port: 8000
      gpu:
        vram: 24Gi
        runtime: cuda
`, e2eOpts{})

	requireNoErrors(t, r)

	// Container-mode model → Deployment (models no longer deploy StatefulSets)
	if !r.hasResource("Deployment", "my-agent-model-llm") {
		t.Error("expected model Deployment for container-mode model")
	}
	if !r.hasResource("Service", "my-agent-model-llm") {
		t.Error("expected model Service")
	}

	// Agent env should have MODEL_LLM_HOST/PORT/URL wired
	env := r.DeploymentSpec.Agent.Environment
	for _, key := range []string{"MODEL_LLM_HOST", "MODEL_LLM_PORT", "MODEL_LLM_URL"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected agent env key %s", key)
		}
	}

	// ConfigMap should have resolved MODEL_LLM_HOST to a service DNS
	ns := r.Namespace
	cmName := deployment.GenerateConfigMapName("my-agent", "build-001")
	cm := r.getConfigMap(t, ns, cmName)
	modelHost := cm.Data["MODEL_LLM_HOST"]
	if !strings.Contains(modelHost, "my-agent-model-llm") {
		t.Errorf("expected MODEL_LLM_HOST to contain service DNS, got %q", modelHost)
	}
}

func TestE2E_CloudModelCredentials(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    provider: anthropic
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_API_KEY": "sk-ant-test-key",
		},
	})

	requireNoErrors(t, r)

	// Cloud provider → no model Deployment/StatefulSet
	if r.hasResource("Deployment", "my-agent-model-llm") {
		t.Error("did not expect model Deployment for cloud provider")
	}
	if r.hasResource("StatefulSet", "my-agent-model-llm") {
		t.Error("did not expect model StatefulSet for cloud provider")
	}

	// Secret should contain the credential
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["ANTHROPIC_API_KEY"]) != "sk-ant-test-key" {
		t.Errorf("expected ANTHROPIC_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}

	// Agent env should reference the variable
	if ref, ok := r.DeploymentSpec.Agent.Environment["ANTHROPIC_API_KEY"]; !ok {
		t.Error("expected ANTHROPIC_API_KEY in agent env")
	} else if !strings.Contains(ref, "variables") {
		t.Errorf("expected variable reference, got %q", ref)
	}
}

func TestE2E_KnowledgeStore_Qdrant_Persistent(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  docs:
    provider: qdrant
`, e2eOpts{})

	requireNoErrors(t, r)

	// Persistent qdrant → StatefulSet
	if !r.hasResource("StatefulSet", "my-agent-knowledge-docs") {
		t.Error("expected knowledge StatefulSet for persistent qdrant")
	}
	if !r.hasResource("Service", "my-agent-knowledge-docs") {
		t.Error("expected knowledge Service")
	}
	// Should NOT have a Deployment for this knowledge
	if r.hasResource("Deployment", "my-agent-knowledge-docs") {
		t.Error("did not expect Deployment for persistent knowledge")
	}

	// Agent env should have QDRANT_HOST, QDRANT_PORT, QDRANT_URL
	env := r.DeploymentSpec.Agent.Environment
	for _, key := range []string{"QDRANT_HOST", "QDRANT_PORT", "QDRANT_URL"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected agent env key %s", key)
		}
	}
}

// Container-mode without volume → ephemeral → Deployment, not StatefulSet.
// This is the only way to express non-persistent knowledge under the
// mount-path-implies-persistent rule: provider mode always carries a MountPath.
func TestE2E_KnowledgeStore_Container_NonPersistent(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  cache:
    container:
      image: redis:7-alpine
      port: 6379
`, e2eOpts{})

	requireNoErrors(t, r)

	// Container mode without volume → Deployment (not StatefulSet)
	if !r.hasResource("Deployment", "my-agent-knowledge-cache") {
		t.Error("expected knowledge Deployment for ephemeral container-mode entry")
	}
	if r.hasResource("StatefulSet", "my-agent-knowledge-cache") {
		t.Error("did not expect StatefulSet for ephemeral container-mode entry")
	}
	if !r.hasResource("Service", "my-agent-knowledge-cache") {
		t.Error("expected knowledge Service")
	}
}

func TestE2E_CloudIntegration_GitHub(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  github:
    provider: github
`, e2eOpts{
		Credentials: map[string]string{
			"GITHUB_TOKEN": "ghp_test123",
		},
	})

	requireNoErrors(t, r)

	// Cloud integration → no tool Deployment
	if r.hasResource("Deployment", "my-agent-integration-github") {
		t.Error("did not expect tool Deployment for cloud provider")
	}

	// Secret should have GITHUB_TOKEN
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["GITHUB_TOKEN"]) != "ghp_test123" {
		t.Errorf("expected GITHUB_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}

	// Agent env should reference the credential
	env := r.DeploymentSpec.Agent.Environment
	if _, ok := env["GITHUB_TOKEN"]; !ok {
		t.Error("expected GITHUB_TOKEN in agent env")
	}
}

func TestE2E_IntegrationCredentials(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
providers:
  jira:
    scope: [integrations]
    variables:
      - name: API_TOKEN
        datatype: string
        secret: true
        description: "Jira API token"
      - name: EMAIL
        datatype: string
        secret: true
        description: "Jira account email"
integrations:
  jira:
    provider: jira
`, e2eOpts{
		Credentials: map[string]string{
			"JIRA_API_TOKEN": "token-abc",
			"JIRA_EMAIL":     "user@example.com",
		},
	})

	requireNoErrors(t, r)

	// Secret should have both integration credentials (keys are {UPPER(provider)}_{varName})
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["JIRA_API_TOKEN"]) != "token-abc" {
		t.Errorf("expected JIRA_API_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["JIRA_EMAIL"]) != "user@example.com" {
		t.Errorf("expected JIRA_EMAIL in secret, got keys: %v", keysOf(secret.Data))
	}

	// Agent env should reference both credentials
	env := r.DeploymentSpec.Agent.Environment
	for _, key := range []string{"JIRA_API_TOKEN", "JIRA_EMAIL"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected %s in agent env", key)
		}
	}
}

func TestE2E_Ingestion_Schedule(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
ingestion:
  daily:
    container:
      image: my-ingest:latest
    trigger:
      type: schedule
`, e2eOpts{
		Schedules: map[string]string{
			"daily": "0 0 * * *",
		},
	})

	requireNoErrors(t, r)

	if !r.hasResource("CronJob", "my-agent-ingestion-daily") {
		t.Error("expected CronJob for schedule ingestion")
	}
}

func TestE2E_Ingestion_Startup(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
ingestion:
  init:
    container:
      image: my-ingest:latest
    trigger:
      type: startup
`, e2eOpts{})

	requireNoErrors(t, r)

	if !r.hasResource("Job", "my-agent-ingestion-init") {
		t.Error("expected Job for startup ingestion")
	}
}

func TestE2E_FullStack(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    container:
      image: my-model:latest
      port: 8000
      gpu:
        vram: 24Gi
        runtime: cuda
  cloud:
    provider: anthropic
knowledge:
  docs:
    provider: qdrant
  cache:
    provider: redis
integrations:
  github:
    provider: github
  jira:
    provider: jira
providers:
  jira:
    scope: [integrations]
    variables:
      - name: TOKEN
        datatype: string
        secret: true
        description: "Jira token"
ingestion:
  daily:
    container:
      image: my-ingest:latest
    trigger:
      type: schedule
  init:
    container:
      image: my-init:latest
    trigger:
      type: startup
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_API_KEY": "sk-ant-123",
			"GITHUB_TOKEN":      "ghp_456",
			"JIRA_TOKEN":        "jira-789",
		},
		Schedules: map[string]string{
			"daily": "0 2 * * *",
		},
	})

	requireNoErrors(t, r)

	// Agent
	if !r.hasResource("StatefulSet", "my-agent-agent") {
		t.Error("expected agent StatefulSet")
	}
	if !r.hasResource("Service", "my-agent-agent") {
		t.Error("expected agent Service")
	}

	// Self-hosted container-mode model → Deployment (models deploy as Deployments)
	if !r.hasResource("Deployment", "my-agent-model-llm") {
		t.Error("expected container-mode model Deployment")
	}
	if !r.hasResource("Service", "my-agent-model-llm") {
		t.Error("expected container-mode model Service")
	}

	// Cloud model (anthropic) → no container resources
	if r.hasResource("Deployment", "my-agent-model-cloud") {
		t.Error("did not expect Deployment for cloud model")
	}

	// Knowledge: persistent qdrant → StatefulSet
	if !r.hasResource("StatefulSet", "my-agent-knowledge-docs") {
		t.Error("expected qdrant knowledge StatefulSet")
	}
	if !r.hasResource("Service", "my-agent-knowledge-docs") {
		t.Error("expected qdrant knowledge Service")
	}

	// Knowledge: redis provider has MountPath → StatefulSet
	if !r.hasResource("StatefulSet", "my-agent-knowledge-cache") {
		t.Error("expected redis knowledge StatefulSet")
	}
	if !r.hasResource("Service", "my-agent-knowledge-cache") {
		t.Error("expected redis knowledge Service")
	}

	// Cloud integration (github) → no container, but credential in Secret
	if r.hasResource("Deployment", "my-agent-integration-github") {
		t.Error("did not expect Deployment for cloud integration")
	}

	// Secret should exist with credentials
	if !r.hasResource("Secret", deployment.GenerateSecretName("my-agent", "build-001")) {
		t.Error("expected credentials Secret")
	}

	// ConfigMap should exist with resolved env
	if !r.hasResource("ConfigMap", deployment.GenerateConfigMapName("my-agent", "build-001")) {
		t.Error("expected config ConfigMap")
	}

	// Observability → collector is a standalone deployment with its own service
	if !r.hasResource("Deployment", "my-agent-collector") {
		t.Error("expected collector Deployment")
	}
	if !r.hasResource("Service", "my-agent-collector") {
		t.Error("expected collector Service")
	}

	// Ingestion
	if !r.hasResource("CronJob", "my-agent-ingestion-daily") {
		t.Error("expected CronJob for daily ingestion")
	}
	if !r.hasResource("Job", "my-agent-ingestion-init") {
		t.Error("expected Job for init ingestion")
	}

	// Verify env wiring in ConfigMap
	ns := r.Namespace
	cmName := deployment.GenerateConfigMapName("my-agent", "build-001")
	cm := r.getConfigMap(t, ns, cmName)

	// Container-mode model env should be wired
	if host, ok := cm.Data["MODEL_LLM_HOST"]; !ok || !strings.Contains(host, "my-agent-model-llm") {
		t.Errorf("expected MODEL_LLM_HOST wired to model service DNS, got %q", cm.Data["MODEL_LLM_HOST"])
	}
	// QDRANT env should be wired
	if host, ok := cm.Data["QDRANT_HOST"]; !ok || !strings.Contains(host, "my-agent-knowledge-docs") {
		t.Errorf("expected QDRANT_HOST wired to knowledge service DNS, got %q", cm.Data["QDRANT_HOST"])
	}
	// REDIS env should be wired
	if host, ok := cm.Data["REDIS_HOST"]; !ok || !strings.Contains(host, "my-agent-knowledge-cache") {
		t.Errorf("expected REDIS_HOST wired to knowledge service DNS, got %q", cm.Data["REDIS_HOST"])
	}
	// Platform metadata
	if cm.Data["ASTRO_AGENT_NAME"] != "my-agent" {
		t.Errorf("expected ASTRO_AGENT_NAME=my-agent, got %q", cm.Data["ASTRO_AGENT_NAME"])
	}

	// Verify Secret data
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	expectedSecretKeys := []string{"ANTHROPIC_API_KEY", "GITHUB_TOKEN", "JIRA_TOKEN"}
	for _, key := range expectedSecretKeys {
		if _, ok := secret.Data[key]; !ok {
			t.Errorf("expected %s in secret, got keys: %v", key, keysOf(secret.Data))
		}
	}

	// Verify resource counts
	if got := r.resourceCount("Service"); got < 4 {
		t.Errorf("expected at least 4 Services, got %d", got)
	}
	if got := r.resourceCount("StatefulSet"); got != 3 {
		t.Errorf("expected 3 StatefulSets (agent + qdrant + redis), got %d", got)
	}
}

// --- provider env wiring tests ---

// assertConfigMapValues checks that every key in want exists in the ConfigMap with the expected value.
func assertConfigMapValues(t *testing.T, r *e2eResult, want map[string]string) {
	t.Helper()
	ns := r.Namespace
	cmName := deployment.GenerateConfigMapName(r.DeploymentSpec.Source.Name, r.DeploymentSpec.Source.Build)
	cm := r.getConfigMap(t, ns, cmName)
	for key, expected := range want {
		got, ok := cm.Data[key]
		if !ok {
			t.Errorf("ConfigMap missing key %s; available keys: %v", key, configMapKeys(cm))
			continue
		}
		if got != expected {
			t.Errorf("ConfigMap[%s] = %q, want %q", key, got, expected)
		}
	}
}

// assertConfigMapAbsent checks that none of the given keys exist in the ConfigMap.
func assertConfigMapAbsent(t *testing.T, r *e2eResult, keys []string) {
	t.Helper()
	ns := r.Namespace
	cmName := deployment.GenerateConfigMapName(r.DeploymentSpec.Source.Name, r.DeploymentSpec.Source.Build)
	cm := r.getConfigMap(t, ns, cmName)
	for _, key := range keys {
		if _, ok := cm.Data[key]; ok {
			t.Errorf("ConfigMap should not contain key %s", key)
		}
	}
}

func configMapKeys(cm *corev1.ConfigMap) []string {
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	return keys
}

// assertSecretValues checks that the given key-value pairs exist in the K8s Secret.
func assertSecretValues(t *testing.T, r *e2eResult, want map[string]string) {
	t.Helper()
	ns := r.Namespace
	secretName := deployment.GenerateSecretName(r.DeploymentSpec.Source.Name, r.DeploymentSpec.Source.Build)
	secret := r.getSecret(t, ns, secretName)
	for key, expected := range want {
		got, ok := secret.Data[key]
		if !ok {
			t.Errorf("Secret missing key %s; available keys: %v", key, keysOf(secret.Data))
			continue
		}
		if string(got) != expected {
			t.Errorf("Secret[%s] = %q, want %q", key, string(got), expected)
		}
	}
}

// serviceDNS is a shorthand to build the expected k8s service DNS.
func serviceDNS(name, ns string) string {
	return name + "." + ns + ".svc.cluster.local"
}

// effectiveAgentEnv returns the merged env the agent's "app" container will
// see at runtime: envFrom Secret + envFrom ConfigMap + container.env, with
// container.env overriding envFrom on key collisions (matches k8s precedence).
// secretKeyRef / configMapKeyRef in container.env are resolved against the
// stored Secrets/ConfigMaps. Used to verify that every expected env var is
// reachable regardless of which path put it there.
func effectiveAgentEnv(t *testing.T, r *e2eResult) map[string]string {
	t.Helper()
	ns := r.Namespace
	agentName := r.DeploymentSpec.Source.Name
	deplName := deployment.GenerateAgentResourceName(agentName, "agent")
	depl, err := r.Clientset.AppsV1().StatefulSets(ns).Get(context.Background(), deplName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get agent statefulset %q: %v", deplName, err)
	}
	var app *corev1.Container
	for i := range depl.Spec.Template.Spec.Containers {
		c := &depl.Spec.Template.Spec.Containers[i]
		if c.Name == "app" {
			app = c
			break
		}
	}
	if app == nil {
		t.Fatalf("agent app container not found in deployment %q", deplName)
	}

	env := map[string]string{}

	// Phase 1: envFrom — Secret and ConfigMap data merged into env.
	for _, src := range app.EnvFrom {
		if src.SecretRef != nil {
			s, err := r.Clientset.CoreV1().Secrets(ns).Get(context.Background(), src.SecretRef.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("envFrom secret %q: %v", src.SecretRef.Name, err)
			}
			for k, v := range s.Data {
				env[k] = string(v)
			}
		}
		if src.ConfigMapRef != nil {
			cm, err := r.Clientset.CoreV1().ConfigMaps(ns).Get(context.Background(), src.ConfigMapRef.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("envFrom configmap %q: %v", src.ConfigMapRef.Name, err)
			}
			for k, v := range cm.Data {
				env[k] = v
			}
		}
	}

	// Phase 2: env entries — override envFrom on key collisions.
	for _, e := range app.Env {
		switch {
		case e.ValueFrom == nil:
			env[e.Name] = e.Value
		case e.ValueFrom.SecretKeyRef != nil:
			ref := e.ValueFrom.SecretKeyRef
			s, err := r.Clientset.CoreV1().Secrets(ns).Get(context.Background(), ref.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("env %q secretKeyRef %s/%s: %v", e.Name, ref.Name, ref.Key, err)
			}
			val, ok := s.Data[ref.Key]
			if !ok {
				t.Fatalf("env %q secretKeyRef %s/%s: key not found in secret", e.Name, ref.Name, ref.Key)
			}
			env[e.Name] = string(val)
		case e.ValueFrom.ConfigMapKeyRef != nil:
			ref := e.ValueFrom.ConfigMapKeyRef
			cm, err := r.Clientset.CoreV1().ConfigMaps(ns).Get(context.Background(), ref.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("env %q configMapKeyRef %s/%s: %v", e.Name, ref.Name, ref.Key, err)
			}
			val, ok := cm.Data[ref.Key]
			if !ok {
				t.Fatalf("env %q configMapKeyRef %s/%s: key not found in configmap", e.Name, ref.Name, ref.Key)
			}
			env[e.Name] = val
			// Other ValueFrom kinds (FieldRef, ResourceFieldRef) are not used
			// by the agent template path; ignore until they are.
		}
	}

	return env
}

func TestE2E_ProviderEnv_ContainerModel(t *testing.T) {
	// Container-mode model (no provider) → generic MODEL_{NAME}_ prefix
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  custom-llm:
    container:
      image: my-model:latest
      port: 5000
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-model-custom-llm", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"MODEL_CUSTOM_LLM_HOST": host,
		"MODEL_CUSTOM_LLM_PORT": "5000",
		"MODEL_CUSTOM_LLM_URL":  "http://" + host + ":5000",
	})

	// Should NOT have an unprefixed/bare MODEL_HOST — container models are always name-qualified
	assertConfigMapAbsent(t, r, []string{"MODEL_HOST", "MODEL_PORT"})
}

func TestE2E_ProviderEnv_CloudModelOpenAI(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  gpt:
    provider: openai
`, e2eOpts{
		Credentials: map[string]string{
			"OPENAI_API_KEY": "sk-openai-test",
		},
	})

	requireNoErrors(t, r)

	// Cloud provider → credential in Secret only (not ConfigMap)
	assertSecretValues(t, r, map[string]string{
		"OPENAI_API_KEY": "sk-openai-test",
	})

	// No container env vars for cloud provider, and credential should not be in ConfigMap
	assertConfigMapAbsent(t, r, []string{"OPENAI_HOST", "OPENAI_PORT", "MODEL_GPT_HOST", "OPENAI_API_KEY"})
}

func TestE2E_ProviderEnv_CloudModelMultiple(t *testing.T) {
	// Multiple cloud model providers → each gets its own provider-prefixed credential key
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  claude:
    provider: anthropic
  gpt:
    provider: openai
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_API_KEY": "sk-ant-123",
			"OPENAI_API_KEY":    "sk-openai-456",
		},
	})

	requireNoErrors(t, r)

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	// Each cloud provider gets a {PROVIDER}_API_KEY credential
	if string(secret.Data["ANTHROPIC_API_KEY"]) != "sk-ant-123" {
		t.Errorf("expected ANTHROPIC_API_KEY in secret")
	}
	if string(secret.Data["OPENAI_API_KEY"]) != "sk-openai-456" {
		t.Errorf("expected OPENAI_API_KEY in secret")
	}

	// Neither should create container resources
	if r.hasResource("Deployment", "my-agent-model-claude") || r.hasResource("Deployment", "my-agent-model-gpt") {
		t.Error("did not expect Deployments for cloud model providers")
	}
}

func TestE2E_ProviderEnv_KnowledgeQdrant(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  docs:
    provider: qdrant
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-docs", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"QDRANT_HOST": host,
		"QDRANT_PORT": "6333",
		"QDRANT_URL":  "http://" + host + ":6333", // URLScheme=http
	})
}

func TestE2E_ProviderEnv_KnowledgeRedis(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  cache:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-cache", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"REDIS_HOST": host,
		"REDIS_PORT": "6379",
		"REDIS_URL":  "redis://" + host + ":6379", // URLScheme=redis
	})
}

func TestE2E_ProviderEnv_KnowledgePostgres(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  db:
    provider: postgres
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-db", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"POSTGRES_HOST": host,
		"POSTGRES_PORT": "5432",
	})

	// Postgres has no URLScheme → no POSTGRES_URL
	assertConfigMapAbsent(t, r, []string{"POSTGRES_URL"})
}

func TestE2E_ProviderEnv_KnowledgeNeo4j(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  graph:
    provider: neo4j
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-graph", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"NEO4J_HOST": host,
		"NEO4J_PORT": "7474",
		"NEO4J_URL":  "bolt://" + host + ":7474", // URLScheme=bolt
	})
}

func TestE2E_KnowledgeService_ExposesAllProviderPorts(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  graph:
    provider: neo4j
  docs:
    provider: qdrant
  cache:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)

	ctx := context.Background()

	// Neo4j: Service must expose both http (7474) and bolt (7687)
	neo4jSvc, err := r.Clientset.CoreV1().Services(r.Namespace).Get(ctx,
		deployment.GenerateResourceName("my-agent", "knowledge", "graph"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("neo4j service not found: %v", err)
	}
	neo4jPorts := make(map[string]int32)
	for _, p := range neo4jSvc.Spec.Ports {
		neo4jPorts[p.Name] = p.Port
	}
	if neo4jPorts["http"] != 7474 {
		t.Errorf("neo4j service: http port = %d, want 7474", neo4jPorts["http"])
	}
	if neo4jPorts["bolt"] != 7687 {
		t.Errorf("neo4j service: bolt port = %d, want 7687", neo4jPorts["bolt"])
	}

	// Qdrant: Service must expose both http (6333) and grpc (6334)
	qdrantSvc, err := r.Clientset.CoreV1().Services(r.Namespace).Get(ctx,
		deployment.GenerateResourceName("my-agent", "knowledge", "docs"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("qdrant service not found: %v", err)
	}
	qdrantPorts := make(map[string]int32)
	for _, p := range qdrantSvc.Spec.Ports {
		qdrantPorts[p.Name] = p.Port
	}
	if qdrantPorts["http"] != 6333 {
		t.Errorf("qdrant service: http port = %d, want 6333", qdrantPorts["http"])
	}
	if qdrantPorts["grpc"] != 6334 {
		t.Errorf("qdrant service: grpc port = %d, want 6334", qdrantPorts["grpc"])
	}

	// Redis: single port only (no extra ports)
	redisSvc, err := r.Clientset.CoreV1().Services(r.Namespace).Get(ctx,
		deployment.GenerateResourceName("my-agent", "knowledge", "cache"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("redis service not found: %v", err)
	}
	if len(redisSvc.Spec.Ports) != 1 {
		t.Errorf("redis service: expected 1 port, got %d", len(redisSvc.Spec.Ports))
	}
	if redisSvc.Spec.Ports[0].Port != 6379 {
		t.Errorf("redis service: port = %d, want 6379", redisSvc.Spec.Ports[0].Port)
	}
}

func TestE2E_ProviderEnv_KnowledgeContainer(t *testing.T) {
	// Container-mode knowledge (no provider) → generic KNOWLEDGE_{NAME}_ prefix
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  mydb:
    container:
      image: my-db:latest
      port: 9200
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-mydb", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"KNOWLEDGE_MYDB_HOST": host,
		"KNOWLEDGE_MYDB_PORT": "9200",
	})

	// Container-mode has no URLScheme → no _URL generated in template
	assertConfigMapAbsent(t, r, []string{"KNOWLEDGE_MYDB_URL"})
}

func TestE2E_ProviderEnv_CloudKnowledgePinecone(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  vectors:
    provider: pinecone
`, e2eOpts{
		Credentials: map[string]string{
			"PINECONE_API_KEY": "pc-test-key",
		},
	})

	requireNoErrors(t, r)

	// Cloud knowledge → no container
	if r.hasResource("Deployment", "my-agent-knowledge-vectors") || r.hasResource("StatefulSet", "my-agent-knowledge-vectors") {
		t.Error("did not expect container resources for cloud knowledge provider")
	}

	// Credential in Secret
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["PINECONE_API_KEY"]) != "pc-test-key" {
		t.Errorf("expected PINECONE_API_KEY in secret")
	}

	// No container env vars
	assertConfigMapAbsent(t, r, []string{"PINECONE_HOST", "PINECONE_PORT"})
}

func TestE2E_ProviderEnv_ContainerIntegration(t *testing.T) {
	// Container-mode integration → INTEGRATION_{NAME}_ prefix (§8.3)
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  search:
    container:
      image: my-search:latest
      port: 3000
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-integration-search", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"INTEGRATION_SEARCH_HOST": host,
		"INTEGRATION_SEARCH_PORT": "3000",
		"INTEGRATION_SEARCH_URL":  "http://" + host + ":3000",
	})
}

func TestE2E_ProviderEnv_CloudIntegrationGitLab(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  gitlab:
    provider: gitlab
`, e2eOpts{
		Credentials: map[string]string{
			"GITLAB_TOKEN": "glpat-test",
		},
	})

	requireNoErrors(t, r)

	// Cloud integration → no container
	if r.hasResource("Deployment", "my-agent-integration-gitlab") {
		t.Error("did not expect Deployment for cloud integration provider")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["GITLAB_TOKEN"]) != "glpat-test" {
		t.Errorf("expected GITLAB_TOKEN in secret")
	}

	// No container env vars
	assertConfigMapAbsent(t, r, []string{"INTEGRATION_GITLAB_HOST", "INTEGRATION_GITLAB_PORT"})
}

func TestE2E_ProviderEnv_PlatformMetadata(t *testing.T) {
	// Every deployment gets ASTRO_AGENT_NAME, ASTRO_AGENT_BUILD, ASTRO_AGENT_URL, OTEL endpoint
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
`, e2eOpts{})

	requireNoErrors(t, r)

	agentHost := serviceDNS("my-agent-agent", "test-ns")
	collectorHost := serviceDNS("my-agent-collector", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"ASTRO_AGENT_NAME":            "my-agent",
		"ASTRO_AGENT_BUILD":           "build-001",
		"ASTRO_AGENT_URL":             "http://" + agentHost + ":8080",
		"ASTRO_AGENT_HOST":            agentHost,
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://" + collectorHost + ":4318",
	})
}

func TestE2E_ProviderEnv_MixedModelAndKnowledge(t *testing.T) {
	// Container-mode model + qdrant knowledge + redis cache — all env vars coexist
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    container:
      image: my-model:latest
      port: 8000
knowledge:
  docs:
    provider: qdrant
  cache:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)

	llmHost := serviceDNS("my-agent-model-llm", "test-ns")
	qdrantHost := serviceDNS("my-agent-knowledge-docs", "test-ns")
	redisHost := serviceDNS("my-agent-knowledge-cache", "test-ns")

	assertConfigMapValues(t, r, map[string]string{
		// Container-mode model
		"MODEL_LLM_HOST": llmHost,
		"MODEL_LLM_PORT": "8000",
		"MODEL_LLM_URL":  "http://" + llmHost + ":8000",
		// Qdrant
		"QDRANT_HOST": qdrantHost,
		"QDRANT_PORT": "6333",
		"QDRANT_URL":  "http://" + qdrantHost + ":6333",
		// Redis
		"REDIS_HOST": redisHost,
		"REDIS_PORT": "6379",
		"REDIS_URL":  "redis://" + redisHost + ":6379",
	})
}

func TestE2E_ProviderEnv_CredentialResolvedValues(t *testing.T) {
	// Credential references resolve to the actual credential value in the ConfigMap
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  claude:
    provider: anthropic
integrations:
  github:
    provider: github
  slack:
    provider: slack-provider
providers:
  slack-provider:
    scope: [integrations]
    variables:
      - name: WEBHOOK_URL
        datatype: string
        secret: true
        description: "Slack webhook"
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_API_KEY":          "sk-ant-real",
			"GITHUB_TOKEN":               "ghp_real",
			"SLACK_PROVIDER_WEBHOOK_URL": "https://hooks.slack.com/test",
		},
	})

	requireNoErrors(t, r)

	// All credential values should be in the Secret only (not ConfigMap)
	assertSecretValues(t, r, map[string]string{
		"ANTHROPIC_API_KEY":          "sk-ant-real",
		"GITHUB_TOKEN":               "ghp_real",
		"SLACK_PROVIDER_WEBHOOK_URL": "https://hooks.slack.com/test",
	})
	assertConfigMapAbsent(t, r, []string{
		"ANTHROPIC_API_KEY", "GITHUB_TOKEN", "SLACK_PROVIDER_WEBHOOK_URL",
	})
}

// --- duplicate provider tests ---

func TestE2E_ProviderEnv_TwoQdrantKnowledge(t *testing.T) {
	// Two knowledge stores using the same provider (qdrant). Both get their own
	// Service + StatefulSet. The bare QDRANT_* prefix points to the first entry
	// alphabetically ("primary"), and name-qualified vars exist for all entries.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  primary:
    provider: qdrant
  secondary:
    provider: qdrant
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both get their own StatefulSet + Service
	if !r.hasResource("StatefulSet", "my-agent-knowledge-primary") {
		t.Error("expected StatefulSet for primary qdrant")
	}
	if !r.hasResource("StatefulSet", "my-agent-knowledge-secondary") {
		t.Error("expected StatefulSet for secondary qdrant")
	}
	if !r.hasResource("Service", "my-agent-knowledge-primary") {
		t.Error("expected Service for primary qdrant")
	}
	if !r.hasResource("Service", "my-agent-knowledge-secondary") {
		t.Error("expected Service for secondary qdrant")
	}

	ns := r.Namespace
	primaryDNS := serviceDNS("my-agent-knowledge-primary", ns)
	secondaryDNS := serviceDNS("my-agent-knowledge-secondary", ns)

	// Bare prefix points to first alphabetically (primary)
	// Name-qualified vars exist for all entries
	assertConfigMapValues(t, r, map[string]string{
		"QDRANT_HOST":           primaryDNS,
		"QDRANT_PORT":           "6333",
		"QDRANT_URL":            "http://" + primaryDNS + ":6333",
		"QDRANT_PRIMARY_HOST":   primaryDNS,
		"QDRANT_PRIMARY_PORT":   "6333",
		"QDRANT_PRIMARY_URL":    "http://" + primaryDNS + ":6333",
		"QDRANT_SECONDARY_HOST": secondaryDNS,
		"QDRANT_SECONDARY_PORT": "6333",
		"QDRANT_SECONDARY_URL":  "http://" + secondaryDNS + ":6333",
	})
}

func TestE2E_ProviderEnv_TwoContainerModels(t *testing.T) {
	// Two container-mode models. Each gets its own Service + Deployment and
	// name-qualified MODEL_<NAME>_* env vars (container models have no shared/bare prefix).
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  fast:
    container:
      image: fast-model:latest
      port: 8000
  big:
    container:
      image: big-model:latest
      port: 8000
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both deploy as Deployments with their own Service (models deploy as Deployments)
	if !r.hasResource("Deployment", "my-agent-model-fast") {
		t.Error("expected Deployment for fast model")
	}
	if !r.hasResource("Deployment", "my-agent-model-big") {
		t.Error("expected Deployment for big model")
	}
	if !r.hasResource("Service", "my-agent-model-fast") {
		t.Error("expected Service for fast model")
	}
	if !r.hasResource("Service", "my-agent-model-big") {
		t.Error("expected Service for big model")
	}

	ns := r.Namespace
	bigDNS := serviceDNS("my-agent-model-big", ns)
	fastDNS := serviceDNS("my-agent-model-fast", ns)

	// Each container model is name-qualified; no bare/shared prefix.
	assertConfigMapValues(t, r, map[string]string{
		"MODEL_BIG_HOST":  bigDNS,
		"MODEL_BIG_PORT":  "8000",
		"MODEL_BIG_URL":   "http://" + bigDNS + ":8000",
		"MODEL_FAST_HOST": fastDNS,
		"MODEL_FAST_PORT": "8000",
		"MODEL_FAST_URL":  "http://" + fastDNS + ":8000",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsSameProvider(t *testing.T) {
	// Two models with provider:anthropic and neither name matches the
	// provider. Per §8.1: qualified keys only, NO bare ANTHROPIC_API_KEY.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  claude-haiku:
    provider: anthropic
  claude-opus:
    provider: anthropic
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_CLAUDE_HAIKU_API_KEY": "sk-haiku",
			"ANTHROPIC_CLAUDE_OPUS_API_KEY":  "sk-opus",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-claude-haiku") || r.hasResource("Deployment", "my-agent-model-claude-opus") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["ANTHROPIC_CLAUDE_HAIKU_API_KEY"]) != "sk-haiku" {
		t.Errorf("expected ANTHROPIC_CLAUDE_HAIKU_API_KEY=sk-haiku in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["ANTHROPIC_CLAUDE_OPUS_API_KEY"]) != "sk-opus" {
		t.Errorf("expected ANTHROPIC_CLAUDE_OPUS_API_KEY=sk-opus in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["ANTHROPIC_API_KEY"]; hasBare {
		t.Error("bare ANTHROPIC_API_KEY must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"ANTHROPIC_CLAUDE_HAIKU_API_KEY", "ANTHROPIC_CLAUDE_OPUS_API_KEY",
	})
}

func TestE2E_ProviderEnv_TwoCloudIntegrationsSameProvider(t *testing.T) {
	// Two integrations with provider:github and neither name matches the
	// provider. Per §8.1: qualified keys only, NO bare GITHUB_TOKEN.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  gh-main:
    provider: github
  gh-secondary:
    provider: github
`, e2eOpts{
		Credentials: map[string]string{
			"GITHUB_GH_MAIN_TOKEN":      "ghp_main",
			"GITHUB_GH_SECONDARY_TOKEN": "ghp_secondary",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-integration-gh-main") || r.hasResource("Deployment", "my-agent-integration-gh-secondary") {
		t.Error("did not expect Deployments for cloud integrations")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GITHUB_GH_MAIN_TOKEN"]) != "ghp_main" {
		t.Errorf("expected GITHUB_GH_MAIN_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITHUB_GH_SECONDARY_TOKEN"]) != "ghp_secondary" {
		t.Errorf("expected GITHUB_GH_SECONDARY_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["GITHUB_TOKEN"]; hasBare {
		t.Error("bare GITHUB_TOKEN must not appear when neither entry name matches the provider (§8.1)")
	}
}

func TestE2E_ProviderEnv_TwoCloudModelsSameProviderNameMatch(t *testing.T) {
	// When one entry's name matches the provider, it becomes the natural primary:
	// bare key only (no redundant ANTHROPIC_ANTHROPIC_API_KEY), others get name-qualified.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  anthropic:
    provider: anthropic
  fallback:
    provider: anthropic
`, e2eOpts{
		Credentials: map[string]string{
			"ANTHROPIC_API_KEY":          "sk-primary",
			"ANTHROPIC_FALLBACK_API_KEY": "sk-fallback",
		},
	})

	requireNoErrors(t, r)

	// No containers for cloud providers
	if r.hasResource("Deployment", "my-agent-model-anthropic") || r.hasResource("Deployment", "my-agent-model-fallback") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	// Only 2 keys: bare + name-qualified for fallback (no ANTHROPIC_ANTHROPIC_API_KEY)
	if string(secret.Data["ANTHROPIC_API_KEY"]) != "sk-primary" {
		t.Errorf("expected ANTHROPIC_API_KEY=sk-primary in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["ANTHROPIC_FALLBACK_API_KEY"]) != "sk-fallback" {
		t.Errorf("expected ANTHROPIC_FALLBACK_API_KEY=sk-fallback in secret, got keys: %v", keysOf(secret.Data))
	}

	// Should NOT have the redundant ANTHROPIC_ANTHROPIC_API_KEY or credentials in ConfigMap
	assertConfigMapAbsent(t, r, []string{
		"ANTHROPIC_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_FALLBACK_API_KEY",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsOpenAI(t *testing.T) {
	// Two models with provider:openai, neither name matches. Qualified only.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  gpt-fast:
    provider: openai
  gpt-smart:
    provider: openai
`, e2eOpts{
		Credentials: map[string]string{
			"OPENAI_GPT_FAST_API_KEY":  "sk-fast",
			"OPENAI_GPT_SMART_API_KEY": "sk-smart",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-gpt-fast") || r.hasResource("Deployment", "my-agent-model-gpt-smart") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["OPENAI_GPT_FAST_API_KEY"]) != "sk-fast" {
		t.Errorf("expected OPENAI_GPT_FAST_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["OPENAI_GPT_SMART_API_KEY"]) != "sk-smart" {
		t.Errorf("expected OPENAI_GPT_SMART_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["OPENAI_API_KEY"]; hasBare {
		t.Error("bare OPENAI_API_KEY must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"OPENAI_GPT_FAST_API_KEY", "OPENAI_GPT_SMART_API_KEY",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsGoogle(t *testing.T) {
	// Two models with provider:google, neither name matches. Qualified only.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  gemini-flash:
    provider: google
  gemini-pro:
    provider: google
`, e2eOpts{
		Credentials: map[string]string{
			"GOOGLE_GEMINI_FLASH_API_KEY": "goog-flash",
			"GOOGLE_GEMINI_PRO_API_KEY":   "goog-pro",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-gemini-flash") || r.hasResource("Deployment", "my-agent-model-gemini-pro") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GOOGLE_GEMINI_FLASH_API_KEY"]) != "goog-flash" {
		t.Errorf("expected GOOGLE_GEMINI_FLASH_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GOOGLE_GEMINI_PRO_API_KEY"]) != "goog-pro" {
		t.Errorf("expected GOOGLE_GEMINI_PRO_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["GOOGLE_API_KEY"]; hasBare {
		t.Error("bare GOOGLE_API_KEY must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"GOOGLE_GEMINI_FLASH_API_KEY", "GOOGLE_GEMINI_PRO_API_KEY",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsCohere(t *testing.T) {
	// Two models with provider:cohere, neither name matches. Qualified only.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  embed:
    provider: cohere
  rerank:
    provider: cohere
`, e2eOpts{
		Credentials: map[string]string{
			"COHERE_EMBED_API_KEY":  "co-embed",
			"COHERE_RERANK_API_KEY": "co-rerank",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-embed") || r.hasResource("Deployment", "my-agent-model-rerank") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["COHERE_EMBED_API_KEY"]) != "co-embed" {
		t.Errorf("expected COHERE_EMBED_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["COHERE_RERANK_API_KEY"]) != "co-rerank" {
		t.Errorf("expected COHERE_RERANK_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["COHERE_API_KEY"]; hasBare {
		t.Error("bare COHERE_API_KEY must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"COHERE_EMBED_API_KEY", "COHERE_RERANK_API_KEY",
	})
}

func TestE2E_ProviderEnv_TwoCloudKnowledgePinecone(t *testing.T) {
	// Two knowledge stores with provider:pinecone, neither name matches.
	// Qualified only — no bare PINECONE_API_KEY.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  embeddings:
    provider: pinecone
  search:
    provider: pinecone
`, e2eOpts{
		Credentials: map[string]string{
			"PINECONE_EMBEDDINGS_API_KEY": "pc-embed",
			"PINECONE_SEARCH_API_KEY":     "pc-search",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-knowledge-embeddings") || r.hasResource("StatefulSet", "my-agent-knowledge-embeddings") {
		t.Error("did not expect container resources for cloud knowledge")
	}
	if r.hasResource("Deployment", "my-agent-knowledge-search") || r.hasResource("StatefulSet", "my-agent-knowledge-search") {
		t.Error("did not expect container resources for cloud knowledge")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["PINECONE_EMBEDDINGS_API_KEY"]) != "pc-embed" {
		t.Errorf("expected PINECONE_EMBEDDINGS_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["PINECONE_SEARCH_API_KEY"]) != "pc-search" {
		t.Errorf("expected PINECONE_SEARCH_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["PINECONE_API_KEY"]; hasBare {
		t.Error("bare PINECONE_API_KEY must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"PINECONE_EMBEDDINGS_API_KEY", "PINECONE_SEARCH_API_KEY",
		"PINECONE_HOST", "PINECONE_PORT",
	})
}

func TestE2E_ProviderEnv_TwoCloudIntegrationsGitLab(t *testing.T) {
	// Two integrations with provider:gitlab, neither name matches. Qualified only.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  gl-main:
    provider: gitlab
  gl-deploy:
    provider: gitlab
`, e2eOpts{
		Credentials: map[string]string{
			"GITLAB_GL_DEPLOY_TOKEN": "glpat-deploy",
			"GITLAB_GL_MAIN_TOKEN":   "glpat-main",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-integration-gl-main") || r.hasResource("Deployment", "my-agent-integration-gl-deploy") {
		t.Error("did not expect Deployments for cloud integrations")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GITLAB_GL_DEPLOY_TOKEN"]) != "glpat-deploy" {
		t.Errorf("expected GITLAB_GL_DEPLOY_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITLAB_GL_MAIN_TOKEN"]) != "glpat-main" {
		t.Errorf("expected GITLAB_GL_MAIN_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if _, hasBare := secret.Data["GITLAB_TOKEN"]; hasBare {
		t.Error("bare GITLAB_TOKEN must not appear when neither entry name matches the provider (§8.1)")
	}

	assertConfigMapAbsent(t, r, []string{
		"GITLAB_GL_DEPLOY_TOKEN", "GITLAB_GL_MAIN_TOKEN",
	})
}

func TestE2E_ProviderEnv_TwoRedisKnowledge(t *testing.T) {
	// Two redis knowledge stores. Bare REDIS_* points to first alphabetically
	// ("durable"), name-qualified vars for all.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  sessions:
    provider: redis
  durable:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both redis entries → StatefulSet (provider has MountPath)
	if !r.hasResource("StatefulSet", "my-agent-knowledge-sessions") {
		t.Error("expected StatefulSet for sessions redis")
	}
	if !r.hasResource("StatefulSet", "my-agent-knowledge-durable") {
		t.Error("expected StatefulSet for durable redis")
	}

	// Both get their own Service
	if !r.hasResource("Service", "my-agent-knowledge-sessions") {
		t.Error("expected Service for sessions redis")
	}
	if !r.hasResource("Service", "my-agent-knowledge-durable") {
		t.Error("expected Service for durable redis")
	}

	ns := r.Namespace
	durableDNS := serviceDNS("my-agent-knowledge-durable", ns)
	sessionsDNS := serviceDNS("my-agent-knowledge-sessions", ns)

	// Bare prefix points to first alphabetically ("durable")
	// Name-qualified vars exist for all entries
	assertConfigMapValues(t, r, map[string]string{
		"REDIS_HOST":          durableDNS,
		"REDIS_PORT":          "6379",
		"REDIS_URL":           "redis://" + durableDNS + ":6379",
		"REDIS_DURABLE_HOST":  durableDNS,
		"REDIS_DURABLE_PORT":  "6379",
		"REDIS_DURABLE_URL":   "redis://" + durableDNS + ":6379",
		"REDIS_SESSIONS_HOST": sessionsDNS,
		"REDIS_SESSIONS_PORT": "6379",
		"REDIS_SESSIONS_URL":  "redis://" + sessionsDNS + ":6379",
	})
}

func TestE2E_ProviderEnv_TwoPostgresKnowledge(t *testing.T) {
	// Two knowledge stores using the same provider (postgres). Bare POSTGRES_*
	// points to the first entry alphabetically ("analytics"), name-qualified vars for all.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  analytics:
    provider: postgres
  users:
    provider: postgres
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both get StatefulSets (persistent)
	if !r.hasResource("StatefulSet", "my-agent-knowledge-analytics") {
		t.Error("expected StatefulSet for analytics postgres")
	}
	if !r.hasResource("StatefulSet", "my-agent-knowledge-users") {
		t.Error("expected StatefulSet for users postgres")
	}
	if !r.hasResource("Service", "my-agent-knowledge-analytics") {
		t.Error("expected Service for analytics postgres")
	}
	if !r.hasResource("Service", "my-agent-knowledge-users") {
		t.Error("expected Service for users postgres")
	}

	ns := r.Namespace
	analyticsDNS := serviceDNS("my-agent-knowledge-analytics", ns)
	usersDNS := serviceDNS("my-agent-knowledge-users", ns)

	// Bare prefix → first alphabetically ("analytics"); name-qualified for all.
	// DB lands in the deploy Secret via the credentials-ref path — see
	// TestE2E_PostgresKnowledge_CredentialFlow which asserts that.
	// Postgres has no URLScheme → no _URL vars
	assertConfigMapValues(t, r, map[string]string{
		"POSTGRES_HOST":           analyticsDNS,
		"POSTGRES_PORT":           "5432",
		"POSTGRES_ANALYTICS_HOST": analyticsDNS,
		"POSTGRES_ANALYTICS_PORT": "5432",
		"POSTGRES_USERS_HOST":     usersDNS,
		"POSTGRES_USERS_PORT":     "5432",
	})

	// Postgres has no URLScheme → no URL vars at all
	assertConfigMapAbsent(t, r, []string{
		"POSTGRES_URL", "POSTGRES_ANALYTICS_URL", "POSTGRES_USERS_URL",
	})
}

// When two postgres knowledge stores exist, their auto-generated cred
// secrets share the literal keys POSTGRES_USER / POSTGRES_PASSWORD.
// envFrom-mounting both onto the agent collapses them silently — only one
// store's credentials survive at runtime.
//
// Fix: the agent must NOT envFrom the cred secrets. Instead each (store,
// suffix) pair is wired onto the agent via secretKeyRef under an RFC §8.2
// name (POSTGRES_USER for the matching entry; POSTGRES_USERS_USER for
// others). This test asserts both halves: no envFrom, and per-store named
// secretKeyRef env vars on the agent container.
func TestE2E_ProviderEnv_TwoPostgresKnowledge_AgentHasPerStoreCreds(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  postgres:
    provider: postgres
  users:
    provider: postgres
`, e2eOpts{})

	requireNoErrors(t, r)

	// Fetch the agent StatefulSet and locate the "app" container.
	depl, err := r.Clientset.AppsV1().StatefulSets(r.Namespace).Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get agent StatefulSet: %v", err)
	}
	var app *corev1.Container
	for i := range depl.Spec.Template.Spec.Containers {
		c := &depl.Spec.Template.Spec.Containers[i]
		if c.Name == "app" {
			app = c
			break
		}
	}
	if app == nil {
		t.Fatal("agent app container not found")
	}

	// envFrom MUST NOT include any *-knowledge-*-creds secret — that's the
	// silent-collision path we're moving away from.
	for _, ef := range app.EnvFrom {
		if ef.SecretRef == nil {
			continue
		}
		name := ef.SecretRef.Name
		if strings.Contains(name, "knowledge-") && strings.HasSuffix(name, "-creds") {
			t.Errorf("agent must not envFrom knowledge cred secret %q (use secretKeyRef per env var instead)", name)
		}
	}

	// Each store's credentials must be on the agent with RFC §8.2-correct
	// names, bound via secretKeyRef to the literal keys in the right secret.
	wantRefs := map[string]struct {
		secret string
		key    string
	}{
		"POSTGRES_USER":           {"my-agent-knowledge-postgres-creds", "POSTGRES_USER"},
		"POSTGRES_PASSWORD":       {"my-agent-knowledge-postgres-creds", "POSTGRES_PASSWORD"},
		"POSTGRES_USERS_USER":     {"my-agent-knowledge-users-creds", "POSTGRES_USER"},
		"POSTGRES_USERS_PASSWORD": {"my-agent-knowledge-users-creds", "POSTGRES_PASSWORD"},
	}
	got := map[string]corev1.EnvVar{}
	for _, e := range app.Env {
		got[e.Name] = e
	}
	for name, want := range wantRefs {
		ev, ok := got[name]
		if !ok {
			t.Errorf("agent missing env var %q", name)
			continue
		}
		if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
			t.Errorf("env %q: expected SecretKeyRef, got %+v", name, ev)
			continue
		}
		ref := ev.ValueFrom.SecretKeyRef
		if ref.Name != want.secret || ref.Key != want.key {
			t.Errorf("env %q: ref = (%s,%s), want (%s,%s)", name, ref.Name, ref.Key, want.secret, want.key)
		}
	}

	// The redundant qualified form MUST NOT be emitted for the matching entry.
	for _, bad := range []string{"POSTGRES_POSTGRES_USER", "POSTGRES_POSTGRES_PASSWORD"} {
		if _, present := got[bad]; present {
			t.Errorf("redundant env var %q must not be emitted (entry name matches provider)", bad)
		}
	}
}

// When one entry's name matches the provider name ("postgres"), it gets the
// bare key only — the redundant qualified form (POSTGRES_POSTGRES_HOST) MUST
// NOT be emitted. The other entry gets only its qualified form. This is
// exactly RFC §8.2's "name == provider → omit qualified form" rule.
func TestE2E_ProviderEnv_TwoPostgresKnowledge_NameMatchesProvider(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  postgres:
    provider: postgres
  users:
    provider: postgres
`, e2eOpts{})

	requireNoErrors(t, r)

	ns := r.Namespace
	postgresDNS := serviceDNS("my-agent-knowledge-postgres", ns)
	usersDNS := serviceDNS("my-agent-knowledge-users", ns)

	// `postgres` entry matches provider → bare keys only.
	// `users` entry → qualified keys only.
	// DB lands in deploy Secret via credentials-ref path; this test only
	// covers the ConfigMap-routed connection details (HOST/PORT).
	assertConfigMapValues(t, r, map[string]string{
		"POSTGRES_HOST":       postgresDNS,
		"POSTGRES_PORT":       "5432",
		"POSTGRES_USERS_HOST": usersDNS,
		"POSTGRES_USERS_PORT": "5432",
	})

	// Redundant qualified form for the matching entry MUST NOT exist.
	assertConfigMapAbsent(t, r, []string{
		"POSTGRES_POSTGRES_HOST",
		"POSTGRES_POSTGRES_PORT",
		"POSTGRES_POSTGRES_DB",
		"POSTGRES_URL",
		"POSTGRES_USERS_URL",
		"POSTGRES_POSTGRES_URL",
	})
}

func TestE2E_ProviderEnv_TwoNeo4jKnowledge(t *testing.T) {
	// Two knowledge stores using the same provider (neo4j). Bare NEO4J_*
	// points to the first entry alphabetically ("friends"), name-qualified vars for all.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  friends:
    provider: neo4j
  products:
    provider: neo4j
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both get StatefulSets (persistent)
	if !r.hasResource("StatefulSet", "my-agent-knowledge-friends") {
		t.Error("expected StatefulSet for friends neo4j")
	}
	if !r.hasResource("StatefulSet", "my-agent-knowledge-products") {
		t.Error("expected StatefulSet for products neo4j")
	}
	if !r.hasResource("Service", "my-agent-knowledge-friends") {
		t.Error("expected Service for friends neo4j")
	}
	if !r.hasResource("Service", "my-agent-knowledge-products") {
		t.Error("expected Service for products neo4j")
	}

	ns := r.Namespace
	friendsDNS := serviceDNS("my-agent-knowledge-friends", ns)
	productsDNS := serviceDNS("my-agent-knowledge-products", ns)

	// Bare prefix → first alphabetically ("friends"); name-qualified for all
	assertConfigMapValues(t, r, map[string]string{
		"NEO4J_HOST":          friendsDNS,
		"NEO4J_PORT":          "7474",
		"NEO4J_URL":           "bolt://" + friendsDNS + ":7474",
		"NEO4J_FRIENDS_HOST":  friendsDNS,
		"NEO4J_FRIENDS_PORT":  "7474",
		"NEO4J_FRIENDS_URL":   "bolt://" + friendsDNS + ":7474",
		"NEO4J_PRODUCTS_HOST": productsDNS,
		"NEO4J_PRODUCTS_PORT": "7474",
		"NEO4J_PRODUCTS_URL":  "bolt://" + productsDNS + ":7474",
	})
}

func TestE2E_ProviderEnv_ProviderAndContainerSameCategory(t *testing.T) {
	// Mix of provider-mode and container-mode in the same category.
	// Provider-mode gets provider-prefixed env (QDRANT_*), container-mode gets
	// generic KNOWLEDGE_{NAME}_* — no collision.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  vectors:
    provider: qdrant
  embeddings:
    container:
      image: my-embeddings:latest
      port: 9200
`, e2eOpts{})

	requireNoErrors(t, r)

	// Provider-mode qdrant
	if !r.hasResource("StatefulSet", "my-agent-knowledge-vectors") {
		t.Error("expected StatefulSet for qdrant")
	}
	// Container-mode
	if !r.hasResource("Deployment", "my-agent-knowledge-embeddings") {
		t.Error("expected Deployment for container knowledge")
	}

	ns := r.Namespace
	qdrantDNS := serviceDNS("my-agent-knowledge-vectors", ns)
	embeddingsDNS := serviceDNS("my-agent-knowledge-embeddings", ns)

	// Provider-mode gets QDRANT_* env vars
	assertConfigMapValues(t, r, map[string]string{
		"QDRANT_HOST": qdrantDNS,
		"QDRANT_PORT": "6333",
		"QDRANT_URL":  "http://" + qdrantDNS + ":6333",
	})

	// Container-mode gets KNOWLEDGE_{NAME}_* env vars
	assertConfigMapValues(t, r, map[string]string{
		"KNOWLEDGE_EMBEDDINGS_HOST": embeddingsDNS,
		"KNOWLEDGE_EMBEDDINGS_PORT": "9200",
	})

	// No cross-contamination
	assertConfigMapAbsent(t, r, []string{"KNOWLEDGE_VECTORS_HOST", "QDRANT_EMBEDDINGS_HOST"})
}

func TestE2E_ProviderEnv_TwoContainerIntegrations(t *testing.T) {
	// Two container-mode tools — each gets distinct INTEGRATION_{NAME}_* env vars, no collision.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
integrations:
  search:
    container:
      image: my-search:latest
      port: 3000
  rerank:
    container:
      image: my-rerank:latest
      port: 4000
`, e2eOpts{})

	requireNoErrors(t, r)

	if !r.hasResource("Deployment", "my-agent-integration-search") {
		t.Error("expected Deployment for search integration")
	}
	if !r.hasResource("Deployment", "my-agent-integration-rerank") {
		t.Error("expected Deployment for rerank tool")
	}

	ns := r.Namespace
	searchDNS := serviceDNS("my-agent-integration-search", ns)
	rerankDNS := serviceDNS("my-agent-integration-rerank", ns)

	assertConfigMapValues(t, r, map[string]string{
		"INTEGRATION_SEARCH_HOST": searchDNS,
		"INTEGRATION_SEARCH_PORT": "3000",
		"INTEGRATION_SEARCH_URL":  "http://" + searchDNS + ":3000",
		"INTEGRATION_RERANK_HOST": rerankDNS,
		"INTEGRATION_RERANK_PORT": "4000",
		"INTEGRATION_RERANK_URL":  "http://" + rerankDNS + ":4000",
	})
}

// --- utility ---

func variableKeys(m map[string]spec.Variable) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func keysOf(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestE2E_PostgresKnowledge_CredentialFlow verifies the full credential flow
// for postgres knowledge entries: auto-generated Secrets for knowledge containers,
// credential refs resolved into the deployment Secret for the agent, and no
// duplicate envFrom sources on the agent pod.
func TestE2E_PostgresKnowledge_CredentialFlow(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
knowledge:
  analytics:
    provider: postgres
  users:
    provider: postgres
  cache:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)
	ns := r.Namespace

	// --- Per-entry credential Secrets (for knowledge containers) ---

	// Analytics postgres entry's credential Secret
	analyticsCredSecret := r.getSecret(t, ns, knowledgeCredSecretName("my-agent", "analytics"))
	if string(analyticsCredSecret.Data["POSTGRES_USER"]) != "astro" {
		t.Errorf("analytics cred secret POSTGRES_USER: expected 'astro', got %q", string(analyticsCredSecret.Data["POSTGRES_USER"]))
	}
	if len(analyticsCredSecret.Data["POSTGRES_PASSWORD"]) == 0 {
		t.Error("analytics cred secret POSTGRES_PASSWORD: expected non-empty")
	}

	// Users postgres entry's credential Secret
	usersCredSecret := r.getSecret(t, ns, knowledgeCredSecretName("my-agent", "users"))
	if string(usersCredSecret.Data["POSTGRES_USER"]) != "astro" {
		t.Errorf("users cred secret POSTGRES_USER: expected 'astro', got %q", string(usersCredSecret.Data["POSTGRES_USER"]))
	}
	if len(usersCredSecret.Data["POSTGRES_PASSWORD"]) == 0 {
		t.Error("users cred secret POSTGRES_PASSWORD: expected non-empty")
	}

	// Each entry should have its own password (not shared).
	if string(analyticsCredSecret.Data["POSTGRES_PASSWORD"]) == string(usersCredSecret.Data["POSTGRES_PASSWORD"]) {
		t.Error("analytics and users should have different auto-generated passwords")
	}

	// Redis cache entry's credential Secret
	cacheCredSecret := r.getSecret(t, ns, knowledgeCredSecretName("my-agent", "cache"))
	if len(cacheCredSecret.Data["REDIS_PASSWORD"]) == 0 {
		t.Error("cache cred secret REDIS_PASSWORD: expected non-empty")
	}

	// --- Deployment Secret ---
	//
	// Credential env vars (POSTGRES_USER / _PASSWORD / _DB and their
	// per-store renamed forms; REDIS_PASSWORD) no longer materialise
	// in the agent's deploy Secret — they flow exclusively via direct
	// secretKeyRef entries on the agent container, pointing at the
	// per-store cred Secrets asserted above. The merged effective env
	// check further down confirms the agent still sees them.

	// --- ConfigMap (HOST, PORT — static values; DB now in Secret) ---

	analyticsDNS := serviceDNS("my-agent-knowledge-analytics", ns)
	usersDNS := serviceDNS("my-agent-knowledge-users", ns)
	assertConfigMapValues(t, r, map[string]string{
		"POSTGRES_HOST":           analyticsDNS,
		"POSTGRES_PORT":           "5432",
		"POSTGRES_ANALYTICS_HOST": analyticsDNS,
		"POSTGRES_ANALYTICS_PORT": "5432",
		"POSTGRES_USERS_HOST":     usersDNS,
		"POSTGRES_USERS_PORT":     "5432",
	})

	// --- Agent StatefulSet: no duplicate envFrom for knowledge secrets ---

	agentDepl, err := r.Clientset.AppsV1().StatefulSets(ns).Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get agent statefulset: %v", err)
	}
	agentContainer := agentDepl.Spec.Template.Spec.Containers[0]
	for _, envFrom := range agentContainer.EnvFrom {
		if envFrom.SecretRef != nil {
			name := envFrom.SecretRef.Name
			if strings.Contains(name, "knowledge") && strings.Contains(name, "creds") {
				t.Errorf("agent should not mount knowledge credential secret %q — credentials flow through deployment secret", name)
			}
		}
	}

	// --- Knowledge StatefulSets: each mounts its own credential Secret ---

	analyticsSS, err := r.Clientset.AppsV1().StatefulSets(ns).Get(
		context.Background(), "my-agent-knowledge-analytics", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get analytics statefulset: %v", err)
	}
	analyticsEnvFrom := analyticsSS.Spec.Template.Spec.Containers[0].EnvFrom
	foundAnalyticsCreds := false
	for _, ef := range analyticsEnvFrom {
		if ef.SecretRef != nil && ef.SecretRef.Name == knowledgeCredSecretName("my-agent", "analytics") {
			foundAnalyticsCreds = true
		}
	}
	if !foundAnalyticsCreds {
		t.Error("analytics StatefulSet should mount its own credential secret")
	}

	usersSS, err := r.Clientset.AppsV1().StatefulSets(ns).Get(
		context.Background(), "my-agent-knowledge-users", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get users statefulset: %v", err)
	}
	usersEnvFrom := usersSS.Spec.Template.Spec.Containers[0].EnvFrom
	foundUsersCreds := false
	for _, ef := range usersEnvFrom {
		if ef.SecretRef != nil && ef.SecretRef.Name == knowledgeCredSecretName("my-agent", "users") {
			foundUsersCreds = true
		}
	}
	if !foundUsersCreds {
		t.Error("users StatefulSet should mount its own credential secret")
	}

	// --- Agent's effective env (merged across envFrom + container.env) ---
	//
	// Verifies that every expected key reaches the agent regardless of which
	// path put it there:
	//   - HOST / PORT come from main ConfigMap (envFrom).
	//   - DB, and the BindCredentials-resolved USER / PASSWORD literals come
	//     from main Secret (envFrom).
	//   - USER / PASSWORD also exist as container.env secretKeyRef entries
	//     pointing at the per-store cred Secrets; container.env overrides
	//     envFrom on collisions (k8s precedence).
	// All paths must agree on values; mismatches would silently corrupt
	// the agent's view without this check.
	cacheDNS := serviceDNS("my-agent-knowledge-cache", ns)
	analyticsPwd := string(analyticsCredSecret.Data["POSTGRES_PASSWORD"])
	usersPwd := string(usersCredSecret.Data["POSTGRES_PASSWORD"])
	cachePwd := string(cacheCredSecret.Data["REDIS_PASSWORD"])

	wantEnv := map[string]string{
		// Postgres bare keys → primary entry "analytics" (alphabetically first;
		// no name matches provider).
		"POSTGRES_HOST":     analyticsDNS,
		"POSTGRES_PORT":     "5432",
		"POSTGRES_USER":     "astro",
		"POSTGRES_PASSWORD": analyticsPwd,
		"POSTGRES_DB":       "my_agent",
		// Postgres qualified for "analytics" (primary also gets qualified per RFC §8.2
		// when entry name != provider).
		"POSTGRES_ANALYTICS_HOST":     analyticsDNS,
		"POSTGRES_ANALYTICS_PORT":     "5432",
		"POSTGRES_ANALYTICS_USER":     "astro",
		"POSTGRES_ANALYTICS_PASSWORD": analyticsPwd,
		"POSTGRES_ANALYTICS_DB":       "my_agent",
		// Postgres qualified for "users" (non-primary, no bare).
		"POSTGRES_USERS_HOST":     usersDNS,
		"POSTGRES_USERS_PORT":     "5432",
		"POSTGRES_USERS_USER":     "astro",
		"POSTGRES_USERS_PASSWORD": usersPwd,
		"POSTGRES_USERS_DB":       "my_agent",
		// Redis: only one entry "cache" → bare keys only (not duplicate).
		"REDIS_HOST":     cacheDNS,
		"REDIS_PORT":     "6379",
		"REDIS_URL":      "redis://" + cacheDNS + ":6379",
		"REDIS_PASSWORD": cachePwd,
	}
	gotEnv := effectiveAgentEnv(t, r)
	for k, want := range wantEnv {
		got, ok := gotEnv[k]
		if !ok {
			t.Errorf("agent effective env missing %q (want %q)", k, want)
			continue
		}
		if got != want {
			t.Errorf("agent effective env[%q] = %q, want %q", k, got, want)
		}
	}
	// Doubled-prefix forms must NOT appear (RFC §8.2 — cache happens to not
	// match its provider, but check the postgres pair to lock the rule).
	for _, bad := range []string{"POSTGRES_POSTGRES_HOST", "POSTGRES_POSTGRES_USER", "POSTGRES_POSTGRES_DB"} {
		if _, present := gotEnv[bad]; present {
			t.Errorf("agent effective env must not contain %q (no entry name matches provider here, but the doubled form is wrong regardless)", bad)
		}
	}
}
