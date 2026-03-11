package spec

import (
	"testing"
)

// baseTemplate returns a minimal deployment-template/v1 spec.
func baseTemplate() *AstroDeploymentSpec {
	return &AstroDeploymentSpec{
		Spec:   "deployment-template/v1",
		Source: DeploymentSource{Name: "my-agent", Build: "b1", Account: "acme", Registry: "reg.io"},
		Agent: DeploymentAgent{
			Image:     "reg.io/my-agent:b1",
			Endpoints: map[string]Endpoint{"http": {Port: 8080}},
		},
	}
}

func TestEnforceEditable_NoChanges(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestEnforceEditable_SourceNameChanged(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	subm.Source.Name = "other-agent"
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for source.name change")
	}
}

func TestEnforceEditable_AgentImageChanged(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	subm.Agent.Image = "evil-image:latest"
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for agent.image change")
	}
}

func TestEnforceEditable_AgentEndpointPortChanged(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	subm.Agent.Endpoints = map[string]Endpoint{"http": {Port: 9999}}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for agent endpoint port change")
	}
}

func TestEnforceEditable_AgentEndpointExposeAllowed(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	// Expose is editable — changing it should be allowed
	subm.Agent.Endpoints = map[string]Endpoint{
		"http": {Port: 8080, Expose: &EndpointExpose{Enabled: true, Domain: "myagent.example.com"}},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("expose change should be allowed: %v", errs)
	}
}

func TestEnforceEditable_ModelImageChanged(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Models = map[string]DeploymentModel{
		"llm": {Image: "ollama:latest", Endpoints: map[string]Endpoint{"http": {Port: 11434}}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Models["llm"] = DeploymentModel{
		Image:     "evil-model:latest",
		Endpoints: map[string]Endpoint{"http": {Port: 11434}},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for model image change")
	}
}

func TestEnforceEditable_ModelReplicasAllowed(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Models = map[string]DeploymentModel{
		"llm": {Image: "ollama:latest", Endpoints: map[string]Endpoint{"http": {Port: 11434}}, Replicas: 1},
	}
	subm := CloneDeploymentSpec(tmpl)
	m := subm.Models["llm"]
	m.Replicas = 2 // replicas is editable
	subm.Models["llm"] = m
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("replicas change should be allowed: %v", errs)
	}
}

func TestEnforceEditable_AddingModelNotAllowed(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	subm.Models = map[string]DeploymentModel{
		"injected": {Image: "evil:latest", Endpoints: map[string]Endpoint{"http": {Port: 8080}}},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for adding model not in template")
	}
}

func TestEnforceEditable_RemovingModelNotAllowed(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Models = map[string]DeploymentModel{
		"llm": {Image: "ollama:latest", Endpoints: map[string]Endpoint{"http": {Port: 11434}}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Models = nil // remove model
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for removing model from template")
	}
}

func TestEnforceEditable_IngestionTriggerTypeChanged(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Ingestion = map[string]DeploymentIngestion{
		"sync": {Image: "ingest:latest", Trigger: DeploymentTrigger{Type: "schedule"}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Ingestion["sync"] = DeploymentIngestion{
		Image:   "ingest:latest",
		Trigger: DeploymentTrigger{Type: "startup"},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for ingestion trigger.type change")
	}
}

func TestEnforceEditable_IngestionScheduleAllowed(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Ingestion = map[string]DeploymentIngestion{
		"sync": {Image: "ingest:latest", Trigger: DeploymentTrigger{Type: "schedule", Schedule: ""}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Ingestion["sync"] = DeploymentIngestion{
		Image:   "ingest:latest",
		Trigger: DeploymentTrigger{Type: "schedule", Schedule: "0 * * * *"},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("schedule fill-in should be allowed: %v", errs)
	}
}

func TestEnforceEditable_VariableSecretChanged(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Variables = map[string]Variable{
		"API_KEY": {Secret: true, Targets: []string{"agent"}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Variables["API_KEY"] = Variable{Secret: false, Targets: []string{"agent"}, Value: "val"}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for variable.secret change")
	}
}

func TestEnforceEditable_VariableValueAllowed(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Variables = map[string]Variable{
		"API_KEY": {Secret: true, Optional: false, Targets: []string{"agent"}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Variables["API_KEY"] = Variable{
		Secret: true, Optional: false, Targets: []string{"agent"}, Value: "sk-real-key",
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("variable value fill-in should be allowed: %v", errs)
	}
}

func TestEnforceEditable_AddingVariableNotAllowed(t *testing.T) {
	tmpl := baseTemplate()
	subm := CloneDeploymentSpec(tmpl)
	subm.Variables = map[string]Variable{
		"INJECTED": {Secret: true, Targets: []string{"agent"}, Value: "val"},
	}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for adding variable not in template")
	}
}

func TestEnforceEditable_InterfacesImageChanged(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Interfaces = &DeploymentInterfaces{
		Image:     "astro-messaging:v1",
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090, Protocol: "grpc"}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Interfaces.Image = "evil-messaging:latest"
	errs := EnforceEditable(tmpl, subm)
	if len(errs) == 0 {
		t.Fatal("expected error for interfaces.image change")
	}
}

func TestEnforceEditable_InterfacesAdaptersAllowed(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Interfaces = &DeploymentInterfaces{
		Image:     "astro-messaging:v1",
		Adapters:  []string{},
		Endpoints: map[string]Endpoint{"grpc": {Port: 9090, Protocol: "grpc"}},
	}
	subm := CloneDeploymentSpec(tmpl)
	subm.Interfaces.Adapters = []string{"slack", "web"}
	errs := EnforceEditable(tmpl, subm)
	if len(errs) > 0 {
		t.Errorf("adapters change should be allowed: %v", errs)
	}
}

func TestEnforceEditable_MultipleViolations(t *testing.T) {
	tmpl := baseTemplate()
	tmpl.Models = map[string]DeploymentModel{
		"llm": {Image: "ollama:latest", Endpoints: map[string]Endpoint{"http": {Port: 11434}}},
	}
	subm := CloneDeploymentSpec(tmpl)
	// Multiple violations at once
	subm.Source.Name = "different-agent"
	subm.Agent.Image = "evil:latest"
	m := subm.Models["llm"]
	m.Image = "evil-model:latest"
	subm.Models["llm"] = m
	errs := EnforceEditable(tmpl, subm)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestCloneDeploymentSpec(t *testing.T) {
	original := baseTemplate()
	original.Models = map[string]DeploymentModel{
		"llm": {Image: "ollama:latest", Endpoints: map[string]Endpoint{"http": {Port: 11434}}},
	}
	original.Variables = map[string]Variable{
		"KEY": {Secret: true, Targets: []string{"agent"}},
	}

	clone := CloneDeploymentSpec(original)

	// Modifying clone must not affect original
	clone.Source.Name = "changed"
	clone.Models["llm"] = DeploymentModel{Image: "changed:latest"}
	clone.Variables["KEY"] = Variable{Secret: false}

	if original.Source.Name != "my-agent" {
		t.Error("original source.name was mutated by clone modification")
	}
	if original.Models["llm"].Image != "ollama:latest" {
		t.Error("original model image was mutated by clone modification")
	}
	if !original.Variables["KEY"].Secret {
		t.Error("original variable secret was mutated by clone modification")
	}
}
