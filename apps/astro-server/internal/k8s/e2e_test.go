package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
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

	// Applier overrides
	GalileoAPIKey  string
	GalileoProject string
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
		registryURL:     opts.RegistryURL,
		imageResolver:   NewImageResolver("", opts.RegistryURL, "test"),
		imagePullPolicy: corev1.PullNever,
		galileoAPIKey:   opts.GalileoAPIKey,
		galileoProject:  opts.GalileoProject,
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

	// Should have agent Deployment + Service
	if !r.hasResource("Deployment", "my-agent-agent") {
		t.Error("expected agent Deployment")
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

func TestE2E_SelfHostedModel_Ollama(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    provider: ollama
    model: llama3.2
`, e2eOpts{})

	requireNoErrors(t, r)

	// Ollama with model name → persistent → StatefulSet
	if !r.hasResource("StatefulSet", "my-agent-model-llm") {
		t.Error("expected model StatefulSet for ollama with model pull")
	}
	if !r.hasResource("Service", "my-agent-model-llm") {
		t.Error("expected model Service")
	}

	// Agent env should have OLLAMA_HOST, OLLAMA_PORT wired
	env := r.DeploymentSpec.Agent.Environment
	for _, key := range []string{"OLLAMA_HOST", "OLLAMA_PORT", "OLLAMA_URL", "OLLAMA_BASE_URL", "OLLAMA_MODEL"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected agent env key %s", key)
		}
	}

	// ConfigMap should have resolved OLLAMA_HOST to a service DNS
	ns := r.Namespace
	cmName := deployment.GenerateConfigMapName("my-agent", "build-001")
	cm := r.getConfigMap(t, ns, cmName)
	ollamaHost := cm.Data["OLLAMA_HOST"]
	if !strings.Contains(ollamaHost, "my-agent-model-llm") {
		t.Errorf("expected OLLAMA_HOST to contain service DNS, got %q", ollamaHost)
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
    persistent: true
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

func TestE2E_KnowledgeStore_Redis_NonPersistent(t *testing.T) {
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

	// Non-persistent redis → Deployment (not StatefulSet)
	if !r.hasResource("Deployment", "my-agent-knowledge-cache") {
		t.Error("expected knowledge Deployment for non-persistent redis")
	}
	if r.hasResource("StatefulSet", "my-agent-knowledge-cache") {
		t.Error("did not expect StatefulSet for non-persistent knowledge")
	}
	if !r.hasResource("Service", "my-agent-knowledge-cache") {
		t.Error("expected knowledge Service")
	}

	// Agent env should have REDIS_HOST, REDIS_PORT, REDIS_URL
	env := r.DeploymentSpec.Agent.Environment
	for _, key := range []string{"REDIS_HOST", "REDIS_PORT", "REDIS_URL"} {
		if _, ok := env[key]; !ok {
			t.Errorf("expected agent env key %s", key)
		}
	}
}

func TestE2E_CloudTool_GitHub(t *testing.T) {
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

	// Cloud tool → no tool Deployment
	if r.hasResource("Deployment", "my-agent-tool-github") {
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
    provider: ollama
    model: llama3.2
  cloud:
    provider: anthropic
knowledge:
  docs:
    provider: qdrant
    persistent: true
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
		GalileoAPIKey:  "gal-key",
		GalileoProject: "test-project",
	})

	requireNoErrors(t, r)

	// Agent
	if !r.hasResource("Deployment", "my-agent-agent") {
		t.Error("expected agent Deployment")
	}
	if !r.hasResource("Service", "my-agent-agent") {
		t.Error("expected agent Service")
	}

	// Self-hosted model (ollama + model name → StatefulSet)
	if !r.hasResource("StatefulSet", "my-agent-model-llm") {
		t.Error("expected ollama model StatefulSet")
	}
	if !r.hasResource("Service", "my-agent-model-llm") {
		t.Error("expected ollama model Service")
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

	// Knowledge: non-persistent redis → Deployment
	if !r.hasResource("Deployment", "my-agent-knowledge-cache") {
		t.Error("expected redis knowledge Deployment")
	}
	if !r.hasResource("Service", "my-agent-knowledge-cache") {
		t.Error("expected redis knowledge Service")
	}

	// Cloud tool (github) → no container, but credential in Secret
	if r.hasResource("Deployment", "my-agent-tool-github") {
		t.Error("did not expect Deployment for cloud tool")
	}

	// Secret should exist with credentials
	if !r.hasResource("Secret", deployment.GenerateSecretName("my-agent", "build-001")) {
		t.Error("expected credentials Secret")
	}

	// ConfigMap should exist with resolved env
	if !r.hasResource("ConfigMap", deployment.GenerateConfigMapName("my-agent", "build-001")) {
		t.Error("expected config ConfigMap")
	}

	// Observability → collector Deployment + Service
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

	// OLLAMA env should be wired
	if host, ok := cm.Data["OLLAMA_HOST"]; !ok || !strings.Contains(host, "my-agent-model-llm") {
		t.Errorf("expected OLLAMA_HOST wired to model service DNS, got %q", cm.Data["OLLAMA_HOST"])
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
	if got := r.resourceCount("StatefulSet"); got != 2 {
		t.Errorf("expected 2 StatefulSets (ollama + qdrant), got %d", got)
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

// serviceDNS is a shorthand to build the expected k8s service DNS.
func serviceDNS(name, ns string) string {
	return name + "." + ns + ".svc.cluster.local"
}

func TestE2E_ProviderEnv_OllamaModel(t *testing.T) {
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    provider: ollama
    model: llama3.2
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-model-llm", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"OLLAMA_HOST":     host,
		"OLLAMA_PORT":     "11434",
		"OLLAMA_URL":      "http://" + host + ":11434",
		"OLLAMA_BASE_URL": "http://" + host + ":11434/api",
		"OLLAMA_MODEL":    "llama3.2",
	})
}

func TestE2E_ProviderEnv_OllamaNoModel(t *testing.T) {
	// Ollama without a model name — still gets HOST/PORT/URL but no MODEL
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    provider: ollama
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-model-llm", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"OLLAMA_HOST":     host,
		"OLLAMA_PORT":     "11434",
		"OLLAMA_URL":      "http://" + host + ":11434",
		"OLLAMA_BASE_URL": "http://" + host + ":11434/api",
	})

	// OLLAMA_MODEL should NOT be in the ConfigMap when no model is specified
	assertConfigMapAbsent(t, r, []string{"OLLAMA_MODEL"})
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

	// Should NOT have provider-prefixed env vars
	assertConfigMapAbsent(t, r, []string{"OLLAMA_HOST", "OLLAMA_PORT"})
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

	// Cloud provider → credential in Secret, reference resolved in ConfigMap
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	if string(secret.Data["OPENAI_API_KEY"]) != "sk-openai-test" {
		t.Errorf("expected OPENAI_API_KEY=sk-openai-test in secret")
	}

	// Resolved credential value should be in ConfigMap
	cmName := deployment.GenerateConfigMapName("my-agent", "build-001")
	cm := r.getConfigMap(t, ns, cmName)
	if cm.Data["OPENAI_API_KEY"] != "sk-openai-test" {
		t.Errorf("expected OPENAI_API_KEY resolved to credential value in ConfigMap, got %q", cm.Data["OPENAI_API_KEY"])
	}

	// No container env vars for cloud provider
	assertConfigMapAbsent(t, r, []string{"OPENAI_HOST", "OPENAI_PORT", "MODEL_GPT_HOST"})
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
    persistent: true
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
    persistent: true
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
    persistent: true
`, e2eOpts{})

	requireNoErrors(t, r)

	host := serviceDNS("my-agent-knowledge-graph", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"NEO4J_HOST": host,
		"NEO4J_PORT": "7474",
		"NEO4J_URL":  "bolt://" + host + ":7474", // URLScheme=bolt
	})
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

func TestE2E_ProviderEnv_ContainerTool(t *testing.T) {
	// Container-mode tool → INTEGRATION_{NAME}_ prefix (§8.3)
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

	host := serviceDNS("my-agent-tool-search", "test-ns")
	assertConfigMapValues(t, r, map[string]string{
		"INTEGRATION_SEARCH_HOST": host,
		"INTEGRATION_SEARCH_PORT": "3000",
		"INTEGRATION_SEARCH_URL":  "http://" + host + ":3000",
	})
}

func TestE2E_ProviderEnv_CloudToolGitLab(t *testing.T) {
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

	// Cloud tool → no container
	if r.hasResource("Deployment", "my-agent-tool-gitlab") {
		t.Error("did not expect Deployment for cloud tool provider")
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
	// Every deployment gets ASTRO_AGENT_NAME, ASTRO_AGENT_BUILD, AGENT_URL, OTEL endpoint
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
		"AGENT_URL":                   "http://" + agentHost + ":8080",
		"AGENT_HOST":                  agentHost,
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://" + collectorHost + ":4318",
	})
}

func TestE2E_ProviderEnv_MixedModelAndKnowledge(t *testing.T) {
	// Ollama model + qdrant knowledge + redis cache — all env vars coexist
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  llm:
    provider: ollama
    model: llama3.2
knowledge:
  docs:
    provider: qdrant
    persistent: true
  cache:
    provider: redis
`, e2eOpts{})

	requireNoErrors(t, r)

	ollamaHost := serviceDNS("my-agent-model-llm", "test-ns")
	qdrantHost := serviceDNS("my-agent-knowledge-docs", "test-ns")
	redisHost := serviceDNS("my-agent-knowledge-cache", "test-ns")

	assertConfigMapValues(t, r, map[string]string{
		// Ollama
		"OLLAMA_HOST":     ollamaHost,
		"OLLAMA_PORT":     "11434",
		"OLLAMA_URL":      "http://" + ollamaHost + ":11434",
		"OLLAMA_BASE_URL": "http://" + ollamaHost + ":11434/api",
		"OLLAMA_MODEL":    "llama3.2",
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

	ns := r.Namespace

	// All credential values should be in the Secret
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)
	wantSecret := map[string]string{
		"ANTHROPIC_API_KEY":          "sk-ant-real",
		"GITHUB_TOKEN":               "ghp_real",
		"SLACK_PROVIDER_WEBHOOK_URL": "https://hooks.slack.com/test",
	}
	for key, val := range wantSecret {
		if string(secret.Data[key]) != val {
			t.Errorf("Secret[%s] = %q, want %q", key, string(secret.Data[key]), val)
		}
	}

	// Resolved values should also appear in ConfigMap (credential refs resolve to actual values)
	assertConfigMapValues(t, r, map[string]string{
		"ANTHROPIC_API_KEY":          "sk-ant-real",
		"GITHUB_TOKEN":               "ghp_real",
		"SLACK_PROVIDER_WEBHOOK_URL": "https://hooks.slack.com/test",
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
    persistent: true
  secondary:
    provider: qdrant
    persistent: true
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

func TestE2E_ProviderEnv_TwoOllamaModels(t *testing.T) {
	// Two models using the same provider (ollama). Both get containers.
	// Bare OLLAMA_* points to first alphabetically ("big"), name-qualified vars for all.
	r := runE2E(t, `
spec: package/v1
name: my-agent
agent:
  image: my-agent:latest
models:
  fast:
    provider: ollama
    model: llama3.2
  big:
    provider: ollama
    model: deepseek-r1
`, e2eOpts{})

	requireNoErrors(t, r)

	// Both get their own StatefulSet + Service (model with name → persistent)
	if !r.hasResource("StatefulSet", "my-agent-model-fast") {
		t.Error("expected StatefulSet for fast model")
	}
	if !r.hasResource("StatefulSet", "my-agent-model-big") {
		t.Error("expected StatefulSet for big model")
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

	// Bare prefix points to first alphabetically ("big")
	// Name-qualified vars exist for all entries
	assertConfigMapValues(t, r, map[string]string{
		"OLLAMA_HOST":          bigDNS,
		"OLLAMA_PORT":          "11434",
		"OLLAMA_URL":           "http://" + bigDNS + ":11434",
		"OLLAMA_BASE_URL":      "http://" + bigDNS + ":11434/api",
		"OLLAMA_MODEL":         "deepseek-r1",
		"OLLAMA_BIG_HOST":      bigDNS,
		"OLLAMA_BIG_PORT":      "11434",
		"OLLAMA_BIG_URL":       "http://" + bigDNS + ":11434",
		"OLLAMA_BIG_BASE_URL":  "http://" + bigDNS + ":11434/api",
		"OLLAMA_BIG_MODEL":     "deepseek-r1",
		"OLLAMA_FAST_HOST":     fastDNS,
		"OLLAMA_FAST_PORT":     "11434",
		"OLLAMA_FAST_URL":      "http://" + fastDNS + ":11434",
		"OLLAMA_FAST_BASE_URL": "http://" + fastDNS + ":11434/api",
		"OLLAMA_FAST_MODEL":    "llama3.2",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsSameProvider(t *testing.T) {
	// Two models using the same cloud provider (anthropic). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"ANTHROPIC_API_KEY":              "sk-bare",
			"ANTHROPIC_CLAUDE-HAIKU_API_KEY": "sk-haiku",
			"ANTHROPIC_CLAUDE-OPUS_API_KEY":  "sk-opus",
		},
	})

	requireNoErrors(t, r)

	// No containers for cloud providers
	if r.hasResource("Deployment", "my-agent-model-claude-haiku") || r.hasResource("Deployment", "my-agent-model-claude-opus") {
		t.Error("did not expect Deployments for cloud providers")
	}

	// Provider-prefixed keys in the Secret
	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["ANTHROPIC_API_KEY"]) != "sk-bare" {
		t.Errorf("expected ANTHROPIC_API_KEY=sk-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["ANTHROPIC_CLAUDE-HAIKU_API_KEY"]) != "sk-haiku" {
		t.Errorf("expected ANTHROPIC_CLAUDE-HAIKU_API_KEY=sk-haiku in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["ANTHROPIC_CLAUDE-OPUS_API_KEY"]) != "sk-opus" {
		t.Errorf("expected ANTHROPIC_CLAUDE-OPUS_API_KEY=sk-opus in secret, got keys: %v", keysOf(secret.Data))
	}

	// All resolved in ConfigMap
	assertConfigMapValues(t, r, map[string]string{
		"ANTHROPIC_API_KEY":              "sk-bare",
		"ANTHROPIC_CLAUDE-HAIKU_API_KEY": "sk-haiku",
		"ANTHROPIC_CLAUDE-OPUS_API_KEY":  "sk-opus",
	})
}

func TestE2E_ProviderEnv_TwoCloudToolsSameProvider(t *testing.T) {
	// Two tools using the same cloud provider (github). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"GITHUB_TOKEN":              "ghp_bare",
			"GITHUB_GH-MAIN_TOKEN":      "ghp_main",
			"GITHUB_GH-SECONDARY_TOKEN": "ghp_secondary",
		},
	})

	requireNoErrors(t, r)

	// No containers
	if r.hasResource("Deployment", "my-agent-tool-gh-main") || r.hasResource("Deployment", "my-agent-tool-gh-secondary") {
		t.Error("did not expect Deployments for cloud tools")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GITHUB_TOKEN"]) != "ghp_bare" {
		t.Errorf("expected GITHUB_TOKEN=ghp_bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITHUB_GH-MAIN_TOKEN"]) != "ghp_main" {
		t.Errorf("expected GITHUB_GH-MAIN_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITHUB_GH-SECONDARY_TOKEN"]) != "ghp_secondary" {
		t.Errorf("expected GITHUB_GH-SECONDARY_TOKEN in secret, got keys: %v", keysOf(secret.Data))
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

	// Should NOT have the redundant ANTHROPIC_ANTHROPIC_API_KEY
	assertConfigMapAbsent(t, r, []string{"ANTHROPIC_ANTHROPIC_API_KEY"})

	assertConfigMapValues(t, r, map[string]string{
		"ANTHROPIC_API_KEY":          "sk-primary",
		"ANTHROPIC_FALLBACK_API_KEY": "sk-fallback",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsOpenAI(t *testing.T) {
	// Two models using the same cloud provider (openai). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"OPENAI_API_KEY":           "sk-bare",
			"OPENAI_GPT-FAST_API_KEY":  "sk-fast",
			"OPENAI_GPT-SMART_API_KEY": "sk-smart",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-gpt-fast") || r.hasResource("Deployment", "my-agent-model-gpt-smart") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["OPENAI_API_KEY"]) != "sk-bare" {
		t.Errorf("expected OPENAI_API_KEY=sk-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["OPENAI_GPT-FAST_API_KEY"]) != "sk-fast" {
		t.Errorf("expected OPENAI_GPT-FAST_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["OPENAI_GPT-SMART_API_KEY"]) != "sk-smart" {
		t.Errorf("expected OPENAI_GPT-SMART_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}

	assertConfigMapValues(t, r, map[string]string{
		"OPENAI_API_KEY":           "sk-bare",
		"OPENAI_GPT-FAST_API_KEY":  "sk-fast",
		"OPENAI_GPT-SMART_API_KEY": "sk-smart",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsGoogle(t *testing.T) {
	// Two models using the same cloud provider (google). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"GOOGLE_API_KEY":              "goog-bare",
			"GOOGLE_GEMINI-FLASH_API_KEY": "goog-flash",
			"GOOGLE_GEMINI-PRO_API_KEY":   "goog-pro",
		},
	})

	requireNoErrors(t, r)

	if r.hasResource("Deployment", "my-agent-model-gemini-flash") || r.hasResource("Deployment", "my-agent-model-gemini-pro") {
		t.Error("did not expect Deployments for cloud providers")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GOOGLE_API_KEY"]) != "goog-bare" {
		t.Errorf("expected GOOGLE_API_KEY=goog-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GOOGLE_GEMINI-FLASH_API_KEY"]) != "goog-flash" {
		t.Errorf("expected GOOGLE_GEMINI-FLASH_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GOOGLE_GEMINI-PRO_API_KEY"]) != "goog-pro" {
		t.Errorf("expected GOOGLE_GEMINI-PRO_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}

	assertConfigMapValues(t, r, map[string]string{
		"GOOGLE_API_KEY":              "goog-bare",
		"GOOGLE_GEMINI-FLASH_API_KEY": "goog-flash",
		"GOOGLE_GEMINI-PRO_API_KEY":   "goog-pro",
	})
}

func TestE2E_ProviderEnv_TwoCloudModelsCohere(t *testing.T) {
	// Two models using the same cloud provider (cohere). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"COHERE_API_KEY":        "co-bare",
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

	if string(secret.Data["COHERE_API_KEY"]) != "co-bare" {
		t.Errorf("expected COHERE_API_KEY=co-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["COHERE_EMBED_API_KEY"]) != "co-embed" {
		t.Errorf("expected COHERE_EMBED_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["COHERE_RERANK_API_KEY"]) != "co-rerank" {
		t.Errorf("expected COHERE_RERANK_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}

	assertConfigMapValues(t, r, map[string]string{
		"COHERE_API_KEY":        "co-bare",
		"COHERE_EMBED_API_KEY":  "co-embed",
		"COHERE_RERANK_API_KEY": "co-rerank",
	})
}

func TestE2E_ProviderEnv_TwoCloudKnowledgePinecone(t *testing.T) {
	// Two knowledge stores using the same cloud provider (pinecone). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"PINECONE_API_KEY":            "pc-bare",
			"PINECONE_EMBEDDINGS_API_KEY": "pc-embed",
			"PINECONE_SEARCH_API_KEY":     "pc-search",
		},
	})

	requireNoErrors(t, r)

	// Cloud knowledge → no containers
	if r.hasResource("Deployment", "my-agent-knowledge-embeddings") || r.hasResource("StatefulSet", "my-agent-knowledge-embeddings") {
		t.Error("did not expect container resources for cloud knowledge")
	}
	if r.hasResource("Deployment", "my-agent-knowledge-search") || r.hasResource("StatefulSet", "my-agent-knowledge-search") {
		t.Error("did not expect container resources for cloud knowledge")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["PINECONE_API_KEY"]) != "pc-bare" {
		t.Errorf("expected PINECONE_API_KEY=pc-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["PINECONE_EMBEDDINGS_API_KEY"]) != "pc-embed" {
		t.Errorf("expected PINECONE_EMBEDDINGS_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["PINECONE_SEARCH_API_KEY"]) != "pc-search" {
		t.Errorf("expected PINECONE_SEARCH_API_KEY in secret, got keys: %v", keysOf(secret.Data))
	}

	assertConfigMapValues(t, r, map[string]string{
		"PINECONE_API_KEY":            "pc-bare",
		"PINECONE_EMBEDDINGS_API_KEY": "pc-embed",
		"PINECONE_SEARCH_API_KEY":     "pc-search",
	})

	// No container env vars
	assertConfigMapAbsent(t, r, []string{"PINECONE_HOST", "PINECONE_PORT"})
}

func TestE2E_ProviderEnv_TwoCloudToolsGitLab(t *testing.T) {
	// Two tools using the same cloud provider (gitlab). Gets bare provider key
	// for first alphabetically + name-qualified keys for all.
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
			"GITLAB_TOKEN":           "glpat-bare",
			"GITLAB_GL-DEPLOY_TOKEN": "glpat-deploy",
			"GITLAB_GL-MAIN_TOKEN":   "glpat-main",
		},
	})

	requireNoErrors(t, r)

	// No containers
	if r.hasResource("Deployment", "my-agent-tool-gl-main") || r.hasResource("Deployment", "my-agent-tool-gl-deploy") {
		t.Error("did not expect Deployments for cloud tools")
	}

	ns := r.Namespace
	secretName := deployment.GenerateSecretName("my-agent", "build-001")
	secret := r.getSecret(t, ns, secretName)

	if string(secret.Data["GITLAB_TOKEN"]) != "glpat-bare" {
		t.Errorf("expected GITLAB_TOKEN=glpat-bare in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITLAB_GL-DEPLOY_TOKEN"]) != "glpat-deploy" {
		t.Errorf("expected GITLAB_GL-DEPLOY_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}
	if string(secret.Data["GITLAB_GL-MAIN_TOKEN"]) != "glpat-main" {
		t.Errorf("expected GITLAB_GL-MAIN_TOKEN in secret, got keys: %v", keysOf(secret.Data))
	}

	assertConfigMapValues(t, r, map[string]string{
		"GITLAB_TOKEN":           "glpat-bare",
		"GITLAB_GL-DEPLOY_TOKEN": "glpat-deploy",
		"GITLAB_GL-MAIN_TOKEN":   "glpat-main",
	})
}

func TestE2E_ProviderEnv_SameProviderMixedPersistence(t *testing.T) {
	// Two redis knowledge stores: one persistent, one not.
	// Bare REDIS_* points to first alphabetically ("durable"), name-qualified vars for all.
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
    persistent: true
`, e2eOpts{})

	requireNoErrors(t, r)

	// Non-persistent → Deployment, persistent → StatefulSet
	if !r.hasResource("Deployment", "my-agent-knowledge-sessions") {
		t.Error("expected Deployment for non-persistent redis")
	}
	if r.hasResource("StatefulSet", "my-agent-knowledge-sessions") {
		t.Error("did not expect StatefulSet for non-persistent redis")
	}
	if !r.hasResource("StatefulSet", "my-agent-knowledge-durable") {
		t.Error("expected StatefulSet for persistent redis")
	}
	if r.hasResource("Deployment", "my-agent-knowledge-durable") {
		t.Error("did not expect Deployment for persistent redis")
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
    persistent: true
  users:
    provider: postgres
    persistent: true
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

	// Bare prefix → first alphabetically ("analytics"); name-qualified for all
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
    persistent: true
  products:
    provider: neo4j
    persistent: true
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
    persistent: true
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

func TestE2E_ProviderEnv_TwoContainerTools(t *testing.T) {
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

	if !r.hasResource("Deployment", "my-agent-tool-search") {
		t.Error("expected Deployment for search tool")
	}
	if !r.hasResource("Deployment", "my-agent-tool-rerank") {
		t.Error("expected Deployment for rerank tool")
	}

	ns := r.Namespace
	searchDNS := serviceDNS("my-agent-tool-search", ns)
	rerankDNS := serviceDNS("my-agent-tool-rerank", ns)

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
