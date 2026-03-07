package cmd

import (
	"sort"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// collectConfigureVars reproduces the credential + non-secret provider var logic from runConfigure.
func collectConfigureVars(astroSpec *spec.AstroSpec) []varEntry {
	var credVars []varEntry

	credKeys := spec.AllCredentialKeys(astroSpec)
	sortedCredKeys := make([]string, 0, len(credKeys))
	for k := range credKeys {
		sortedCredKeys = append(sortedCredKeys, k)
	}
	sort.Strings(sortedCredKeys)
	for _, k := range sortedCredKeys {
		meta := credKeys[k]
		credVars = append(credVars, varEntry{
			key:         k,
			description: meta.Description,
			secret:      true,
		})
	}

	var providerPlainVars []varEntry
	for provName, cp := range referencedCustomProviders(astroSpec) {
		prefix := spec.SanitizeEnvName(provName)
		for _, v := range cp.Variables {
			if v.Secret {
				continue
			}
			providerPlainVars = append(providerPlainVars, varEntry{
				key:         prefix + "_" + v.Name,
				description: v.Description,
				secret:      false,
			})
		}
	}
	sort.Slice(providerPlainVars, func(i, j int) bool { return providerPlainVars[i].key < providerPlainVars[j].key })
	credVars = append(credVars, providerPlainVars...)
	return credVars
}

func configureVarKeys(entries []varEntry) map[string]varEntry {
	m := make(map[string]varEntry, len(entries))
	for _, e := range entries {
		m[e.key] = e
	}
	return m
}

// ─── Regression: non-secret provider vars must appear prefixed in configure ──

func TestConfigureCollectsNonSecretProviderVars(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Secret: true, Description: "Cloudflare Workers AI API key"},
					{Name: "ACCOUNT_ID", Secret: false, Description: "Cloudflare account ID"},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	if _, ok := keys["CLOUDFLARE_AI_API_KEY"]; !ok {
		t.Error("expected CLOUDFLARE_AI_API_KEY (secret)")
	}
	if _, ok := keys["CLOUDFLARE_ACCOUNT_ID"]; !ok {
		t.Error("expected CLOUDFLARE_ACCOUNT_ID (non-secret, prefixed)")
	}
	if _, ok := keys["ACCOUNT_ID"]; ok {
		t.Error("bare ACCOUNT_ID must not appear — should be CLOUDFLARE_ACCOUNT_ID")
	}
}

func TestConfigureNonSecretProviderVarNotSecret(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"myapi": {
				Variables: []spec.Input{
					{Name: "TOKEN", Secret: true},
					{Name: "REGION", Secret: false, Description: "API region"},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"myapi": {Provider: "myapi"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	v, ok := keys["MYAPI_REGION"]
	if !ok {
		t.Fatal("MYAPI_REGION not found in configure vars")
	}
	if v.secret {
		t.Error("MYAPI_REGION should not be marked as secret")
	}
	if v.description != "API region" {
		t.Errorf("MYAPI_REGION description = %q, want %q", v.description, "API region")
	}
}

func TestConfigureNonSecretVars_NoDoubleCounting(t *testing.T) {
	// Secret vars come from AllCredentialKeys; non-secret vars come from our new path.
	// Ensure secrets don't appear twice.
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

	vars := collectConfigureVars(s)
	count := make(map[string]int)
	for _, v := range vars {
		count[v.key]++
	}
	for key, n := range count {
		if n > 1 {
			t.Errorf("key %q appears %d times — should appear exactly once", key, n)
		}
	}
}

func TestConfigureNonSecretVars_AllSecretsProducesNoPlainVars(t *testing.T) {
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

	vars := collectConfigureVars(s)
	for _, v := range vars {
		if !v.secret {
			t.Errorf("unexpected non-secret var %q — all provider vars are secret", v.key)
		}
	}
}

func TestConfigureNonSecretVars_HyphenatedProviderName(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"my-api": {
				Variables: []spec.Input{
					{Name: "HOST", Secret: false},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"t": {Provider: "my-api"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	if _, ok := keys["MY_API_HOST"]; !ok {
		t.Error("expected MY_API_HOST (hyphen sanitized to underscore)")
	}
	if _, ok := keys["MY-API_HOST"]; ok {
		t.Error("unsanitized MY-API_HOST must not appear")
	}
}

func TestConfigureNonSecretVars_MultipleProviders(t *testing.T) {
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
					{Name: "ORG", Secret: false, Description: "GitHub org"},
				},
			},
		},
		Tools: map[string]spec.Tool{
			"cloudflare": {Provider: "cloudflare"},
			"github":     {Provider: "github"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))

	for _, want := range []string{"CLOUDFLARE_API_KEY", "CLOUDFLARE_ACCOUNT_ID", "GITHUB_TOKEN", "GITHUB_ORG"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("expected %s in configure vars", want)
		}
	}
	for _, bare := range []string{"ACCOUNT_ID", "ORG", "API_KEY", "TOKEN"} {
		if _, ok := keys[bare]; ok {
			t.Errorf("bare %s must not appear in configure vars", bare)
		}
	}
}

func TestConfigureNonSecretVars_ReferencedViaModel(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"custom-llm": {
				Variables: []spec.Input{
					{Name: "ENDPOINT", Secret: false, Description: "LLM endpoint"},
				},
			},
		},
		Models: map[string]spec.Model{
			"main": {Provider: "custom-llm"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	v, ok := keys["CUSTOM_LLM_ENDPOINT"]
	if !ok {
		t.Fatal("expected CUSTOM_LLM_ENDPOINT for model-referenced provider")
	}
	if v.description != "LLM endpoint" {
		t.Errorf("description = %q, want %q", v.description, "LLM endpoint")
	}
}

func TestConfigureNonSecretVars_ReferencedViaKnowledge(t *testing.T) {
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

	keys := configureVarKeys(collectConfigureVars(s))
	for _, want := range []string{"CUSTOM_DB_HOST", "CUSTOM_DB_PORT"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("expected %s in configure vars", want)
		}
	}
}

func TestConfigureNonSecretVars_UnreferencedProviderExcluded(t *testing.T) {
	s := &spec.AstroSpec{
		Providers: map[string]spec.CustomProvider{
			"used":   {Variables: []spec.Input{{Name: "X", Secret: false}}},
			"unused": {Variables: []spec.Input{{Name: "Y", Secret: false}}},
		},
		Tools: map[string]spec.Tool{
			"t": {Provider: "used"},
		},
	}

	keys := configureVarKeys(collectConfigureVars(s))
	if _, ok := keys["USED_X"]; !ok {
		t.Error("expected USED_X")
	}
	if _, ok := keys["UNUSED_Y"]; ok {
		t.Error("UNUSED_Y must not appear — provider is unreferenced")
	}
}
