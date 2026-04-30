package k8s

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploytoken"
	spec "github.com/astropods/astro/packages/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// newTestApplier creates an Applier with a fake k8s clientset for testing.
func newTestApplier() *Applier {
	fakeClient := fake.NewClientset()
	return &Applier{
		clientset:       fakeClient,
		namespace:       "default",
		registryURL:     "test-registry.example.com",
		imageResolver:   NewImageResolver("", "test-registry.example.com", "test"),
		imagePullPolicy: corev1.PullNever,
	}
}

func httpEp(port int) map[string]spec.Endpoint {
	return map[string]spec.Endpoint{"http": {Port: port}}
}

func minimalDeploymentSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "my-agent", Build: "build-123", Account: "acme"},
		Target: spec.DeploymentTarget{Runtime: "kubernetes"},
		Agent: spec.DeploymentAgent{
			Image:     "test-registry.example.com/my-agent:latest",
			Endpoints: httpEp(8080),
			Replicas:  1,
			Update:    spec.DefaultUpdateStrategy(),
		},
	}
}

func TestApplyDeploymentSpec_MinimalAgent(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ctx := context.Background()

	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created: namespace + agent service + agent deployment = at least 2 resources
	if len(result.Resources) < 2 {
		t.Errorf("expected at least 2 resources, got %d", len(result.Resources))
	}

	// No errors expected
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Should have an agent-http endpoint
	foundEndpoint := false
	for _, ep := range result.ServiceEndpoints {
		if ep.Name == "agent-http" {
			foundEndpoint = true
			if ep.Port != 8080 {
				t.Errorf("endpoint port: expected 8080, got %d", ep.Port)
			}
		}
	}
	if !foundEndpoint {
		t.Error("expected agent-http service endpoint")
	}

}

func TestApplyDeploymentSpec_WithModel(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Models = map[string]spec.DeploymentModel{
		"llm": {
			Image:     "test-registry.example.com/ollama:latest",
			Endpoints: httpEp(11434),
			Replicas:  1,
			Update:    spec.DefaultUpdateStrategy(),
		},
	}
	ds.Agent.Environment = map[string]string{
		"LLM_URL": "${models.llm.http.url}",
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Should have: ConfigMap + model service + agent service + model deployment + agent deployment = 5+
	hasConfigMap := false
	hasModelService := false
	hasModelDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "ConfigMap" {
			hasConfigMap = true
		}
		if r.Kind == "Service" && r.Name == "my-agent-model-llm" {
			hasModelService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-model-llm" {
			hasModelDeployment = true
		}
	}
	if !hasConfigMap {
		t.Error("expected ConfigMap resource")
	}
	if !hasModelService {
		t.Error("expected model service resource")
	}
	if !hasModelDeployment {
		t.Error("expected model deployment resource")
	}
}

func TestApplyDeploymentSpec_WithKnowledgePersistent(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"docs": {
			Image:      "test-registry.example.com/qdrant:latest",
			Endpoints:  httpEp(6333),
			Replicas:   1,
			Persistent: true,
			Storage:    &spec.StorageConfig{Size: "20Gi", AccessMode: "ReadWriteOnce"},
			Update:     spec.DefaultUpdateStrategy(),
			Provider:   "qdrant",
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasStatefulSet := false
	hasKnowledgeService := false
	for _, r := range result.Resources {
		if r.Kind == "StatefulSet" && r.Name == "my-agent-knowledge-docs" {
			hasStatefulSet = true
		}
		if r.Kind == "Service" && r.Name == "my-agent-knowledge-docs" {
			hasKnowledgeService = true
		}
	}
	if !hasStatefulSet {
		t.Error("expected StatefulSet for persistent knowledge")
	}
	if !hasKnowledgeService {
		t.Error("expected knowledge service")
	}
}

func TestApplyDeploymentSpec_WithKnowledgeNonPersistent(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"cache": {
			Image:      "test-registry.example.com/redis:latest",
			Endpoints:  httpEp(6379),
			Replicas:   1,
			Persistent: false,
			Update:     spec.DefaultUpdateStrategy(),
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Non-persistent should produce a Deployment, not a StatefulSet
	hasDeployment := false
	hasStatefulSet := false
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "my-agent-knowledge-cache" {
			hasDeployment = true
		}
		if r.Kind == "StatefulSet" && r.Name == "my-agent-knowledge-cache" {
			hasStatefulSet = true
		}
	}
	if !hasDeployment {
		t.Error("expected Deployment for non-persistent knowledge")
	}
	if hasStatefulSet {
		t.Error("did not expect StatefulSet for non-persistent knowledge")
	}
}

func TestApplyDeploymentSpec_WithTool(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Integrations = map[string]spec.DeploymentIntegration{
		"search": {
			Image:     "test-registry.example.com/search:latest",
			Endpoints: httpEp(3000),
			Replicas:  1,
			Update:    spec.DefaultUpdateStrategy(),
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasToolService := false
	hasToolDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-integration-search" {
			hasToolService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-integration-search" {
			hasToolDeployment = true
		}
	}
	if !hasToolService {
		t.Error("expected tool service")
	}
	if !hasToolDeployment {
		t.Error("expected tool deployment")
	}
}

func TestApplyDeploymentSpec_WithSecretVariables(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Variables = map[string]spec.Variable{
		"API_KEY": {Value: "sk-secret-123", Secret: true},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasSecret := false
	for _, r := range result.Resources {
		if r.Kind == "Secret" {
			hasSecret = true
		}
	}
	if !hasSecret {
		t.Error("expected Secret resource for secret variables")
	}
}

func TestApplyDeploymentSpec_WithObservability(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{Enabled: true}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Collector is a standalone deployment with its own service
	hasCollectorService := false
	hasCollectorDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-collector" {
			hasCollectorService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-collector" {
			hasCollectorDeployment = true
		}
	}
	if !hasCollectorService {
		t.Error("expected collector service")
	}
	if !hasCollectorDeployment {
		t.Error("expected collector deployment")
	}
}

func TestApplyDeploymentSpec_ObservabilityCustomImage(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{
		Enabled:  true,
		Provider: "langfuse",
		Image:    "custom-registry.example.com/collector:v2",
		Port:     4318,
		Resources: spec.DeploymentResources{
			CPU: "200m", Memory: "256Mi",
			CPULimit: "1", MemoryLimit: "1Gi",
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Collector is a standalone deployment with its own service
	hasCollectorService := false
	hasCollectorDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-collector" {
			hasCollectorService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-collector" {
			hasCollectorDeployment = true
		}
	}
	if !hasCollectorService {
		t.Error("expected collector service")
	}
	if !hasCollectorDeployment {
		t.Error("expected collector deployment")
	}
}

func TestApplyDeploymentSpec_ObservabilityCustomPort(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{
		Enabled: true,
		Port:    5318,
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Collector should be a standalone deployment
	hasCollectorDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "my-agent-collector" {
			hasCollectorDeployment = true
		}
	}
	if !hasCollectorDeployment {
		t.Error("expected collector deployment")
	}
}

func TestApplyDeploymentSpec_ObservabilityDisabled(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{Enabled: false}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range result.Resources {
		if r.Name == "my-agent-collector" {
			t.Error("expected no collector resources when observability is disabled")
		}
	}
}

func TestApplyDeploymentSpec_WithIngestionSchedule(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"daily": {
			Image:   "test-registry.example.com/ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasCronJob := false
	for _, r := range result.Resources {
		if r.Kind == "CronJob" && r.Name == "my-agent-ingestion-daily" {
			hasCronJob = true
		}
	}
	if !hasCronJob {
		t.Error("expected CronJob for schedule ingestion")
	}
}

func TestApplyDeploymentSpec_WithIngestionStartup(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"init": {
			Image:   "test-registry.example.com/ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "startup"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasJob := false
	for _, r := range result.Resources {
		if r.Kind == "Job" && r.Name == "my-agent-ingestion-init" {
			hasJob = true
		}
	}
	if !hasJob {
		t.Error("expected Job for startup ingestion")
	}
}

func TestApplyDeploymentSpec_WithIngestionWebhook(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"hook": {
			Image:     "test-registry.example.com/ingest:latest",
			Endpoints: httpEp(9090),
			Trigger:   spec.DeploymentTrigger{Type: "webhook"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasService := false
	hasDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-ingestion-hook" {
			hasService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-ingestion-hook" {
			hasDeployment = true
		}
	}
	if !hasService {
		t.Error("expected Service for webhook ingestion")
	}
	if !hasDeployment {
		t.Error("expected Deployment for webhook ingestion")
	}
}

func TestApplyDeploymentSpec_FullStack(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Agent.Environment = map[string]string{
		"LLM_URL":  "${models.llm.http.url}",
		"DB_HOST":  "${knowledge.docs.host}",
		"TOOL_URL": "${integrations.search.http.url}",
	}
	ds.Models = map[string]spec.DeploymentModel{
		"llm": {
			Image: "test-registry.example.com/ollama:latest", Endpoints: httpEp(11434),
			Replicas: 2, Update: spec.DefaultUpdateStrategy(),
			GPU:       &spec.DeploymentGPU{VRAM: "24Gi", Runtime: "cuda", Count: 1},
			Resources: spec.GPUResources,
		},
	}
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"docs": {
			Image: "test-registry.example.com/qdrant:latest", Endpoints: httpEp(6333),
			Replicas: 1, Persistent: true, Update: spec.DefaultUpdateStrategy(),
			Storage:  &spec.StorageConfig{Size: "50Gi", Class: "gp3", AccessMode: "ReadWriteOnce"},
			Provider: "qdrant",
		},
	}
	ds.Integrations = map[string]spec.DeploymentIntegration{
		"search": {
			Image: "test-registry.example.com/search:latest", Endpoints: httpEp(3000),
			Replicas: 1, Update: spec.DefaultUpdateStrategy(),
		},
	}
	ds.Variables = map[string]spec.Variable{
		"ANTHROPIC_API_KEY": {Value: "sk-123", Secret: true},
	}
	ds.Observability = spec.DeploymentObservability{Enabled: true}
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"daily": {
			Image:   "test-registry.example.com/ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Count resource types
	counts := map[string]int{}
	for _, r := range result.Resources {
		counts[r.Kind]++
	}

	// Expected: 1 Secret + 1 ConfigMap + 5 Services (model, knowledge, tool, agent, collector)
	//           + 1 model Deployment + 1 knowledge StatefulSet + 1 tool Deployment
	//           + 1 agent Deployment + 1 collector Deployment + 1 CronJob
	if counts["Secret"] != 1 {
		t.Errorf("expected 1 Secret, got %d", counts["Secret"])
	}
	if counts["ConfigMap"] != 1 {
		t.Errorf("expected 1 ConfigMap, got %d", counts["ConfigMap"])
	}
	if counts["Service"] < 5 {
		t.Errorf("expected at least 5 Services (model, knowledge, tool, agent, collector), got %d", counts["Service"])
	}
	if counts["Deployment"] < 4 {
		t.Errorf("expected at least 4 Deployments (model, tool, agent, collector), got %d", counts["Deployment"])
	}
	if counts["StatefulSet"] != 1 {
		t.Errorf("expected 1 StatefulSet, got %d", counts["StatefulSet"])
	}
	if counts["CronJob"] != 1 {
		t.Errorf("expected 1 CronJob, got %d", counts["CronJob"])
	}
}

// TestApplyDeploymentSpec_WorkloadNamesMatchNormalized verifies that every
// Deployment/StatefulSet the applier creates has a name matching what
// SaveNormalizedSpec would insert as a workload row. This catches divergence
// between the K8s applier and the normalized DB representation (e.g., the
// collector being inserted as a sidecar instead of a workload).
func TestApplyDeploymentSpec_WorkloadNamesMatchNormalized(t *testing.T) {
	a := newTestApplier()
	agentName := "my-agent"
	ds := &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: agentName, Build: "build-1", Account: "acme"},
		Target: spec.DeploymentTarget{Runtime: "kubernetes"},
		Agent: spec.DeploymentAgent{
			Image: "test-registry.example.com/agent:latest", Endpoints: httpEp(8080),
			Replicas: 1, Update: spec.DefaultUpdateStrategy(),
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {
				Image: "test-registry.example.com/ollama:latest", Endpoints: httpEp(11434),
				Replicas: 1, Update: spec.DefaultUpdateStrategy(),
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"vectors": {
				Image: "test-registry.example.com/qdrant:latest", Endpoints: httpEp(6333),
				Replicas: 1, Persistent: true, Update: spec.DefaultUpdateStrategy(),
				Storage:  &spec.StorageConfig{Size: "10Gi", AccessMode: "ReadWriteOnce"},
				Provider: "qdrant",
			},
		},
		Integrations: map[string]spec.DeploymentIntegration{
			"search": {
				Image: "test-registry.example.com/search:latest", Endpoints: httpEp(3000),
				Replicas: 1, Update: spec.DefaultUpdateStrategy(),
			},
		},
		Observability: spec.DeploymentObservability{
			Enabled: true, Image: "collector:latest", Port: 4318,
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// Collect Deployment/StatefulSet names from the applier result.
	applierWorkloads := make(map[string]bool)
	for _, r := range result.Resources {
		if r.Kind == "Deployment" || r.Kind == "StatefulSet" {
			applierWorkloads[r.Name] = true
		}
	}

	// Build the expected workload names using the same helpers that
	// SaveNormalizedSpec uses. If these two sets diverge, the normalized
	// tables will be missing workloads (or have extra ones).
	expectedWorkloads := map[string]bool{
		deployment.GenerateAgentResourceName(agentName, "agent"):            true,
		deployment.GenerateResourceName(agentName, "model", "llm"):          true,
		deployment.GenerateResourceName(agentName, "knowledge", "vectors"):  true,
		deployment.GenerateResourceName(agentName, "integration", "search"): true,
		deployment.GenerateAgentResourceName(agentName, "collector"):        true,
	}

	// Every expected workload must exist in the applier output
	for name := range expectedWorkloads {
		if !applierWorkloads[name] {
			t.Errorf("expected workload %q not found in applier output (have: %v)", name, applierWorkloads)
		}
	}

	// Every applier workload must be in the expected set
	for name := range applierWorkloads {
		if !expectedWorkloads[name] {
			t.Errorf("applier created workload %q not in expected normalized set", name)
		}
	}

	// Cross-check orphan cleanup: computeExpectedResourceNames must list
	// every Deployment/StatefulSet the applier creates, otherwise the
	// cleanup will delete resources immediately after they're created.
	orphanExpected := computeExpectedResourceNames(ds, "", "")
	for name := range applierWorkloads {
		kind := "Deployment"
		if !orphanExpected[kind][name] {
			kind = "StatefulSet"
		}
		if !orphanExpected[kind][name] {
			t.Errorf("applier workload %q not in orphan cleanup expected set — cleanup would delete it", name)
		}
	}

	// Also check Services: every Service the applier creates must be expected
	applierServices := make(map[string]bool)
	for _, r := range result.Resources {
		if r.Kind == "Service" {
			applierServices[r.Name] = true
		}
	}
	for name := range applierServices {
		if !orphanExpected["Service"][name] {
			t.Errorf("applier service %q not in orphan cleanup expected set — cleanup would delete it", name)
		}
	}
}

func TestApplyDeploymentSpec_ResourceStatusNames(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range result.Resources {
		if r.Status != "created" {
			t.Errorf("resource %s/%s: expected status 'created', got %s", r.Kind, r.Name, r.Status)
		}
		if r.Namespace != "default" {
			t.Errorf("resource %s/%s: expected namespace default, got %s", r.Kind, r.Name, r.Namespace)
		}
	}
}

func TestApplyDeploymentSpec_WithSlackInterface(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
		Resources: spec.MessagingResources,
		Environment: map[string]string{
			"SLACK_BOT_TOKEN": "${variables.SLACK_BOT_TOKEN}",
			"SLACK_APP_TOKEN": "${variables.SLACK_APP_TOKEN}",
		},
	}
	ds.Variables = map[string]spec.Variable{
		"SLACK_BOT_TOKEN": {Value: "xoxb-test", Secret: true},
		"SLACK_APP_TOKEN": {Value: "xapp-test", Secret: true},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Messaging is colocated in the agent pod — service exists but no separate deployment
	hasMsgService := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-messaging" {
			hasMsgService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			t.Error("messaging should be a sidecar, not a separate deployment")
		}
	}
	if !hasMsgService {
		t.Error("expected messaging service")
	}
}

func TestApplyDeploymentSpec_InterfaceEnvInMessagingContainer(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()

	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
		Resources: spec.MessagingResources,
		Environment: map[string]string{
			"SLACK_CONFIG": "${variables.SLACK_CONFIG}",
		},
	}
	ds.Variables = map[string]spec.Variable{
		"SLACK_CONFIG": {Value: `{"actionable_reactions":["ticket","bug"],"allowed_channel_ids":["C123","C999"],"allowed_user_ids":["U123","U999"]}`, Secret: false, Targets: []string{"interface.slack"}},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	agentDepl, err := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("failed to get agent Deployment: %v", err)
	}

	var msgContainer *corev1.Container
	for i, c := range agentDepl.Spec.Template.Spec.InitContainers {
		if c.Name == "messaging" {
			msgContainer = &agentDepl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if msgContainer == nil {
		t.Fatal("messaging sidecar container not found in agent Deployment")
	}

	envMap := make(map[string]string)
	for _, e := range msgContainer.Env {
		envMap[e.Name] = e.Value
	}

	slackCfg, ok := envMap["SLACK_CONFIG"]
	if !ok {
		t.Fatal("SLACK_CONFIG not found in messaging container env")
	}
	wantCfg := `{"actionable_reactions":["ticket","bug"],"allowed_channel_ids":["C123","C999"],"allowed_user_ids":["U123","U999"]}`
	if slackCfg != wantCfg {
		t.Errorf("SLACK_CONFIG = %q, want %q", slackCfg, wantCfg)
	}
}

func TestApplyDeploymentSpec_TemplateContract_SlackAllowlist(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	input := deployment.TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "test-registry.example.com/my-agent:latest"},
			Dev: &spec.Dev{
				Interfaces: &spec.DevInterfaces{
					Messaging: &spec.DevMessaging{
						Adapters: []string{"slack"},
						Slack: &spec.SlackAdapterConfig{
							ActionableReactions: []string{"ticket", "bug"},
							AllowedChannelIDs:   []string{"C123", "C999"},
							AllowedUserIDs:      []string{"U123", "U999"},
							SocketMode:          boolPtr(false),
							AutoThread:          boolPtr(true),
						},
					},
				},
			},
		},
		AgentName:   "my-agent",
		Account:     "acme",
		BuildID:     "build-123",
		RegistryURL: "test-registry.example.com",
	}

	ds, err := deployment.GenerateDeploymentTemplate(input)
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}

	ds.Interfaces.Adapters = []string{"slack"}
	deployment.ResolveDeploymentSpecEnv(ds, deployment.ResolveContext{})

	a := newTestApplier()
	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error applying generated template: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	agentDepl, err := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("failed to get agent Deployment: %v", err)
	}

	var msgContainer *corev1.Container
	for i := range agentDepl.Spec.Template.Spec.InitContainers {
		if agentDepl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
			msgContainer = &agentDepl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if msgContainer == nil {
		t.Fatal("messaging sidecar container not found in agent Deployment")
	}

	envMap := make(map[string]string, len(msgContainer.Env))
	for _, e := range msgContainer.Env {
		envMap[e.Name] = e.Value
	}

	want := ds.Variables["SLACK_CONFIG"].Value
	got := envMap["SLACK_CONFIG"]
	if got != want {
		t.Errorf("SLACK_CONFIG = %q, want %q", got, want)
	}
}

func TestApplyDeploymentSpec_WithWebInterfaceExpose(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	a.acmCertificateARN = "arn:aws:acm:test"
	a.albGroupName = "test-group"
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {
				Port:     8080,
				Protocol: "http",
				Expose: &spec.EndpointExpose{
					Enabled: true,
					Domain:  "my-agent.custom.example.com",
				},
			},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasMsgService := false
	hasIngress := false
	hasEndpoint := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-messaging" {
			hasMsgService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			t.Error("messaging should be a sidecar, not a separate deployment")
		}
		if r.Kind == "Ingress" && r.Name == "my-agent-ingress-messaging" {
			hasIngress = true
		}
	}
	for _, ep := range result.ServiceEndpoints {
		if ep.Name == "messaging" && ep.URL == "https://my-agent.custom.example.com" {
			hasEndpoint = true
		}
	}
	if !hasMsgService {
		t.Error("expected messaging service")
	}
	if !hasIngress {
		t.Error("expected ingress for web adapter")
	}
	if !hasEndpoint {
		t.Error("expected service endpoint with custom domain")
	}
}

func TestApplyDeploymentSpec_AdapterExposedWhenDefined(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasIngress := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			hasIngress = true
		}
	}
	if !hasIngress {
		t.Error("expected ingress when adapter is defined and ingressDomain is set")
	}
}

func TestApplyDeploymentSpec_NoIngressWithoutDomain(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "" // no ingress domain configured
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			t.Error("expected no ingress when no ingressDomain or expose.domain is set")
		}
	}
}

func TestApplyDeploymentSpec_SlackOnlyNoIngress(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range result.Resources {
		if r.Kind == "Ingress" {
			t.Error("expected no ingress for slack-only adapter (uses socket mode)")
		}
	}
}

func TestApplyDeploymentSpec_InterfaceCustomResources(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 7070, Protocol: "grpc"},
		},
		Resources: spec.DeploymentResources{
			CPU: "200m", Memory: "256Mi",
			CPULimit: "1", MemoryLimit: "1Gi",
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Messaging is colocated — verify no separate deployment
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			t.Error("messaging should be a sidecar, not a separate deployment")
		}
	}
}

func TestApplyDeploymentSpec_WithFrontendExpose(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	a.acmCertificateARN = "arn:aws:acm:test"
	a.albGroupName = "test-group"
	ds := minimalDeploymentSpec()
	// Agent exposes its own frontend on port 80
	ds.Agent.Endpoints = map[string]spec.Endpoint{
		"http": {Port: 80, Protocol: "http", Expose: &spec.EndpointExpose{Enabled: true}},
	}
	// No messaging sidecar
	ds.Interfaces = nil

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	hasAgentIngress := false
	hasFrontendEndpoint := false
	for _, r := range result.Resources {
		if r.Kind == "Ingress" && r.Name == "my-agent-ingress-agent" {
			hasAgentIngress = true
		}
	}
	for _, ep := range result.ServiceEndpoints {
		if ep.Name == "agent" && ep.Type == "frontend" {
			hasFrontendEndpoint = true
		}
	}
	if !hasAgentIngress {
		t.Error("expected agent ingress for frontend")
	}
	if !hasFrontendEndpoint {
		t.Error("expected frontend service endpoint")
	}

	// Should NOT have messaging resources
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-messaging" {
			t.Error("should not have messaging service when interfaces is nil")
		}
	}
}

func TestApplyIngress_RejectsZeroPort(t *testing.T) {
	a := newTestApplier()
	ingress := BuildIngress(IngressConfig{
		Name: "test-ingress", Namespace: "default", AgentName: "agent",
		BuildID: "b1", Component: "messaging",
		ServiceName: "test-svc", ServicePort: 0, Host: "test.example.com",
	})

	status, err := a.applyIngress(context.Background(), ingress)
	if err == nil {
		t.Fatal("expected error when ingress has backend port 0")
	}
	if status.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", status.Status)
	}
}

func TestApplyDeploymentSpec_UpdateExistingResources(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ctx := context.Background()

	// First apply — creates
	result1, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if len(result1.Errors) > 0 {
		t.Fatalf("first apply errors: %v", result1.Errors)
	}

	// Second apply — updates
	result2, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(result2.Errors) > 0 {
		t.Fatalf("second apply errors: %v", result2.Errors)
	}

	// All resources should be "updated" on second apply
	for _, r := range result2.Resources {
		if r.Status != "updated" {
			t.Errorf("second apply: resource %s/%s: expected status 'updated', got %s", r.Kind, r.Name, r.Status)
		}
	}
}

// TestNetworkPolicies_NoPortlessEgressRule guards against a specific AWS VPC CNI
// bug: when an egress rule has only Ports and no To field, the PolicyEndpoint
// controller merges those ports onto the 0.0.0.0/0 ipBlock rule — restricting
// internet egress to port 53 only and blocking outbound API calls (e.g. OpenAI).
//
// This test enforces two invariants on the allow-namespace-traffic policy:
//  1. No egress rule is To-less with only Ports (the pattern that triggers the merge).
//  2. The 0.0.0.0/0 ipBlock egress rule carries no port restriction.
func TestNetworkPolicies_NoPortlessEgressRule(t *testing.T) {
	fakeClient := fake.NewClientset()
	a := &Applier{
		clientset:      fakeClient,
		namespace:      "test-ns",
		podSubnetCIDRs: []string{"10.3.11.0/24", "10.3.12.0/24"},
	}

	if err := a.applyNetworkPolicies(context.Background()); err != nil {
		t.Fatalf("applyNetworkPolicies: %v", err)
	}

	np, err := fakeClient.NetworkingV1().NetworkPolicies("test-ns").Get(
		context.Background(), "allow-namespace-traffic", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get allow-namespace-traffic: %v", err)
	}

	for i, rule := range np.Spec.Egress {
		// Invariant 1: every egress rule must have a To field.
		// A To-less ports-only rule is the pattern that causes the AWS VPC CNI
		// PolicyEndpoint controller to merge port restrictions onto the ipBlock rule.
		if len(rule.To) == 0 && len(rule.Ports) > 0 {
			t.Errorf(
				"egress rule %d has Ports but no To — AWS VPC CNI will merge these ports "+
					"onto the 0.0.0.0/0 ipBlock rule, blocking internet egress to all ports except those listed",
				i,
			)
		}

		// Invariant 2: the 0.0.0.0/0 internet egress rule must allow all ports.
		for _, peer := range rule.To {
			if peer.IPBlock != nil && peer.IPBlock.CIDR == "0.0.0.0/0" && len(rule.Ports) > 0 {
				t.Errorf(
					"egress rule %d targets 0.0.0.0/0 but restricts ports to %v — "+
						"pods will be unable to reach external APIs on unrestricted ports",
					i, rule.Ports,
				)
			}
		}
	}
}

// TestNetworkPolicies_MonitoringIngressRule verifies that the allow-namespace-traffic
// policy includes an ingress rule allowing the monitoring namespace to reach
// port 9091 (Alloy scraping messaging sidecar metrics).
func TestNetworkPolicies_MonitoringIngressRule(t *testing.T) {
	fakeClient := fake.NewClientset()
	a := &Applier{
		clientset:      fakeClient,
		namespace:      "test-ns",
		podSubnetCIDRs: []string{"10.3.11.0/24", "10.3.12.0/24"},
	}

	if err := a.applyNetworkPolicies(context.Background()); err != nil {
		t.Fatalf("applyNetworkPolicies: %v", err)
	}

	np, err := fakeClient.NetworkingV1().NetworkPolicies("test-ns").Get(
		context.Background(), "allow-namespace-traffic", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get allow-namespace-traffic: %v", err)
	}

	// Find an ingress rule that selects the monitoring namespace on port 9091.
	found := false
	for _, rule := range np.Spec.Ingress {
		for _, from := range rule.From {
			if from.NamespaceSelector == nil {
				continue
			}
			if from.NamespaceSelector.MatchLabels["name"] != "monitoring" {
				continue
			}
			// Verify the rule is scoped to TCP 9091.
			if len(rule.Ports) != 1 {
				t.Fatalf("monitoring ingress rule has %d ports, want 1", len(rule.Ports))
			}
			p := rule.Ports[0]
			if p.Protocol == nil || *p.Protocol != corev1.ProtocolTCP {
				t.Error("monitoring ingress rule protocol is not TCP")
			}
			if p.Port == nil || p.Port.IntValue() != 9091 {
				t.Errorf("monitoring ingress rule port = %v, want 9091", p.Port)
			}
			found = true
		}
	}
	if !found {
		t.Error("allow-namespace-traffic is missing an ingress rule for the monitoring namespace")
	}
}

// TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer verifies that when
// secret variables are present in the spec (i.e. after rehydration), the K8s
// Secret is created with the correct values AND the messaging sidecar container
// has envFrom referencing that secret.
func TestApplyDeploymentSpec_SlackSecretsOnMessagingContainer(t *testing.T) {
	slackSpec := func() *spec.AstroDeploymentSpec {
		ds := minimalDeploymentSpec()
		ds.Interfaces = &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Image:    "test-registry.example.com/messaging:latest",
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
			},
			Environment: map[string]string{
				"SLACK_BOT_TOKEN": "${variables.SLACK_BOT_TOKEN}",
				"SLACK_APP_TOKEN": "${variables.SLACK_APP_TOKEN}",
			},
		}
		return ds
	}

	t.Run("stripped spec produces no secret", func(t *testing.T) {
		a := newTestApplier()
		ds := slackSpec()
		// Simulate stripped spec: secret variables exist but values are empty
		ds.Variables = map[string]spec.Variable{
			"SLACK_BOT_TOKEN": {Value: "", Secret: true, Targets: []string{"interface.slack"}},
			"SLACK_APP_TOKEN": {Value: "", Secret: true, Targets: []string{"interface.slack"}},
		}

		result, err := a.ApplyDeploymentSpec(context.Background(), ds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Errors) > 0 {
			t.Fatalf("unexpected apply errors: %v", result.Errors)
		}

		// No Secret should exist — secret values are empty
		secrets, err := a.clientset.CoreV1().Secrets("default").List(
			context.Background(), metav1.ListOptions{},
		)
		if err != nil {
			t.Fatalf("list secrets: %v", err)
		}
		if len(secrets.Items) != 0 {
			t.Errorf("expected 0 secrets for stripped spec, got %d", len(secrets.Items))
		}
	})

	t.Run("rehydrated spec creates two secrets and scopes envFrom correctly", func(t *testing.T) {
		a := newTestApplier()
		ds := slackSpec()
		// Simulate rehydrated spec: secret variables have values restored
		ds.Variables = map[string]spec.Variable{
			"SLACK_BOT_TOKEN": {Value: "xoxb-real-token", Secret: true, Targets: []string{"interface.slack"}},
			"SLACK_APP_TOKEN": {Value: "xapp-real-token", Secret: true, Targets: []string{"interface.slack"}},
		}

		result, err := a.ApplyDeploymentSpec(context.Background(), ds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Errors) > 0 {
			t.Fatalf("unexpected apply errors: %v", result.Errors)
		}

		// The agent's main credentials Secret no longer carries
		// interface-only variables (SLACK_BOT_TOKEN / SLACK_APP_TOKEN
		// target only "interface.slack"; the scope filter excludes them
		// from the agent's bundle). With only interface-only variables in
		// this fixture, the agent's Secret should not exist at all.
		agentSecretName := deployment.GenerateSecretName("my-agent", "build-123")
		if _, err := a.clientset.CoreV1().Secrets("default").Get(
			context.Background(), agentSecretName, metav1.GetOptions{},
		); err == nil {
			t.Errorf("agent secret %q should not exist when the deployment has only interface-targeted secrets", agentSecretName)
		}

		// The messaging-only Secret holds the slack tokens — the messaging
		// sidecar mounts this narrower bundle and never sees the agent's
		// credentials.
		msgSecretName := deployment.GenerateMessagingSecretName("my-agent", "build-123")
		msgSecret, err := a.clientset.CoreV1().Secrets("default").Get(
			context.Background(), msgSecretName, metav1.GetOptions{},
		)
		if err != nil {
			t.Fatalf("get messaging secret %q: %v", msgSecretName, err)
		}
		if got := string(msgSecret.Data["SLACK_BOT_TOKEN"]); got != "xoxb-real-token" {
			t.Errorf("messaging secret SLACK_BOT_TOKEN: got %q, want %q", got, "xoxb-real-token")
		}
		if got := string(msgSecret.Data["SLACK_APP_TOKEN"]); got != "xapp-real-token" {
			t.Errorf("messaging secret SLACK_APP_TOKEN: got %q, want %q", got, "xapp-real-token")
		}

		// Verify the agent deployment's messaging container envFrom points at
		// the messaging-only Secret, NOT the agent secret.
		deplName := deployment.GenerateAgentResourceName("my-agent", "agent")
		depl, err := a.clientset.AppsV1().Deployments("default").Get(
			context.Background(), deplName, metav1.GetOptions{},
		)
		if err != nil {
			t.Fatalf("get deployment %q: %v", deplName, err)
		}

		var msgContainer *corev1.Container
		for i := range depl.Spec.Template.Spec.InitContainers {
			if depl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
				msgContainer = &depl.Spec.Template.Spec.InitContainers[i]
				break
			}
		}
		if msgContainer == nil {
			t.Fatal("messaging container not found in agent deployment")
		}

		var refNames []string
		for _, ef := range msgContainer.EnvFrom {
			if ef.SecretRef != nil {
				refNames = append(refNames, ef.SecretRef.Name)
			}
		}
		foundMsgSecret := false
		for _, n := range refNames {
			if n == agentSecretName {
				t.Errorf("messaging container should NOT mount agent secret %q; envFrom secrets: %v", agentSecretName, refNames)
			}
			if n == msgSecretName {
				foundMsgSecret = true
			}
		}
		if !foundMsgSecret {
			t.Errorf("messaging container missing envFrom secretRef %q; envFrom secrets: %v", msgSecretName, refNames)
		}

		// Check SLACK_ENABLED env var is set on the messaging container
		foundSlackEnabled := false
		for _, ev := range msgContainer.Env {
			if ev.Name == "SLACK_ENABLED" && ev.Value == "true" {
				foundSlackEnabled = true
			}
		}
		if !foundSlackEnabled {
			t.Error("messaging container missing SLACK_ENABLED=true env var")
		}

		// The main agent container's envFrom no longer references the
		// (now non-existent) agent Secret, since this fixture has only
		// interface-targeted secrets. With non-interface secrets in the
		// spec, the agent Secret would exist and the agent would mount it.
		var agentContainer *corev1.Container
		for i := range depl.Spec.Template.Spec.Containers {
			c := &depl.Spec.Template.Spec.Containers[i]
			if c.Name != "messaging" && !strings.HasPrefix(c.Name, "collector") {
				agentContainer = c
				break
			}
		}
		if agentContainer == nil {
			t.Fatal("agent container not found")
		}
		for _, ef := range agentContainer.EnvFrom {
			if ef.SecretRef != nil && ef.SecretRef.Name == agentSecretName {
				t.Errorf("agent container should not mount %q when only interface-targeted secrets exist", agentSecretName)
			}
		}
	})
}

func TestApplyDeploymentSpec_ManagedAnthropicKey(t *testing.T) {
	fakeClient := fake.NewClientset()
	a := &Applier{
		clientset:              fakeClient,
		namespace:              "test-ns",
		registryURL:            "test-registry.example.com",
		imageResolver:          NewImageResolver("", "test-registry.example.com", "test"),
		imagePullPolicy:        corev1.PullNever,
		managedAnthropicAPIKey: "sk-ant-managed-test",
	}
	ds := minimalDeploymentSpec()
	ctx := context.Background()

	_, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The managed key should appear in the K8s Secret
	secretName := deployment.GenerateSecretName(ds.Source.Name, ds.Source.Build)
	secret, err := fakeClient.CoreV1().Secrets("test-ns").Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get secret %q: %v", secretName, err)
	}
	got := string(secret.Data["ANTHROPIC_API_KEY"])
	if got != "sk-ant-managed-test" {
		t.Errorf("ANTHROPIC_API_KEY in secret = %q, want %q", got, "sk-ant-managed-test")
	}
}

// TestApplyCronJob_SuspendedCronJobIsUnsuspendedOnApply verifies the fix to
// applyCronJob: when a CronJob already exists (e.g. after a pause set
// Suspend=true), a subsequent ApplyDeploymentSpec call must fetch the existing
// resource version and update it — unsuspending the CronJob in the process.
func TestApplyCronJob_SuspendedCronJobIsUnsuspendedOnApply(t *testing.T) {
	cronJobName := deployment.GenerateResourceName("my-agent", "ingestion", "daily")
	suspend := true
	existing := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cronJobName,
			Namespace:       "default",
			ResourceVersion: "42",
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 0 * * *",
			Suspend:  &suspend,
		},
	}

	fakeClient := fake.NewClientset(existing)
	a := &Applier{
		clientset:       fakeClient,
		namespace:       "default",
		registryURL:     "test-registry.example.com",
		imageResolver:   NewImageResolver("", "test-registry.example.com", "test"),
		imagePullPolicy: corev1.PullNever,
	}

	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"daily": {
			Image:     "test-registry.example.com/my-agent:latest",
			Resources: spec.StandardResources,
			Trigger:   spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"},
		},
	}

	ctx := context.Background()
	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	updated, err := fakeClient.BatchV1().CronJobs("default").Get(ctx, cronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get CronJob after apply: %v", err)
	}
	if updated.Spec.Suspend != nil && *updated.Spec.Suspend {
		t.Error("expected CronJob to be unsuspended after apply, but Suspend was still true")
	}
}

func TestApplyDeploymentSpec_OIDCAuth_EnabledWhenOptIn(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	a.messagingOIDCAuth = &OIDCAuthConfig{
		Issuer:                "https://auth.example.com",
		AuthorizationEndpoint: "https://auth.example.com/oauth2/authorize",
		TokenEndpoint:         "https://auth.example.com/oauth2/token",
		UserInfoEndpoint:      "https://auth.example.com/oauth2/userinfo",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
	}
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
		Auth: &spec.DeploymentInterfacesAuth{
			Web: &spec.DeploymentWebAuth{Type: "oidc"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	// OIDC secret should be created
	hasOIDCSecret := false
	for _, r := range result.Resources {
		if r.Kind == "Secret" && r.Name == messagingOIDCSecretName {
			hasOIDCSecret = true
		}
	}
	if !hasOIDCSecret {
		t.Error("expected messaging-oidc secret to be created when auth.web.type is oidc")
	}

	// Ingress should have OIDC auth annotations
	fakeClient := a.clientset.(*fake.Clientset)
	ing, err := fakeClient.NetworkingV1().Ingresses("default").Get(
		context.Background(), "my-agent-ingress-messaging", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("failed to get ingress: %v", err)
	}
	if ing.Annotations["alb.ingress.kubernetes.io/auth-type"] != "oidc" {
		t.Errorf("expected auth-type oidc annotation, got %q", ing.Annotations["alb.ingress.kubernetes.io/auth-type"])
	}
}

func TestApplyDeploymentSpec_OIDCAuth_DisabledWhenNotOptIn(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	a.messagingOIDCAuth = &OIDCAuthConfig{
		Issuer:       "https://auth.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
		// No Auth field — default is no auth
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No OIDC secret should be created
	for _, r := range result.Resources {
		if r.Kind == "Secret" && r.Name == messagingOIDCSecretName {
			t.Error("expected no messaging-oidc secret when auth not opted in")
		}
	}

	// Ingress should have no OIDC annotations
	fakeClient := a.clientset.(*fake.Clientset)
	ing, err := fakeClient.NetworkingV1().Ingresses("default").Get(
		context.Background(), "my-agent-ingress-messaging", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("failed to get ingress: %v", err)
	}
	if _, ok := ing.Annotations["alb.ingress.kubernetes.io/auth-type"]; ok {
		t.Error("expected no auth-type annotation when auth not opted in")
	}
}

func TestApplyDeploymentSpec_OIDCAuth_DisabledWhenServerNotConfigured(t *testing.T) {
	a := newTestApplier()
	a.ingressDomain = "example.com"
	// messagingOIDCAuth is nil — server not configured
	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
		Auth: &spec.DeploymentInterfacesAuth{
			Web: &spec.DeploymentWebAuth{Type: "oidc"},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No OIDC secret or annotations — server config missing
	for _, r := range result.Resources {
		if r.Kind == "Secret" && r.Name == messagingOIDCSecretName {
			t.Error("expected no messaging-oidc secret when server OIDC not configured")
		}
	}

	fakeClient := a.clientset.(*fake.Clientset)
	ing, err := fakeClient.NetworkingV1().Ingresses("default").Get(
		context.Background(), "my-agent-ingress-messaging", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("failed to get ingress: %v", err)
	}
	if _, ok := ing.Annotations["alb.ingress.kubernetes.io/auth-type"]; ok {
		t.Error("expected no auth-type annotation when server OIDC not configured")
	}
}

// Verify both the agent container and the messaging sidecar receive
// ASTRO_AUTHZ_TOKEN. The token is signed once per deploy and injected on
// both so each can authenticate calls back to astro-server.
func TestApplyDeploymentSpec_IdentityTokenInjectedIntoAgentAndMessaging(t *testing.T) {
	a := newTestApplier()
	a.deploymentID = "dep-123"
	a.deployTokenSecret = "test-secret"

	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("apply: %v", err)
	}

	depl, err := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}

	// Locate both containers in the pod.
	var agentContainer, msgContainer *corev1.Container
	for i := range depl.Spec.Template.Spec.Containers {
		c := &depl.Spec.Template.Spec.Containers[i]
		if c.Name != "messaging" && !strings.HasPrefix(c.Name, "collector") {
			agentContainer = c
			break
		}
	}
	for i := range depl.Spec.Template.Spec.InitContainers {
		if depl.Spec.Template.Spec.InitContainers[i].Name == "messaging" {
			msgContainer = &depl.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if agentContainer == nil {
		t.Fatal("agent container not found")
	}
	if msgContainer == nil {
		t.Fatal("messaging container not found")
	}

	getEnv := func(c *corev1.Container, name string) (string, bool) {
		for _, e := range c.Env {
			if e.Name == name {
				return e.Value, true
			}
		}
		return "", false
	}

	agentTok, agentOK := getEnv(agentContainer, "ASTRO_AUTHZ_TOKEN")
	if !agentOK {
		t.Fatal("agent container is missing ASTRO_AUTHZ_TOKEN")
	}
	msgTok, msgOK := getEnv(msgContainer, "ASTRO_AUTHZ_TOKEN")
	if !msgOK {
		t.Fatal("messaging container is missing ASTRO_AUTHZ_TOKEN")
	}
	if agentTok == "" || msgTok == "" {
		t.Errorf("ASTRO_AUTHZ_TOKEN must be non-empty (agent=%q messaging=%q)", agentTok, msgTok)
	}
	if agentTok != msgTok {
		t.Errorf("agent and messaging should share the same identity token; got distinct values")
	}
}

// An `anyone` grant under slack must populate the deploy token's
// anyone_adapters claim with "slack" so the messaging container can fast-path
// slack traffic the same way it does for web.
func TestApplyDeploymentSpec_SlackAnyoneGrantInToken(t *testing.T) {
	a := newTestApplier()
	a.deploymentID = "dep-123"
	a.deployTokenSecret = "test-secret"

	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
		Auth: &spec.DeploymentInterfacesAuth{
			Slack: &spec.DeploymentSlackAuth{
				Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}},
			},
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("apply: %v", err)
	}

	depl, err := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}

	var token string
	for _, c := range depl.Spec.Template.Spec.Containers {
		for _, e := range c.Env {
			if e.Name == "ASTRO_AUTHZ_TOKEN" {
				token = e.Value
			}
		}
	}
	if token == "" {
		t.Fatal("ASTRO_AUTHZ_TOKEN not set on agent container")
	}
	_, anyoneAdapters, err := deploytoken.Verify(token, "test-secret")
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !slices.Contains(anyoneAdapters, "slack") {
		t.Errorf("expected anyone_adapters to contain \"slack\", got %v", anyoneAdapters)
	}
}

// If grants are configured but the deploy-token secret is unset, the apply
// must refuse — the messaging container would otherwise fall back to
// AllowAll() and the spec's grants would be silently unenforced.
func TestApplyDeploymentSpec_RefusesGrantsWithoutSecret(t *testing.T) {
	a := newTestApplier()
	a.deploymentID = "dep-123"
	// a.deployTokenSecret intentionally empty

	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
		},
		Auth: &spec.DeploymentInterfacesAuth{
			Web: &spec.DeploymentWebAuth{
				Grants: []spec.DeploymentAuthorizationGrant{{Anyone: true}},
			},
		},
	}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected a deploy error refusing to apply without DEPLOY_TOKEN_SECRET")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error, "DEPLOY_TOKEN_SECRET") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DEPLOY_TOKEN_SECRET in error message; got %+v", result.Errors)
	}
}

// When deployTokenSecret is empty (local dev with no secret configured) and
// no grants are configured, no token is signed and neither container
// receives ASTRO_AUTHZ_TOKEN.
func TestApplyDeploymentSpec_IdentityTokenSkippedWhenSecretUnset(t *testing.T) {
	a := newTestApplier()
	a.deploymentID = "dep-123"
	// a.deployTokenSecret left empty

	ds := minimalDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "test-registry.example.com/messaging:latest",
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http"},
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("apply: %v", err)
	}

	depl, _ := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	for _, c := range depl.Spec.Template.Spec.Containers {
		for _, e := range c.Env {
			if e.Name == "ASTRO_AUTHZ_TOKEN" {
				t.Errorf("agent container %q should not have ASTRO_AUTHZ_TOKEN when secret unset", c.Name)
			}
		}
	}
	for _, c := range depl.Spec.Template.Spec.InitContainers {
		for _, e := range c.Env {
			if e.Name == "ASTRO_AUTHZ_TOKEN" {
				t.Errorf("init container %q should not have ASTRO_AUTHZ_TOKEN when secret unset", c.Name)
			}
		}
	}
}

func TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Agent(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"db": {
			Image:     "test-registry.example.com/pgvector:latest",
			Endpoints: httpEp(5432),
			Replicas:  1,
			Update:    spec.DefaultUpdateStrategy(),
			Provider:  "postgres",
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	depl, err := a.clientset.AppsV1().Deployments("default").Get(
		context.Background(), "my-agent-agent", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get agent deployment: %v", err)
	}

	var agentContainer *corev1.Container
	for i := range depl.Spec.Template.Spec.Containers {
		c := &depl.Spec.Template.Spec.Containers[i]
		if c.Name != "messaging" && !strings.HasPrefix(c.Name, "collector") {
			agentContainer = c
			break
		}
	}
	if agentContainer == nil {
		t.Fatal("agent container not found")
	}

	// Agent must receive POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB via
	// secretKeyRef — not as plain env values.
	credSecretName := knowledgeCredSecretName("my-agent", "db")
	wantKeys := []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"}
	for _, key := range wantKeys {
		found := false
		for _, ev := range agentContainer.Env {
			if ev.Name == key && ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil &&
				ev.ValueFrom.SecretKeyRef.Name == credSecretName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agent container missing secretKeyRef for %s (secret %q)", key, credSecretName)
		}
	}
}

func TestApplyDeploymentSpec_KnowledgeCredSecretKeyRefs_Ingestion(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"db": {
			Image:     "test-registry.example.com/pgvector:latest",
			Endpoints: httpEp(5432),
			Replicas:  1,
			Update:    spec.DefaultUpdateStrategy(),
			Provider:  "postgres",
		},
	}
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"sync": {
			Image:   "test-registry.example.com/sync:latest",
			Trigger: spec.DeploymentTrigger{Type: "startup"},
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobName := deployment.GenerateResourceName("my-agent", "ingestion", "sync")
	job, err := a.clientset.BatchV1().Jobs("default").Get(
		context.Background(), jobName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get ingestion job: %v", err)
	}

	if len(job.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("ingestion job has no containers")
	}
	container := &job.Spec.Template.Spec.Containers[0]

	credSecretName := knowledgeCredSecretName("my-agent", "db")
	wantKeys := []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"}
	for _, key := range wantKeys {
		found := false
		for _, ev := range container.Env {
			if ev.Name == key && ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil &&
				ev.ValueFrom.SecretKeyRef.Name == credSecretName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ingestion container missing secretKeyRef for %s (secret %q)", key, credSecretName)
		}
	}
}

// K8: ingestion container mounts the deployment ConfigMap and Secret via envFrom.
func TestApplyDeploymentSpec_K8_IngestionContainerMountsConfigMapAndSecret(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Variables = map[string]spec.Variable{
		"PLAIN_VAR":  {Value: "hello", Secret: false, Targets: []string{"agent", "ingestion"}},
		"SECRET_VAR": {Value: "secret", Secret: true, Targets: []string{"agent", "ingestion"}},
	}
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"nightly": {
			Image:   "test-registry.example.com/sync:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 0 * * *"},
		},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cronJobName := deployment.GenerateResourceName("my-agent", "ingestion", "nightly")
	cronJob, err := a.clientset.BatchV1().CronJobs("default").Get(
		context.Background(), cronJobName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get CronJob: %v", err)
	}

	if len(cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("ingestion CronJob has no containers")
	}
	container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	expectedCM := deployment.GenerateConfigMapName("my-agent", "build-123")
	expectedSecret := deployment.GenerateSecretName("my-agent", "build-123")

	hasCM, hasSecret := false, false
	for _, ef := range container.EnvFrom {
		if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name == expectedCM {
			hasCM = true
		}
		if ef.SecretRef != nil && ef.SecretRef.Name == expectedSecret {
			hasSecret = true
		}
	}
	if !hasCM {
		t.Errorf("K8: ingestion container missing envFrom ConfigMapRef %q", expectedCM)
	}
	if !hasSecret {
		t.Errorf("K8: ingestion container missing envFrom SecretRef %q", expectedSecret)
	}
}

// K10: secret variable values never appear in the ConfigMap — they only go into the Secret.
func TestApplyDeploymentSpec_K10_SecretValueNotInConfigMap(t *testing.T) {
	a := newTestApplier()
	ds := minimalDeploymentSpec()
	ds.Agent.Environment = map[string]string{
		"API_KEY":   "${variables.API_KEY}",
		"LOG_LEVEL": "${variables.LOG_LEVEL}",
	}
	ds.Variables = map[string]spec.Variable{
		"API_KEY":   {Value: "sk-top-secret", Secret: true},
		"LOG_LEVEL": {Value: "debug", Secret: false},
	}

	if _, err := a.ApplyDeploymentSpec(context.Background(), ds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmName := deployment.GenerateConfigMapName("my-agent", "build-123")
	cm, err := a.clientset.CoreV1().ConfigMaps("default").Get(
		context.Background(), cmName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("K10: get ConfigMap: %v", err)
	}
	if _, ok := cm.Data["API_KEY"]; ok {
		t.Error("K10: secret value API_KEY must not appear in the ConfigMap")
	}
	if cm.Data["LOG_LEVEL"] != "debug" {
		t.Errorf("K10: non-secret LOG_LEVEL missing from ConfigMap, got %v", cm.Data)
	}
}
