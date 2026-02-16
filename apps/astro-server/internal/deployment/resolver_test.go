package deployment

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
)

func baseDeploymentSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: spec.DeploymentSource{Name: "agent", Build: "b1", Account: "acme"},
		Target: spec.DeploymentTarget{Namespace: "prod", Runtime: "kubernetes"},
		Agent:  spec.DeploymentAgent{Image: "agent:latest", Port: 8080},
	}
}

func TestValidateAndResolve_Valid(t *testing.T) {
	ds := baseDeploymentSpec()
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if result.Spec == nil {
		t.Fatal("expected resolved spec")
	}
}

func TestValidateAndResolve_MissingNamespace(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Target.Namespace = ""
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing namespace")
	}
	found := false
	for _, e := range result.Errors {
		if e == "target.namespace: namespace is required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected namespace error, got %v", result.Errors)
	}
}

func TestValidateAndResolve_EmptyRequiredCredential(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Credentials = map[string]spec.DeploymentCredential{
		"API_KEY": {Value: "", Description: "required key"},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for empty required credential")
	}
}

func TestValidateAndResolve_OptionalEmptyCredential(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Credentials = map[string]spec.DeploymentCredential{
		"OPTIONAL_KEY": {Value: "", Description: "optional", Optional: true},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors for optional empty credential, got %v", result.Errors)
	}
}

func TestValidateAndResolve_InvalidCronExpression(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "not-a-cron"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for invalid cron")
	}
}

func TestValidateAndResolve_ValidCronExpression(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: "0 * * * *"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidateAndResolve_MissingScheduleForScheduleTrigger(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing schedule")
	}
}

func TestValidateAndResolve_InvalidAdapterName(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"telegram"},
		Image:    "messaging:latest",
		Port:     9090,
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for unknown adapter")
	}
}

func TestValidateAndResolve_SlackAdapterRequiresTokens(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "messaging:latest",
		Port:     9090,
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing slack tokens")
	}
}

func TestValidateAndResolve_SlackAdapterWithTokens(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"slack"},
		Image:    "messaging:latest",
		Port:     9090,
	}
	ds.Credentials = map[string]spec.DeploymentCredential{
		"SLACK_BOT_TOKEN": {Value: "xoxb-123"},
		"SLACK_APP_TOKEN": {Value: "xapp-456"},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidateAndResolve_InvalidReference(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Agent.Environment = map[string]string{
		"LLM_URL": "${models.nonexistent.url}",
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for invalid model reference")
	}
}

func TestValidateAndResolve_ValidReference(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Agent.Environment = map[string]string{
		"LLM_URL": "${models.llm.url}",
	}
	ds.Models = map[string]spec.DeploymentModel{
		"llm": {Image: "ollama:latest", Port: 11434},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidateAndResolve_AppliesDefaults(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Agent.Replicas = 0
	ds.Agent.Update = spec.UpdateStrategy{}
	ds.Models = map[string]spec.DeploymentModel{
		"llm": {Image: "ollama:latest", Port: 11434},
	}
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"docs": {Image: "qdrant:latest", Port: 6333, Persistent: true},
	}
	ds.Tools = map[string]spec.DeploymentTool{
		"search": {Image: "search:latest", Port: 3000},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}

	resolved := result.Spec

	// Agent defaults
	if resolved.Agent.Replicas != 1 {
		t.Errorf("agent replicas: expected 1, got %d", resolved.Agent.Replicas)
	}
	if resolved.Agent.Update.Strategy != "rolling" {
		t.Errorf("agent strategy: expected rolling, got %s", resolved.Agent.Update.Strategy)
	}

	// Model defaults
	if resolved.Models["llm"].Replicas != 1 {
		t.Errorf("model replicas: expected 1, got %d", resolved.Models["llm"].Replicas)
	}
	if resolved.Models["llm"].Update.Strategy != "rolling" {
		t.Errorf("model strategy: expected rolling, got %s", resolved.Models["llm"].Update.Strategy)
	}

	// Knowledge defaults
	if resolved.Knowledge["docs"].Replicas != 1 {
		t.Errorf("knowledge replicas: expected 1, got %d", resolved.Knowledge["docs"].Replicas)
	}
	if resolved.Knowledge["docs"].Storage == nil {
		t.Error("expected default storage for persistent knowledge")
	}

	// Tool defaults
	if resolved.Tools["search"].Replicas != 1 {
		t.Errorf("tool replicas: expected 1, got %d", resolved.Tools["search"].Replicas)
	}
}

func TestValidateAndResolve_StripsEditable(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Editable = []string{"credentials"}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Spec.Editable != nil {
		t.Error("expected editable to be stripped after resolution")
	}
}

func TestValidateAndResolve_WebhookIngestionMissingPort(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"data": {
			Image:   "ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "webhook"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for webhook ingestion without port")
	}
}

func TestValidateAndResolve_WebhookIngestionWithPort(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"data": {
			Image:   "ingest:latest",
			Port:    3001,
			Trigger: spec.DeploymentTrigger{Type: "webhook"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidateAndResolve_MissingIngestionImage(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"data": {
			Trigger: spec.DeploymentTrigger{Type: "startup"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing ingestion image")
	}
}

func TestValidateAndResolve_MissingModelPort(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Models = map[string]spec.DeploymentModel{
		"llm": {Image: "ollama:latest"},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for model without port")
	}
}

func TestValidateAndResolve_MissingToolImage(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Tools = map[string]spec.DeploymentTool{
		"search": {Port: 3000},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for tool without image")
	}
}

func TestValidateAndResolve_DiscordAdapterRejected(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"discord"},
		Image:    "messaging:latest",
		Port:     9090,
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for unsupported discord adapter")
	}
}

func TestValidateAndResolve_InterfacesReferenceValidation(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "messaging:latest",
		Port:     9090,
		Environment: map[string]string{
			"AGENT_URL": "${models.missing.url}",
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for interfaces reference to missing model")
	}
}
