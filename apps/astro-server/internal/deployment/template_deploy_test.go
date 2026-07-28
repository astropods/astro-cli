package deployment

import (
	"testing"

	spec "github.com/astropods/astro-spec"
)

// fillAndDeploy simulates the deploy flow now that EnforceEditable is gone:
//  1. Generate the deployment template
//  2. Apply user edits via the fill function (variables, adapters, …)
//  3. ApplyAdapterShaping so variable optionality matches the adapter set
//  4. Validate and resolve the spec
//
// The second return slot is retained as []string for ergonomic test diffs but
// is always nil — tamper detection lives in the deploy handler via signature
// verification, not at the spec layer.
func fillAndDeploy(t *testing.T, input TemplateInput, fill func(*spec.AstroDeploymentSpec)) (*ResolveResult, []string) {
	t.Helper()
	tmpl := mustGenerate(t, input)

	// Deep-copy via serialise/parse so fill edits a clean copy
	raw, err := spec.SerializeDeploymentSpec(tmpl)
	if err != nil {
		t.Fatalf("serialize template: %v", err)
	}
	submitted, err := spec.ParseDeploymentSpec(raw)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	fill(submitted)

	if submitted.Interfaces != nil {
		ApplyAdapterShaping(submitted, submitted.Interfaces.Adapters)
	}

	result, err := ValidateAndResolve(submitted)
	if err != nil {
		t.Fatalf("ValidateAndResolve: %v", err)
	}
	return result, nil
}

// setVarValue updates only the value of an existing variable, preserving all
// other fields (optional, secret, targets, description) set by the template.
func setVarValue(ds *spec.AstroDeploymentSpec, key, value string) {
	if ds.Variables == nil {
		ds.Variables = make(map[string]spec.Variable)
	}
	v := ds.Variables[key]
	v.Value = value
	ds.Variables[key] = v
}

// ===== Minimal deploy =====

func TestTemplateDeploy_MinimalSpec(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors: %v", result.Errors)
	}
	if result.Spec.Spec != "deployment/v1" {
		t.Errorf("resolved spec version: expected deployment/v1, got %s", result.Spec.Spec)
	}
}

// ===== Slack adapter =====

func TestTemplateDeploy_SlackAdapter_WithTokens(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
		ds.Interfaces.Adapters = []string{"slack"}
		// Update only the value — preserve optional/secret/targets from the template
		setVarValue(ds, "SLACK_BOT_TOKEN", "xoxb-test-token")
		setVarValue(ds, "SLACK_APP_TOKEN", "xapp-test-token")
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors: %v", result.Errors)
	}
}

func TestTemplateDeploy_SlackAdapter_MissingBotToken(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
		ds.Interfaces.Adapters = []string{"slack"}
		// Only fill app token, leave bot token empty
		setVarValue(ds, "SLACK_APP_TOKEN", "xapp-test-token")
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing SLACK_BOT_TOKEN")
	}
	found := false
	for _, e := range result.Errors {
		if e == "variables.SLACK_BOT_TOKEN.value: required for slack adapter" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SLACK_BOT_TOKEN error, got %v", result.Errors)
	}
}

func TestTemplateDeploy_SlackAdapter_MissingAppToken(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
		ds.Interfaces.Adapters = []string{"slack"}
		// Only fill bot token, leave app token empty
		setVarValue(ds, "SLACK_BOT_TOKEN", "xoxb-test-token")
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing SLACK_APP_TOKEN")
	}
	found := false
	for _, e := range result.Errors {
		if e == "variables.SLACK_APP_TOKEN.value: required for slack adapter" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SLACK_APP_TOKEN error, got %v", result.Errors)
	}
}

func TestTemplateDeploy_SlackAdapter_MissingBothTokens(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
		ds.Interfaces.Adapters = []string{"slack"}
		// Variables are in the template but left empty — user forgot to fill them in
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing slack tokens")
	}
}

// ===== Web adapter =====

func TestTemplateDeploy_WebAdapter(t *testing.T) {
	result, editErrs := fillAndDeploy(t, baseInput(), func(ds *spec.AstroDeploymentSpec) {
		ds.Interfaces.Adapters = []string{"web"}
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors: %v", result.Errors)
	}
}

// ===== Cloud provider credentials =====

func TestTemplateDeploy_AnthropicModel_WithCredential(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {Provider: "anthropic"},
	}

	result, editErrs := fillAndDeploy(t, input, func(ds *spec.AstroDeploymentSpec) {
		setVarValue(ds, "ANTHROPIC_API_KEY", "sk-ant-test")
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected validation errors: %v", result.Errors)
	}
}

func TestTemplateDeploy_AnthropicModel_MissingCredential(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {Provider: "anthropic"},
	}

	result, editErrs := fillAndDeploy(t, input, func(ds *spec.AstroDeploymentSpec) {
		// ANTHROPIC_API_KEY left empty — user forgot to fill it in
	})
	if len(editErrs) > 0 {
		t.Fatalf("EnforceEditable errors: %v", editErrs)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for missing ANTHROPIC_API_KEY")
	}
}
