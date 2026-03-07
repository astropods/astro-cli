package cmd

import (
	"sort"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// collectPlainProviderVars reproduces the explain.go plainProviderVars logic.
func collectPlainProviderVars(s *spec.AstroSpec) []string {
	var out []string
	for provName, cp := range referencedCustomProviders(s) {
		prefix := spec.SanitizeEnvName(provName)
		for _, v := range cp.Variables {
			if !v.Secret {
				out = append(out, prefix+"_"+v.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// collectAutoEnvNonSecretKeys reproduces the explain.go autoEnv logic for non-secret provider vars.
func collectAutoEnvNonSecretKeys(s *spec.AstroSpec) map[string]string {
	autoEnv := make(map[string]string)
	for provName, cp := range referencedCustomProviders(s) {
		prefix := spec.SanitizeEnvName(provName)
		for _, v := range cp.Variables {
			if v.Secret {
				continue
			}
			autoEnv[prefix+"_"+v.Name] = provName
		}
	}
	return autoEnv
}

// ─── Regression: plainProviderVars must be prefixed (explain "from providers" list) ──

func TestPlainProviderVarsPrefixed(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Secret: true},
					{Name: "ACCOUNT_ID", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	vars := collectPlainProviderVars(s)
	if len(vars) != 1 || vars[0] != "CLOUDFLARE_ACCOUNT_ID" {
		t.Errorf("got %v, want [CLOUDFLARE_ACCOUNT_ID]", vars)
	}
}

func TestPlainProviderVars_BareNameNeverAppears(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "ACCOUNT_ID", Secret: false},
					{Name: "ZONE_ID", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	vars := collectPlainProviderVars(s)
	for _, v := range vars {
		if v == "ACCOUNT_ID" || v == "ZONE_ID" {
			t.Errorf("bare name %q must not appear — should be prefixed with CLOUDFLARE_", v)
		}
	}
}

func TestPlainProviderVars_MultipleProviders(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "API_KEY", Secret: true},
					{Name: "ACCOUNT_ID", Secret: false},
				},
			},
			"github": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "ORG", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
			"github":     {Provider: "github"},
		},
	}

	vars := collectPlainProviderVars(s)
	expected := []string{"CLOUDFLARE_ACCOUNT_ID", "GITHUB_ORG"}
	if len(vars) != len(expected) {
		t.Fatalf("got %v, want %v", vars, expected)
	}
	for i, want := range expected {
		if vars[i] != want {
			t.Errorf("vars[%d] = %q, want %q", i, vars[i], want)
		}
	}
}

func TestPlainProviderVars_AllSecretProducesNone(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"vault": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "ADDR", Secret: true},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"vault": {Provider: "vault"},
		},
	}

	vars := collectPlainProviderVars(s)
	if len(vars) != 0 {
		t.Errorf("expected no plain vars for all-secret provider, got %v", vars)
	}
}

func TestPlainProviderVars_AllNonSecret(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myapi": {
				Variables: []spec.Input{
					{Name: "REGION", Secret: false},
					{Name: "ENDPOINT", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"myapi": {Provider: "myapi"},
		},
	}

	vars := collectPlainProviderVars(s)
	expected := []string{"MYAPI_ENDPOINT", "MYAPI_REGION"}
	if len(vars) != len(expected) {
		t.Fatalf("got %v, want %v", vars, expected)
	}
	for i, want := range expected {
		if vars[i] != want {
			t.Errorf("vars[%d] = %q, want %q", i, vars[i], want)
		}
	}
}

func TestPlainProviderVars_HyphenatedProviderName(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"my-api": {
				Variables: []spec.Input{
					{Name: "HOST", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"mytool": {Provider: "my-api"},
		},
	}

	vars := collectPlainProviderVars(s)
	if len(vars) != 1 || vars[0] != "MY_API_HOST" {
		t.Errorf("got %v, want [MY_API_HOST] (hyphens replaced with underscores)", vars)
	}
}

func TestPlainProviderVars_ReferencedViaModel(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"custom-llm": {
				Variables: []spec.Input{
					{Name: "ENDPOINT", Secret: false},
				},
			},
		},
		Models: map[string]spec.Model{
			"main": {Provider: "custom-llm"},
		},
	}

	vars := collectPlainProviderVars(s)
	if len(vars) != 1 || vars[0] != "CUSTOM_LLM_ENDPOINT" {
		t.Errorf("got %v, want [CUSTOM_LLM_ENDPOINT]", vars)
	}
}

func TestPlainProviderVars_ReferencedViaKnowledge(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"custom-db": {
				Variables: []spec.Input{
					{Name: "HOST", Secret: false},
					{Name: "PORT", Secret: false},
				},
			},
		},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "custom-db"},
		},
	}

	vars := collectPlainProviderVars(s)
	expected := []string{"CUSTOM_DB_HOST", "CUSTOM_DB_PORT"}
	if len(vars) != len(expected) {
		t.Fatalf("got %v, want %v", vars, expected)
	}
	for i, want := range expected {
		if vars[i] != want {
			t.Errorf("vars[%d] = %q, want %q", i, vars[i], want)
		}
	}
}

// ─── Regression: autoEnv collision detection must use prefixed keys ──────────

func TestAutoEnvNonSecretProviderVarsPrefixed(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Secret: true},
					{Name: "ACCOUNT_ID", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	autoEnv := collectAutoEnvNonSecretKeys(s)
	if _, ok := autoEnv["CLOUDFLARE_ACCOUNT_ID"]; !ok {
		t.Error("expected CLOUDFLARE_ACCOUNT_ID in autoEnv")
	}
	if _, ok := autoEnv["ACCOUNT_ID"]; ok {
		t.Error("bare ACCOUNT_ID must not be in autoEnv")
	}
}

func TestAutoEnv_MultipleNonSecretVarsSameProvider(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "ACCOUNT_ID", Secret: false},
					{Name: "ZONE_ID", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	autoEnv := collectAutoEnvNonSecretKeys(s)
	for _, want := range []string{"CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_ZONE_ID"} {
		if _, ok := autoEnv[want]; !ok {
			t.Errorf("expected %s in autoEnv", want)
		}
	}
	for _, bare := range []string{"ACCOUNT_ID", "ZONE_ID"} {
		if _, ok := autoEnv[bare]; ok {
			t.Errorf("bare %s must not be in autoEnv", bare)
		}
	}
}

func TestAutoEnv_SecretsExcludedFromNonSecretPath(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myapi": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "REGION", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"myapi": {Provider: "myapi"},
		},
	}

	autoEnv := collectAutoEnvNonSecretKeys(s)
	if _, ok := autoEnv["MYAPI_TOKEN"]; ok {
		t.Error("secret MYAPI_TOKEN must not appear in non-secret autoEnv path")
	}
	if _, ok := autoEnv["MYAPI_REGION"]; !ok {
		t.Error("expected MYAPI_REGION in autoEnv")
	}
}

// ─── referencedCustomProviders ──────────────────────────────────────────────

func TestReferencedCustomProvidersSkipsUnreferenced(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"used":   {Variables: []spec.Input{{Name: "X", Secret: false}}},
			"unused": {Variables: []spec.Input{{Name: "Y", Secret: false}}},
		},
		Tools: map[string]spec.Tool{
			"mytool": {Provider: "used"},
		},
	}

	refs := referencedCustomProviders(s)
	if _, ok := refs["used"]; !ok {
		t.Error("expected 'used' provider in referencedCustomProviders")
	}
	if _, ok := refs["unused"]; ok {
		t.Error("'unused' provider should not appear in referencedCustomProviders")
	}
}

func TestReferencedCustomProviders_AllSections(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"model-prov":     {},
			"knowledge-prov": {},
			"tool-prov":      {},
		},
		Models:    map[string]spec.Model{"m": {Provider: "model-prov"}},
		Knowledge: map[string]spec.Knowledge{"k": {Provider: "knowledge-prov"}},
		Tools:     map[string]spec.Tool{"t": {Provider: "tool-prov"}},
	}

	refs := referencedCustomProviders(s)
	for _, name := range []string{"model-prov", "knowledge-prov", "tool-prov"} {
		if _, ok := refs[name]; !ok {
			t.Errorf("expected %q in referencedCustomProviders", name)
		}
	}
}

func TestReferencedCustomProviders_NoProviderFieldSkipped(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myprov": {},
		},
		// Tool without provider field — container mode, should not reference myprov
		Tools: map[string]spec.Tool{
			"standalone": {},
		},
	}

	refs := referencedCustomProviders(s)
	if len(refs) != 0 {
		t.Errorf("expected no referenced providers, got %v", refs)
	}
}

func TestReferencedCustomProviders_SameProviderMultipleSections(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"shared": {Variables: []spec.Input{{Name: "KEY", Secret: false}}},
		},
		Models: map[string]spec.Model{"m": {Provider: "shared"}},
		Tools:  map[string]spec.Tool{"t": {Provider: "shared"}},
	}

	refs := referencedCustomProviders(s)
	if len(refs) != 1 {
		t.Fatalf("expected 1 referenced provider, got %d", len(refs))
	}
	if _, ok := refs["shared"]; !ok {
		t.Error("expected 'shared' in referencedCustomProviders")
	}

	// Verify plain vars are deduplicated (provider appears once → one set of vars)
	vars := collectPlainProviderVars(s)
	if len(vars) != 1 || vars[0] != "SHARED_KEY" {
		t.Errorf("got %v, want [SHARED_KEY]", vars)
	}
}
