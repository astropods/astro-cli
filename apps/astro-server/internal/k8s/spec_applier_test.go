package k8s

import (
	"context"
	"testing"

	"github.com/postman/astro/packages/astro-spec"
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
		Target: spec.DeploymentTarget{Namespace: "test-ns", Runtime: "kubernetes"},
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

	// Verify namespace was set
	if a.namespace != "test-ns" {
		t.Errorf("expected namespace to be test-ns, got %s", a.namespace)
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
	ds.Tools = map[string]spec.DeploymentTool{
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
		if r.Kind == "Service" && r.Name == "my-agent-tool-search" {
			hasToolService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-tool-search" {
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
	a.galileoAPIKey = "gal-key"
	a.galileoProject = "test-project"
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{Enabled: true}

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

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
	a.galileoAPIKey = "gal-key"
	ds := minimalDeploymentSpec()
	ds.Observability = spec.DeploymentObservability{
		Enabled:  true,
		Provider: "galileo",
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

	hasCollectorDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "my-agent-collector" {
			hasCollectorDeployment = true
		}
	}
	if !hasCollectorDeployment {
		t.Error("expected collector deployment with custom port")
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

func TestApplyDeploymentSpec_NamespaceFallback(t *testing.T) {
	a := newTestApplier()
	a.namespace = "fallback-ns"
	ds := minimalDeploymentSpec()
	ds.Target.Namespace = "" // empty should fall back

	result, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}

	// Should have used fallback namespace
	if a.namespace != "fallback-ns" {
		t.Errorf("expected namespace fallback-ns, got %s", a.namespace)
	}
}

func TestApplyDeploymentSpec_NamespaceOverride(t *testing.T) {
	a := newTestApplier()
	a.namespace = "old-ns"
	ds := minimalDeploymentSpec()
	ds.Target.Namespace = "new-ns"

	_, err := a.ApplyDeploymentSpec(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if a.namespace != "new-ns" {
		t.Errorf("expected namespace new-ns, got %s", a.namespace)
	}
}

func TestApplyDeploymentSpec_FullStack(t *testing.T) {
	a := newTestApplier()
	a.galileoAPIKey = "gal-key"
	a.galileoProject = "project"
	ds := minimalDeploymentSpec()
	ds.Agent.Environment = map[string]string{
		"LLM_URL":  "${models.llm.http.url}",
		"DB_HOST":  "${knowledge.docs.host}",
		"TOOL_URL": "${tools.search.http.url}",
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
	ds.Tools = map[string]spec.DeploymentTool{
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

	// Expected: 1 Secret + 1 ConfigMap + 4 Services (model, knowledge, tool, agent) + 1 collector service
	//           + 1 model Deployment + 1 knowledge StatefulSet + 1 tool Deployment + 1 agent Deployment
	//           + 1 collector Deployment + 1 CronJob
	if counts["Secret"] != 1 {
		t.Errorf("expected 1 Secret, got %d", counts["Secret"])
	}
	if counts["ConfigMap"] != 1 {
		t.Errorf("expected 1 ConfigMap, got %d", counts["ConfigMap"])
	}
	if counts["Service"] < 4 {
		t.Errorf("expected at least 4 Services, got %d", counts["Service"])
	}
	if counts["Deployment"] < 3 {
		t.Errorf("expected at least 3 Deployments (model, tool, agent), got %d", counts["Deployment"])
	}
	if counts["StatefulSet"] != 1 {
		t.Errorf("expected 1 StatefulSet, got %d", counts["StatefulSet"])
	}
	if counts["CronJob"] != 1 {
		t.Errorf("expected 1 CronJob, got %d", counts["CronJob"])
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
		if r.Namespace != "test-ns" {
			t.Errorf("resource %s/%s: expected namespace test-ns, got %s", r.Kind, r.Name, r.Namespace)
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

	hasMsgService := false
	hasMsgDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-messaging" {
			hasMsgService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			hasMsgDeployment = true
		}
	}
	if !hasMsgService {
		t.Error("expected messaging service")
	}
	if !hasMsgDeployment {
		t.Error("expected messaging deployment")
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
	hasMsgDeployment := false
	hasIngress := false
	hasEndpoint := false
	for _, r := range result.Resources {
		if r.Kind == "Service" && r.Name == "my-agent-messaging" {
			hasMsgService = true
		}
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			hasMsgDeployment = true
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
	if !hasMsgDeployment {
		t.Error("expected messaging deployment")
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

	// Verify the deployment was created with the custom port
	hasMsgDeployment := false
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "my-agent-messaging" {
			hasMsgDeployment = true
		}
	}
	if !hasMsgDeployment {
		t.Error("expected messaging deployment with custom port")
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
