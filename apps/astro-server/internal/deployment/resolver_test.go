package deployment

import (
	"testing"
)

func agentEndpoints() map[string]Endpoint {
	return map[string]Endpoint{"http": {Port: 8080}}
}

func baseDeploymentSpec() *AstroDeploymentSpec {
	return &AstroDeploymentSpec{
		Spec:   "deployment/v1",
		Source: DeploymentSource{Name: "agent", Build: "b1", Account: "acme"},
		Target: DeploymentTarget{Runtime: "kubernetes"},
		Agent:  DeploymentAgent{Image: "agent:latest", Endpoints: agentEndpoints()},
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

func TestValidateAndResolve_EmptyRequiredVariable(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Variables = map[string]Variable{
		"API_KEY": {Value: "", Description: "required key", Secret: true},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for empty required variable")
	}
}

func TestValidateAndResolve_ConfiguredRequiredSecret(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Variables = map[string]Variable{
		"API_KEY": {Secret: true, Configured: true},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected configured secret to be valid during preflight, got %v", result.Errors)
	}
}

func TestValidateAndResolve_OptionalEmptyVariable(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Variables = map[string]Variable{
		"OPTIONAL_KEY": {Value: "", Description: "optional", Optional: true, Secret: true},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors for optional empty variable, got %v", result.Errors)
	}
}

func TestValidateAndResolve_InvalidCronExpression(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: DeploymentTrigger{Type: "schedule", Schedule: "not-a-cron"},
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
	ds.Ingestion = map[string]DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: DeploymentTrigger{Type: "schedule", Schedule: "0 * * * *"},
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
	ds.Ingestion = map[string]DeploymentIngestion{
		"ingest": {
			Image:   "ingest:latest",
			Trigger: DeploymentTrigger{Type: "schedule"},
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
	ds.Interfaces = &DeploymentInterfaces{
		Adapters:  []string{"telegram"},
		Image:     "messaging:latest",
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090}},
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
	ds.Interfaces = &DeploymentInterfaces{
		Adapters:  []string{"slack"},
		Image:     "messaging:latest",
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090}},
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
	ds.Interfaces = &DeploymentInterfaces{
		Adapters:  []string{"slack"},
		Image:     "messaging:latest",
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090}},
	}
	ds.Variables = map[string]Variable{
		"SLACK_BOT_TOKEN": {Value: "xoxb-123", Secret: true},
		"SLACK_APP_TOKEN": {Value: "xapp-456", Secret: true},
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
		"LLM_URL": "${models.nonexistent.http.url}",
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
		"LLM_URL": "${models.llm.http.url}",
	}
	ds.Models = map[string]DeploymentModel{
		"llm": {Image: "my-model:latest", Endpoints: map[string]Endpoint{"http": {Port: 8000}}},
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
	ds.Agent.Update = UpdateStrategy{}
	ds.Models = map[string]DeploymentModel{
		"llm": {Image: "my-model:latest", Endpoints: map[string]Endpoint{"http": {Port: 8000}}},
	}
	ds.Knowledge = map[string]DeploymentKnowledge{
		"docs": {Image: "qdrant:latest", Endpoints: map[string]Endpoint{"http": {Port: 6333}}, Persistent: true},
	}
	ds.Integrations = map[string]DeploymentIntegration{
		"search": {Image: "search:latest", Endpoints: map[string]Endpoint{"http": {Port: 3000}}},
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
	if resolved.Integrations["search"].Replicas != 1 {
		t.Errorf("integration replicas: expected 1, got %d", resolved.Integrations["search"].Replicas)
	}
}

func TestValidateAndResolve_BoundKnowledgeSkipsDefaults(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Knowledge = map[string]DeploymentKnowledge{
		"docs": {Binding: "arn:knowledge-store:acct123:my-pg-store"},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}

	k := result.Spec.Knowledge["docs"]
	if k.Replicas != 0 {
		t.Errorf("bound knowledge replicas: expected 0 (untouched), got %d", k.Replicas)
	}
	if k.Update.Strategy != "" {
		t.Errorf("bound knowledge update strategy: expected empty (untouched), got %s", k.Update.Strategy)
	}
}

func TestValidateAndResolve_BoundKnowledgeSkipsValidation(t *testing.T) {
	ds := baseDeploymentSpec()
	// Bound entry has no image or endpoints — should pass validation
	ds.Knowledge = map[string]DeploymentKnowledge{
		"docs": {Binding: "arn:knowledge-store:acct123:my-pg-store"},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("bound knowledge should not produce validation errors, got %v", result.Errors)
	}
	if !result.Spec.Knowledge["docs"].IsBound() {
		t.Fatal("expected resolved knowledge to remain bound")
	}
	if result.Spec.Knowledge["docs"].Binding != "arn:knowledge-store:acct123:my-pg-store" {
		t.Error("expected binding ARN to be preserved")
	}
}

func TestValidateAndResolve_MixedBoundAndInlineKnowledge(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Knowledge = map[string]DeploymentKnowledge{
		"managed": {Binding: "arn:knowledge-store:acct123:pg-store"},
		"local": {
			Image:      "qdrant:latest",
			Endpoints:  map[string]Endpoint{"http": {Port: 6333}},
			Persistent: true,
		},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}

	// Bound entry untouched
	managed := result.Spec.Knowledge["managed"]
	if managed.Replicas != 0 {
		t.Errorf("bound replicas: expected 0, got %d", managed.Replicas)
	}
	if managed.Update.Strategy != "" {
		t.Errorf("bound update strategy: expected empty, got %s", managed.Update.Strategy)
	}

	// Inline entry gets defaults
	local := result.Spec.Knowledge["local"]
	if local.Replicas != 1 {
		t.Errorf("inline replicas: expected 1, got %d", local.Replicas)
	}
	if local.Update.Strategy != "rolling" {
		t.Errorf("inline update strategy: expected rolling, got %s", local.Update.Strategy)
	}
	if local.Storage == nil {
		t.Error("expected default storage for persistent inline knowledge")
	}
}

func TestValidateAndResolve_InlineKnowledgeMissingImage(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Knowledge = map[string]DeploymentKnowledge{
		"docs": {Endpoints: map[string]Endpoint{"http": {Port: 6333}}},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for inline knowledge without image")
	}
}

func TestValidateAndResolve_InlineKnowledgeMissingEndpoints(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Knowledge = map[string]DeploymentKnowledge{
		"docs": {Image: "qdrant:latest"},
	}

	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for inline knowledge without endpoints")
	}
}

func TestValidateAndResolve_SetsDeploymentSpecVersion(t *testing.T) {
	ds := baseDeploymentSpec()
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Spec.Spec != "deployment/v1" {
		t.Errorf("expected spec version deployment/v1 after resolution, got %s", result.Spec.Spec)
	}
}

func TestValidateAndResolve_WebhookIngestionMissingEndpoints(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]DeploymentIngestion{
		"data": {
			Image:   "ingest:latest",
			Trigger: DeploymentTrigger{Type: "webhook"},
		},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for webhook ingestion without endpoints")
	}
}

func TestValidateAndResolve_WebhookIngestionWithEndpoints(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Ingestion = map[string]DeploymentIngestion{
		"data": {
			Image:     "ingest:latest",
			Endpoints: map[string]Endpoint{"http": {Port: 3001}},
			Trigger:   DeploymentTrigger{Type: "webhook"},
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
	ds.Ingestion = map[string]DeploymentIngestion{
		"data": {
			Trigger: DeploymentTrigger{Type: "startup"},
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

func TestValidateAndResolve_MissingModelEndpoints(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Models = map[string]DeploymentModel{
		"llm": {Image: "my-model:latest"},
	}
	result, err := ValidateAndResolve(ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for model without endpoints")
	}
}

func TestValidateAndResolve_MissingIntegrationImage(t *testing.T) {
	ds := baseDeploymentSpec()
	ds.Integrations = map[string]DeploymentIntegration{
		"search": {Endpoints: map[string]Endpoint{"http": {Port: 3000}}},
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
	ds.Interfaces = &DeploymentInterfaces{
		Adapters:  []string{"discord"},
		Image:     "messaging:latest",
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090}},
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
	ds.Interfaces = &DeploymentInterfaces{
		Adapters: []string{"web"},
		Image:    "messaging:latest",
		Endpoints: map[string]Endpoint{
			"grpc": {Port: 9090},
			"http": {Port: 8080, Expose: &EndpointExpose{Enabled: true}},
		},
		Environment: map[string]string{
			"MY_AGENT_URL": "${models.missing.http.url}",
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
