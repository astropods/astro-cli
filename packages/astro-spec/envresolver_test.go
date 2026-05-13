package spec

import (
	"testing"
)

// ─── §8.5 SanitizeEnvName ────────────────────────────────────────────────────

func TestSanitizeEnvName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"llm", "LLM"},
		{"my_model", "MY_MODEL"},
		{"my.store", "MY_STORE"},
		{"local_llm", "LOCAL_LLM"},
		{"my-service", "MY_SERVICE"},
		{"docs_sync", "DOCS_SYNC"},
		{"a.b.c", "A_B_C"},
		{"A_B_C", "A_B_C"}, // already uppercased input still works
		{"", ""},           // empty
		{"_leading", "LEADING"},
		{"trailing_", "TRAILING"},
		{"double--hyphens", "DOUBLE_HYPHENS"},
		{"hello world", "HELLOWORLD"}, // space removed
		{"my__model", "MY_MODEL"},     // consecutive underscores → single underscore
		// Regression: underscores in raw input must be preserved, not stripped.
		// Previously the regex [^a-z0-9] would remove underscores; the fix
		// changed it to [^a-z0-9_] so underscores survive sanitization.
		{"pre_existing_underscore", "PRE_EXISTING_UNDERSCORE"},
		{"a_b.c-d", "A_B_C_D"}, // mix of all three separators preserved / normalised
		// Regression: dots and hyphens must both be replaced with _ in a single
		// pass. Previously two separate ReplaceAll calls handled them; collapsing
		// them must not change the output.
		{"foo.bar-baz", "FOO_BAR_BAZ"},
		{"foo-bar.baz", "FOO_BAR_BAZ"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := SanitizeEnvName(tt.in); got != tt.want {
				t.Errorf("SanitizeEnvName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ─── §8.1 Cloud credential keys ─────────────────────────────────────────────

func TestCloudCredentialKeys_SingleModelProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"primary": {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_SingleKnowledgeProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"vectors": {Provider: "pinecone"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "PINECONE_API_KEY", "knowledge", false)
}

func TestCloudCredentialKeys_SingleToolProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Integrations: map[string]Integration{
			"github": {Provider: "github"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "GITHUB_TOKEN", "integration", false)
}

func TestCloudCredentialKeys_AllCloudProviders(t *testing.T) {
	// Verify every cloud provider in the registry produces the correct credential key.
	tests := []struct {
		name    string
		spec    *AstroSpec
		wantKey string
		cat     string
	}{
		{
			name: "openai",
			spec: &AstroSpec{Name: "a", Agent: Container{Image: "a:1"},
				Models: map[string]Model{"m": {Provider: "openai"}}},
			wantKey: "OPENAI_API_KEY", cat: "model",
		},
		{
			name: "google",
			spec: &AstroSpec{Name: "a", Agent: Container{Image: "a:1"},
				Models: map[string]Model{"m": {Provider: "google"}}},
			wantKey: "GOOGLE_API_KEY", cat: "model",
		},
		{
			name: "gemini",
			spec: &AstroSpec{Name: "a", Agent: Container{Image: "a:1"},
				Models: map[string]Model{"m": {Provider: "gemini"}}},
			wantKey: "GEMINI_API_KEY", cat: "model",
		},
		{
			name: "cohere",
			spec: &AstroSpec{Name: "a", Agent: Container{Image: "a:1"},
				Models: map[string]Model{"m": {Provider: "cohere"}}},
			wantKey: "COHERE_API_KEY", cat: "model",
		},
		{
			name: "gitlab",
			spec: &AstroSpec{Name: "a", Agent: Container{Image: "a:1"},
				Integrations: map[string]Integration{"t": {Provider: "gitlab"}}},
			wantKey: "GITLAB_TOKEN", cat: "integration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := CloudCredentialKeys(tt.spec)
			assertCredKey(t, keys, tt.wantKey, tt.cat, false)
			if len(keys) != 1 {
				t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
			}
		})
	}
}

func TestCloudCredentialKeys_DuplicateModelProviders_NameMatchesPrimary(t *testing.T) {
	// Entry named "anthropic" using provider "anthropic" is primary.
	// Entry named "sonnet" using provider "anthropic" gets qualified key.
	// "anthropic" (name == provider) → bare key only, no ANTHROPIC_ANTHROPIC_API_KEY.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"anthropic": {Provider: "anthropic"},
			"sonnet":    {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)

	// Bare key for primary.
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	// Qualified key for non-primary.
	assertCredKey(t, keys, "ANTHROPIC_SONNET_API_KEY", "model", false)
	// Redundant double-name form must NOT appear.
	if _, ok := keys["ANTHROPIC_ANTHROPIC_API_KEY"]; ok {
		t.Error("should not produce redundant ANTHROPIC_ANTHROPIC_API_KEY")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_DuplicateModelProviders_NoNameMatch_NoBareKey(t *testing.T) {
	// Regression: when no entry name matches the provider, bareIdx must remain -1
	// so no bare key is emitted. Previously bareIdx defaulted to 0, causing the
	// first-alphabetically entry to incorrectly receive the bare key.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"beta":  {Provider: "anthropic"},
			"alpha": {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)

	// Both entries get only qualified keys; no bare ANTHROPIC_API_KEY.
	assertCredKey(t, keys, "ANTHROPIC_ALPHA_API_KEY", "model", false)
	assertCredKey(t, keys, "ANTHROPIC_BETA_API_KEY", "model", false)
	if _, ok := keys["ANTHROPIC_API_KEY"]; ok {
		t.Error("bare ANTHROPIC_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_MultipleProviders(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "anthropic"},
		},
		Integrations: map[string]Integration{
			"gh": {Provider: "github"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	assertCredKey(t, keys, "GITHUB_TOKEN", "integration", false)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_SkipsCustomProviders(t *testing.T) {
	// A tool referencing a custom provider must not appear in cloud credential keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
	}
	keys := CloudCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("expected 0 cloud keys (only custom provider), got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_OptionalCredential(t *testing.T) {
	// gemini has optional=false by default; validate the Optional field is carried through.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"ai": {Provider: "openai"},
		},
	}
	keys := CloudCredentialKeys(s)
	m, ok := keys["OPENAI_API_KEY"]
	if !ok {
		t.Fatal("OPENAI_API_KEY not found")
	}
	if m.Optional {
		t.Error("expected Optional=false for OPENAI_API_KEY")
	}
}

// ─── Custom provider credential keys ────────────────────────────────────────

func TestCustomProviderCredentialKeys_SecretVariables(t *testing.T) {
	// Custom provider variables follow §8.1: {UPPER(provider)}_{varName}.
	// Variable name is the suffix; provider name is the prefix.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true, Description: "Jira key"},
				{Name: "BASE_URL", Datatype: "string", Secret: false}, // non-secret: not included
				{Name: "TOKEN", Datatype: "string", Secret: true, Optional: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_JIRA_API_KEY", "provider", false)
	assertCredKey(t, keys, "MY_JIRA_TOKEN", "provider", true)
	if _, ok := keys["MY_JIRA_BASE_URL"]; ok {
		t.Error("MY_JIRA_BASE_URL (non-secret) must not appear in credential keys")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestCustomProviderCredentialKeys_DuplicateEntries_NoNameMatch_NoBareKey(t *testing.T) {
	// Regression: when no entry name matches the provider name, bareIdx must remain -1
	// so no bare key is emitted. Previously bareIdx defaulted to 0, causing the
	// first-alphabetically entry ("jira-dev") to incorrectly receive the bare key.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira-prod": {Provider: "my-jira"},
			"jira-dev":  {Provider: "my-jira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	// Both entries get only qualified keys; no bare MY_JIRA_API_KEY.
	assertCredKey(t, keys, "MY_JIRA_JIRA_DEV_API_KEY", "provider", false)  // qualified
	assertCredKey(t, keys, "MY_JIRA_JIRA_PROD_API_KEY", "provider", false) // qualified
	if _, ok := keys["MY_JIRA_API_KEY"]; ok {
		t.Error("bare MY_JIRA_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_UnreferencedProviderExcluded(t *testing.T) {
	// A provider defined but not referenced by any component produces no keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"unused": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "KEY", Datatype: "string", Secret: true},
			}},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for unreferenced provider, got %d: %v", len(keys), keys)
	}
}

// ─── Custom provider credential keys: exhaustive permutations ────────────────

func TestCustomProviderCredentialKeys_SingleModelEntry(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-llm": {Scope: []string{"models"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Models: map[string]Model{
			"llm": {Provider: "my-llm"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_LLM_API_KEY", "provider", false)
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_SingleKnowledgeEntry(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-vec": {Scope: []string{"knowledge"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Knowledge: map[string]Knowledge{
			"store": {Provider: "my-vec"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_VEC_API_KEY", "provider", false)
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_AllNonSecret(t *testing.T) {
	// No secret variables → no credential keys produced.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "BASE_URL", Datatype: "string", Secret: false},
				{Name: "PROJECT", Datatype: "string", Secret: false},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys (no secret variables), got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_MultipleVariables(t *testing.T) {
	// Multiple secret + non-secret variables: only secret ones appear.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-svc": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
				{Name: "CLIENT_SECRET", Datatype: "string", Secret: true, Optional: true},
				{Name: "BASE_URL", Datatype: "string", Secret: false},
				{Name: "REGION", Datatype: "string", Secret: false},
			}},
		},
		Integrations: map[string]Integration{
			"svc": {Provider: "my-svc"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_SVC_API_KEY", "provider", false)
	assertCredKey(t, keys, "MY_SVC_CLIENT_SECRET", "provider", true)
	if _, ok := keys["MY_SVC_BASE_URL"]; ok {
		t.Error("non-secret MY_SVC_BASE_URL must not appear")
	}
	if _, ok := keys["MY_SVC_REGION"]; ok {
		t.Error("non-secret MY_SVC_REGION must not appear")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_ProviderNameSanitized(t *testing.T) {
	// Provider names with hyphens, underscores, dots all become underscores.
	cases := []struct {
		providerName string
		wantPrefix   string
	}{
		{"my-llm", "MY_LLM"},
		{"my_llm", "MY_LLM"},
		{"my.llm", "MY_LLM"},
		{"my-llm-v2", "MY_LLM_V2"},
	}
	for _, tc := range cases {
		t.Run(tc.providerName, func(t *testing.T) {
			s := &AstroSpec{
				Name:  "agent",
				Agent: Container{Image: "a:1"},
				Providers: map[string]CustomProvider{
					tc.providerName: {Scope: []string{"integrations"}, Variables: []Input{
						{Name: "API_KEY", Datatype: "string", Secret: true},
					}},
				},
				Integrations: map[string]Integration{
					"svc": {Provider: tc.providerName},
				},
			}
			keys := CustomProviderCredentialKeys(s)
			want := tc.wantPrefix + "_API_KEY"
			if _, ok := keys[want]; !ok {
				t.Errorf("expected key %q, got %v", want, keys)
			}
		})
	}
}

func TestCustomProviderCredentialKeys_DuplicateFirstAlphaIsPrimary(t *testing.T) {
	// No entry name matches provider; no bare key emitted, only qualified keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"beta":  {Provider: "my-jira"},
			"alpha": {Provider: "my-jira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_JIRA_ALPHA_API_KEY", "provider", false) // qualified
	assertCredKey(t, keys, "MY_JIRA_BETA_API_KEY", "provider", false)  // qualified
	if _, ok := keys["MY_JIRA_API_KEY"]; ok {
		t.Error("bare MY_JIRA_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_DuplicateEntryNameMatchesProvider(t *testing.T) {
	// Entry named "my-jira" matches provider "my-jira" → it is primary, no redundant
	// qualified key MY_JIRA_MY_JIRA_API_KEY; other entries get qualified keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"my-jira": {Provider: "my-jira"}, // name matches provider
			"staging": {Provider: "my-jira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_JIRA_API_KEY", "provider", false)         // bare (primary=my-jira)
	assertCredKey(t, keys, "MY_JIRA_STAGING_API_KEY", "provider", false) // qualified
	if _, ok := keys["MY_JIRA_MY_JIRA_API_KEY"]; ok {
		t.Error("redundant MY_JIRA_MY_JIRA_API_KEY must not be produced")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_DuplicateEntryNameSanitized(t *testing.T) {
	// Entry names with special chars are sanitized in the qualified key.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira-prod": {Provider: "my-jira"}, // hyphen in entry name
			"jira_dev":  {Provider: "my-jira"}, // underscore in entry name
		},
	}
	keys := CustomProviderCredentialKeys(s)
	// No entry name matches provider → no bare key; both get qualified keys with sanitized names.
	assertCredKey(t, keys, "MY_JIRA_JIRA_PROD_API_KEY", "provider", false) // sanitized
	assertCredKey(t, keys, "MY_JIRA_JIRA_DEV_API_KEY", "provider", false)  // sanitized
	if _, ok := keys["MY_JIRA_API_KEY"]; ok {
		t.Error("bare MY_JIRA_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_ThreeEntriesSameProvider(t *testing.T) {
	// Three entries: "a", "b", "c" — "a" is primary.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"svc": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
				{Name: "SECRET", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"a": {Provider: "svc"},
			"b": {Provider: "svc"},
			"c": {Provider: "svc"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	// No entry name matches "svc" → no bare key; all three entries get qualified keys only.
	assertCredKey(t, keys, "SVC_A_API_KEY", "provider", false)
	assertCredKey(t, keys, "SVC_A_SECRET", "provider", false)
	assertCredKey(t, keys, "SVC_B_API_KEY", "provider", false)
	assertCredKey(t, keys, "SVC_B_SECRET", "provider", false)
	assertCredKey(t, keys, "SVC_C_API_KEY", "provider", false)
	assertCredKey(t, keys, "SVC_C_SECRET", "provider", false)
	if _, ok := keys["SVC_API_KEY"]; ok {
		t.Error("bare SVC_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 6 {
		t.Errorf("expected 6 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_MultipleDistinctProviders(t *testing.T) {
	// Two different custom providers, each referenced once.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
			"my-llm": {Scope: []string{"models"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
		Models: map[string]Model{
			"llm": {Provider: "my-llm"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "MY_JIRA_API_KEY", "provider", false)
	assertCredKey(t, keys, "MY_LLM_API_KEY", "provider", false)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_SameProviderCrossSection(t *testing.T) {
	// Same custom provider referenced in both models and tools (scoped to both).
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-svc": {Scope: []string{"models", "integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Models: map[string]Model{
			"m": {Provider: "my-svc"},
		},
		Integrations: map[string]Integration{
			"t": {Provider: "my-svc"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	// No entry name matches "my-svc" → no bare key; both entries get qualified keys only.
	assertCredKey(t, keys, "MY_SVC_M_API_KEY", "provider", false) // qualified
	assertCredKey(t, keys, "MY_SVC_T_API_KEY", "provider", false) // qualified
	if _, ok := keys["MY_SVC_API_KEY"]; ok {
		t.Error("bare MY_SVC_API_KEY must not be emitted when no entry name matches the provider")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_MixedWithCloudProvider(t *testing.T) {
	// Cloud and custom providers coexist; AllCredentialKeys returns both.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Models: map[string]Model{
			"llm": {Provider: "anthropic"},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
	}
	all := AllCredentialKeys(s)
	assertCredKey(t, all, "ANTHROPIC_API_KEY", "model", false)
	assertCredKey(t, all, "MY_JIRA_API_KEY", "provider", false)
	if len(all) != 2 {
		t.Errorf("expected 2 total keys, got %d: %v", len(all), all)
	}
}

func TestCustomProviderCredentialKeys_OnlyOneEntryHasRedundantNameSkipped(t *testing.T) {
	// Single entry where entry name == provider name → only bare key, no qualified.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"svc": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "TOKEN", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"svc": {Provider: "svc"}, // name == provider
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "SVC_TOKEN", "provider", false)
	if _, ok := keys["SVC_SVC_TOKEN"]; ok {
		t.Error("redundant SVC_SVC_TOKEN must not be produced for single entry")
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_ProviderWithOnlyOptionalSecret(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"svc": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "OPTIONAL_KEY", Datatype: "string", Secret: true, Optional: true},
			}},
		},
		Integrations: map[string]Integration{
			"t": {Provider: "svc"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "SVC_OPTIONAL_KEY", "provider", true) // Optional=true
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCustomProviderCredentialKeys_DescriptionCarriedThrough(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"svc": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true, Description: "My service key"},
			}},
		},
		Integrations: map[string]Integration{
			"t": {Provider: "svc"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	m, ok := keys["SVC_API_KEY"]
	if !ok {
		t.Fatal("SVC_API_KEY missing")
	}
	if m.Description != "My service key" {
		t.Errorf("Description = %q, want %q", m.Description, "My service key")
	}
	if m.Provider != "svc" {
		t.Errorf("Provider = %q, want %q", m.Provider, "svc")
	}
}

// ─── Jira-style integration: end-to-end credential key + input verification ──

func TestCustomProviderCredentialKeys_JiraIntegration(t *testing.T) {
	// Mirrors a real Jira integration spec with all-secret variables.
	// Verifies that credential keys are correctly generated and non-secret
	// variables (if any were present) would be excluded.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true, Description: "Jira API token"},
				{Name: "BASE_URL", Datatype: "string", Secret: true, Description: "Jira instance base URL (e.g. https://your-org.atlassian.net)"},
				{Name: "EMAIL", Datatype: "string", Secret: true, Description: "Atlassian account email for Jira API authentication"},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "jira"},
		},
	}

	// CustomProviderCredentialKeys should produce keys for all three secret variables.
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "JIRA_API_KEY", "provider", false)
	assertCredKey(t, keys, "JIRA_BASE_URL", "provider", false)
	assertCredKey(t, keys, "JIRA_EMAIL", "provider", false)
	if len(keys) != 3 {
		t.Errorf("expected 3 credential keys, got %d: %v", len(keys), keys)
	}

	// Descriptions should be carried through.
	if keys["JIRA_API_KEY"].Description != "Jira API token" {
		t.Errorf("JIRA_API_KEY description = %q", keys["JIRA_API_KEY"].Description)
	}
	if keys["JIRA_EMAIL"].Description != "Atlassian account email for Jira API authentication" {
		t.Errorf("JIRA_EMAIL description = %q", keys["JIRA_EMAIL"].Description)
	}

	// AllCredentialKeys should include the same keys (no cloud providers here).
	all := AllCredentialKeys(s)
	if len(all) != 3 {
		t.Errorf("AllCredentialKeys: expected 3, got %d: %v", len(all), all)
	}
}

func TestCustomProviderCredentialKeys_SkipsBuiltinCloudProviders(t *testing.T) {
	// When a provider (e.g. "anthropic") is both a builtin cloud provider and
	// declared in the spec's custom providers map, CustomProviderCredentialKeys
	// must skip it — CloudCredentialKeys already handles the bare key correctly.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"anthropic": {Provider: "anthropic"},
		},
		Providers: map[string]CustomProvider{
			"anthropic": {Scope: []string{"models"}, Variables: []Input{
				{Name: "ANTHROPIC_API_KEY", Datatype: "string", Secret: true},
			}},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	if _, ok := keys["ANTHROPIC_ANTHROPIC_API_KEY"]; ok {
		t.Error("should not generate ANTHROPIC_ANTHROPIC_API_KEY for builtin cloud provider")
	}
	if _, ok := keys["ANTHROPIC_API_KEY"]; ok {
		t.Error("should not generate ANTHROPIC_API_KEY from custom path (cloud path handles it)")
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 custom keys for builtin cloud provider, got %d: %v", len(keys), keys)
	}

	// CloudCredentialKeys should still produce the bare key
	cloud := CloudCredentialKeys(s)
	assertCredKey(t, cloud, "ANTHROPIC_API_KEY", "model", false)
}

func TestResolveEnvVars_JiraIntegration(t *testing.T) {
	// Verifies that Jira custom provider credentials are injected into the agent
	// container environment via ResolveEnvVars.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
				{Name: "BASE_URL", Datatype: "string", Secret: true},
				{Name: "EMAIL", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "jira"},
		},
	}

	credentials := map[string]string{
		"JIRA_API_KEY":  "test-api-key",
		"JIRA_BASE_URL": "https://test.atlassian.net",
		"JIRA_EMAIL":    "user@example.com",
	}

	result := ResolveEnvVars(s, nil, credentials, nil)

	// All three credentials should be in the agent environment.
	assertEnv(t, result.Agent, "JIRA_API_KEY", "test-api-key")
	assertEnv(t, result.Agent, "JIRA_BASE_URL", "https://test.atlassian.net")
	assertEnv(t, result.Agent, "JIRA_EMAIL", "user@example.com")
}

// ─── §8.2 Self-hosted provider connection wiring ─────────────────────────────

func TestAgentConnectionKeys_SelfHostedModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama", Models: []string{"llama3.2"}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.local": {Host: "model-local", Port: "11434", URL: "http://model-local:11434", BaseURL: "http://model-local:11434/api"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "OLLAMA_HOST", "model-local")
	assertEnv(t, env, "OLLAMA_PORT", "11434")
	assertEnv(t, env, "OLLAMA_URL", "http://model-local:11434")
	assertEnv(t, env, "OLLAMA_BASE_URL", "http://model-local:11434/api")
	assertEnv(t, env, "OLLAMA_MODEL", "llama3.2")
}

func TestAgentConnectionKeys_SelfHostedModel_MultiModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama", Models: []string{"llama3.2", "mistral"}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.local": {Host: "model-local", Port: "11434", URL: "http://model-local:11434", BaseURL: "http://model-local:11434/api"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "OLLAMA_HOST", "model-local")
	assertEnv(t, env, "OLLAMA_PORT", "11434")
	assertEnv(t, env, "OLLAMA_URL", "http://model-local:11434")
	assertEnv(t, env, "OLLAMA_BASE_URL", "http://model-local:11434/api")
	assertEnv(t, env, "OLLAMA_MODEL", "llama3.2,mistral")
}

func TestAgentConnectionKeys_SelfHostedModel_NoModel(t *testing.T) {
	// When model name is not set, OLLAMA_MODEL must not be injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.local": {Host: "h", Port: "11434", URL: "http://h:11434", BaseURL: "http://h:11434/api"},
	}
	env := AgentConnectionKeys(s, addrs)
	if _, ok := env["OLLAMA_MODEL"]; ok {
		t.Error("OLLAMA_MODEL must not be set when model name is empty")
	}
}

func TestAgentConnectionKeys_DuplicateSelfHostedModelProvider(t *testing.T) {
	// Two entries both using ollama → qualified keys + bare keys for the first alphabetically.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"large": {Provider: "ollama", Models: []string{"llama3.2:70b"}},
			"small": {Provider: "ollama", Models: []string{"llama3.2"}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.large": {Host: "model-large", Port: "11434", URL: "http://model-large:11434", BaseURL: "http://model-large:11434/api"},
		"models.small": {Host: "model-small", Port: "11435", URL: "http://model-small:11435", BaseURL: "http://model-small:11435/api"},
	}
	env := AgentConnectionKeys(s, addrs)

	// "large" < "small" alphabetically → "large" is first → gets bare + qualified.
	assertEnv(t, env, "OLLAMA_HOST", "model-large")       // bare, first
	assertEnv(t, env, "OLLAMA_LARGE_HOST", "model-large") // qualified
	assertEnv(t, env, "OLLAMA_SMALL_HOST", "model-small") // qualified only
	assertEnv(t, env, "OLLAMA_LARGE_MODEL", "llama3.2:70b")
	assertEnv(t, env, "OLLAMA_SMALL_MODEL", "llama3.2")
	// Bare MODEL key goes to the first (alphabetically) entry — "large".
	assertEnv(t, env, "OLLAMA_MODEL", "llama3.2:70b")
}

// ─── §8.2 Self-hosted knowledge provider connection wiring ───────────────────

func TestAgentConnectionKeys_SelfHostedKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.docs": {Host: "knowledge-docs", Port: "6333", URL: "http://knowledge-docs:6333"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "QDRANT_HOST", "knowledge-docs")
	assertEnv(t, env, "QDRANT_PORT", "6333")
	assertEnv(t, env, "QDRANT_URL", "http://knowledge-docs:6333")
}

func TestAgentConnectionKeys_SelfHostedKnowledge_WithRedisURL(t *testing.T) {
	// Redis has URLScheme="redis" → URL key IS injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"cache": {Provider: "redis"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.cache": {Host: "knowledge-cache", Port: "6379", URL: "redis://knowledge-cache:6379"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "REDIS_HOST", "knowledge-cache")
	assertEnv(t, env, "REDIS_PORT", "6379")
	assertEnv(t, env, "REDIS_URL", "redis://knowledge-cache:6379")
}

func TestAgentConnectionKeys_SelfHostedKnowledge_NoURL(t *testing.T) {
	// Postgres provider has no URLScheme → URL key must not be injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"db": {Provider: "postgres"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.db": {Host: "knowledge-db", Port: "5432"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "POSTGRES_HOST", "knowledge-db")
	assertEnv(t, env, "POSTGRES_PORT", "5432")
	if _, ok := env["POSTGRES_URL"]; ok {
		t.Error("POSTGRES_URL must not be injected (postgres has no URLScheme)")
	}
}

func TestAgentConnectionKeys_DuplicateSelfHostedKnowledgeProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs":   {Provider: "qdrant"},
			"images": {Provider: "qdrant"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.docs":   {Host: "kn-docs", Port: "6333", URL: "http://kn-docs:6333"},
		"knowledge.images": {Host: "kn-images", Port: "6334", URL: "http://kn-images:6334"},
	}
	env := AgentConnectionKeys(s, addrs)

	// "docs" < "images" → bare keys go to docs.
	assertEnv(t, env, "QDRANT_HOST", "kn-docs")
	assertEnv(t, env, "QDRANT_DOCS_HOST", "kn-docs")
	assertEnv(t, env, "QDRANT_IMAGES_HOST", "kn-images")
}

// ─── §8.3 Container-mode connection wiring ───────────────────────────────────

func TestAgentConnectionKeys_ContainerModeModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"embedder": {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.embedder": {Host: "model-embedder", Port: "8000", URL: "http://model-embedder:8000"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "MODEL_EMBEDDER_HOST", "model-embedder")
	assertEnv(t, env, "MODEL_EMBEDDER_PORT", "8000")
	assertEnv(t, env, "MODEL_EMBEDDER_URL", "http://model-embedder:8000")
}

func TestAgentConnectionKeys_ContainerModeModel_NameSanitized(t *testing.T) {
	// Entry name "my_embedder" → key prefix "MODEL_MY_EMBEDDER_*"
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"my_embedder": {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.my_embedder": {Host: "model-my-embedder", Port: "8000", URL: "http://model-my-embedder:8000"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "MODEL_MY_EMBEDDER_HOST", "model-my-embedder")
}

func TestAgentConnectionKeys_ContainerModeKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"custom": {Container: &ContainerConfig{Image: "mydb:latest", Port: 5432}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.custom": {Host: "knowledge-custom", Port: "5432"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "KNOWLEDGE_CUSTOM_HOST", "knowledge-custom")
	assertEnv(t, env, "KNOWLEDGE_CUSTOM_PORT", "5432")
	// Container-mode knowledge does NOT inject URL.
	if _, ok := env["KNOWLEDGE_CUSTOM_URL"]; ok {
		t.Error("KNOWLEDGE_CUSTOM_URL must not be injected for container-mode knowledge")
	}
}

func TestAgentConnectionKeys_ContainerModeTool(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Integrations: map[string]Integration{
			"search": {Container: &ContainerConfig{Image: "search:latest", Port: 3000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"integrations.search": {Host: "integration-search", Port: "3000", URL: "http://integration-search:3000"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "INTEGRATION_SEARCH_HOST", "integration-search")
	assertEnv(t, env, "INTEGRATION_SEARCH_PORT", "3000")
	assertEnv(t, env, "INTEGRATION_SEARCH_URL", "http://integration-search:3000")
}

func TestAgentConnectionKeys_CloudProviderSkipped(t *testing.T) {
	// Cloud model, knowledge, and tool providers produce no connection keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"ai": {Provider: "anthropic"},
		},
		Knowledge: map[string]Knowledge{
			"vec": {Provider: "pinecone"},
		},
		Integrations: map[string]Integration{
			"gh": {Provider: "github"},
		},
	}
	env := AgentConnectionKeys(s, nil)
	if len(env) != 0 {
		t.Errorf("cloud-only providers should produce no connection keys, got %v", env)
	}
}

func TestAgentConnectionKeys_CustomProviderIntegrationSkipped(t *testing.T) {
	// Custom provider referenced by an integration → no connection wiring.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Integrations: map[string]Integration{
			"jira": {Provider: "my-jira"},
		},
	}
	env := AgentConnectionKeys(s, nil)
	if len(env) != 0 {
		t.Errorf("custom provider tool should produce no connection keys, got %v", env)
	}
}

// ─── §8.4 Input injection ─────────────────────────────────────────────────────

func TestResolveEnvVars_TopLevelInputs(t *testing.T) {
	// Top-level inputs are injected into ALL containers.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"LOG_LEVEL": {Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
		},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant"},
		},
		Ingestion: map[string]Ingestion{
			"sync": {Container: ContainerConfig{Image: "sync:latest"}, Trigger: IngestionTrigger{Type: "schedule"}},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)

	assertEnv(t, res.Agent, "LOG_LEVEL", "info")
	assertEnv(t, res.Models["llm"], "LOG_LEVEL", "info")
	assertEnv(t, res.Knowledge["docs"], "LOG_LEVEL", "info")
	assertEnv(t, res.Ingestion["sync"], "LOG_LEVEL", "info")
}

func TestResolveEnvVars_TopLevelInputs_UserOverridesDefault(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"LOG_LEVEL": {Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, map[string]string{"LOG_LEVEL": "debug"})
	assertEnv(t, res.Agent, "LOG_LEVEL", "debug")
}

func TestResolveEnvVars_TopLevelInputs_EmptyDefaultAndNoValue(t *testing.T) {
	// When default is empty and no user value, the key must not appear.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"OPTIONAL_KEY": {Name: "OPTIONAL_KEY", Datatype: "string"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	if _, ok := res.Agent["OPTIONAL_KEY"]; ok {
		t.Error("OPTIONAL_KEY with no default and no value must not appear in env")
	}
}

func TestResolveEnvVars_AgentInputs(t *testing.T) {
	// Agent-specific inputs go only to the agent.
	s := &AstroSpec{
		Name: "agent",
		Agent: Container{
			Image: "a:1",
			Inputs: []Input{
				{Name: "LOG_LEVEL", Datatype: "string", Default: "warn"},
			},
		},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Agent, "LOG_LEVEL", "warn")
	if _, ok := res.Models["llm"]["LOG_LEVEL"]; ok {
		t.Error("agent-specific input must not appear in model container")
	}
}

func TestResolveEnvVars_ModelInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {
				Provider: "ollama",
				Inputs: []Input{
					{Name: "BATCH_SIZE", Datatype: "number", Default: "32"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Models["llm"], "BATCH_SIZE", "32")
	if _, ok := res.Agent["BATCH_SIZE"]; ok {
		t.Error("model-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_KnowledgeInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs": {
				Provider: "qdrant",
				Inputs: []Input{
					{Name: "COLLECTION_NAME", Datatype: "string", Default: "embeddings"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Knowledge["docs"], "COLLECTION_NAME", "embeddings")
	if _, ok := res.Agent["COLLECTION_NAME"]; ok {
		t.Error("knowledge-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_IntegrationInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Integrations: map[string]Integration{
			"search": {
				Container: &ContainerConfig{Image: "search:latest", Port: 3000},
				Inputs: []Input{
					{Name: "RESULT_LIMIT", Datatype: "number", Default: "10"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Integrations["search"], "RESULT_LIMIT", "10")
	if _, ok := res.Agent["RESULT_LIMIT"]; ok {
		t.Error("integration-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_IngestionInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Ingestion: map[string]Ingestion{
			"sync": {
				Container: ContainerConfig{Image: "sync:latest"},
				Trigger:   IngestionTrigger{Type: "schedule"},
				Inputs: []Input{
					{Name: "BATCH_SIZE", Datatype: "number", Default: "100"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Ingestion["sync"], "BATCH_SIZE", "100")
	if _, ok := res.Agent["BATCH_SIZE"]; ok {
		t.Error("ingestion-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_InputScopeIsolation(t *testing.T) {
	// Each component type must only receive inputs declared for it.
	// (No cross-contamination between model, knowledge, tool, ingestion inputs.)
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "ollama", Inputs: []Input{{Name: "MODEL_FLAG", Datatype: "string", Default: "x"}}},
		},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant", Inputs: []Input{{Name: "K_FLAG", Datatype: "string", Default: "y"}}},
		},
		Integrations: map[string]Integration{
			"srch": {Container: &ContainerConfig{Image: "s:1"}, Inputs: []Input{{Name: "TOOL_FLAG", Datatype: "string", Default: "z"}}},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)

	// model input only in model container
	assertEnv(t, res.Models["llm"], "MODEL_FLAG", "x")
	if _, ok := res.Knowledge["docs"]["MODEL_FLAG"]; ok {
		t.Error("model input leaked into knowledge container")
	}
	if _, ok := res.Integrations["srch"]["MODEL_FLAG"]; ok {
		t.Error("model input leaked into integration container")
	}

	// knowledge input only in knowledge container
	assertEnv(t, res.Knowledge["docs"], "K_FLAG", "y")
	if _, ok := res.Models["llm"]["K_FLAG"]; ok {
		t.Error("knowledge input leaked into model container")
	}

	// integration input only in integration container
	assertEnv(t, res.Integrations["srch"], "TOOL_FLAG", "z")
	if _, ok := res.Models["llm"]["TOOL_FLAG"]; ok {
		t.Error("integration input leaked into model container")
	}
}

// ─── Full spec: combined injection ───────────────────────────────────────────

func TestResolveEnvVars_FullSpec(t *testing.T) {
	s := &AstroSpec{
		Name: "agent",
		Agent: Container{
			Image: "agent:latest",
			Inputs: []Input{
				{Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
			},
		},
		Inputs: map[string]Input{
			"ALLOWED_ORIGINS": {Name: "ALLOWED_ORIGINS", Datatype: "string", Default: "http://localhost"},
		},
		Models: map[string]Model{
			"llm":       {Provider: "ollama", Models: []string{"llama3.2"}},
			"embedder":  {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
			"anthropic": {Provider: "anthropic"},
		},
		Knowledge: map[string]Knowledge{
			"docs":  {Provider: "qdrant"},
			"cache": {Provider: "redis"},
		},
		Integrations: map[string]Integration{
			"github": {Provider: "github"},
			"search": {Container: &ContainerConfig{Image: "search:latest", Port: 3000}},
		},
		Providers: map[string]CustomProvider{
			"my-jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "API_KEY", Datatype: "string", Secret: true},
			}},
		},
	}

	addrs := map[string]ConnectionAddress{
		"models.llm":          {Host: "model-llm", Port: "11434", URL: "http://model-llm:11434", BaseURL: "http://model-llm:11434/api"},
		"models.embedder":     {Host: "model-embedder", Port: "8000", URL: "http://model-embedder:8000"},
		"knowledge.docs":      {Host: "knowledge-docs", Port: "6333", URL: "http://knowledge-docs:6333"},
		"knowledge.cache":     {Host: "knowledge-cache", Port: "6379"},
		"integrations.search": {Host: "integration-search", Port: "3000", URL: "http://integration-search:3000"},
	}
	creds := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-test",
		"GITHUB_TOKEN":      "ghp_test",
	}

	res := ResolveEnvVars(s, addrs, creds, nil)

	// Self-hosted model connections
	assertEnv(t, res.Agent, "OLLAMA_HOST", "model-llm")
	assertEnv(t, res.Agent, "OLLAMA_MODEL", "llama3.2")
	assertEnv(t, res.Agent, "OLLAMA_BASE_URL", "http://model-llm:11434/api")

	// Container-mode model connections
	assertEnv(t, res.Agent, "MODEL_EMBEDDER_HOST", "model-embedder")
	assertEnv(t, res.Agent, "MODEL_EMBEDDER_URL", "http://model-embedder:8000")

	// Self-hosted knowledge connections
	assertEnv(t, res.Agent, "QDRANT_HOST", "knowledge-docs")
	assertEnv(t, res.Agent, "QDRANT_URL", "http://knowledge-docs:6333")
	assertEnv(t, res.Agent, "REDIS_HOST", "knowledge-cache")

	// Container-mode integration connections
	assertEnv(t, res.Agent, "INTEGRATION_SEARCH_HOST", "integration-search")
	assertEnv(t, res.Agent, "INTEGRATION_SEARCH_URL", "http://integration-search:3000")

	// Cloud credentials
	assertEnv(t, res.Agent, "ANTHROPIC_API_KEY", "sk-ant-test")
	assertEnv(t, res.Agent, "GITHUB_TOKEN", "ghp_test")

	// Inputs
	assertEnv(t, res.Agent, "LOG_LEVEL", "info")
	assertEnv(t, res.Agent, "ALLOWED_ORIGINS", "http://localhost")

	// Top-level input in all containers
	assertEnv(t, res.Models["llm"], "ALLOWED_ORIGINS", "http://localhost")
	assertEnv(t, res.Knowledge["docs"], "ALLOWED_ORIGINS", "http://localhost")
	assertEnv(t, res.Integrations["search"], "ALLOWED_ORIGINS", "http://localhost")

	// Agent-specific input not in other containers
	if _, ok := res.Models["llm"]["LOG_LEVEL"]; ok {
		t.Error("agent input LOG_LEVEL must not appear in model container")
	}

	// Cloud model/tool providers produce no connection keys
	if _, ok := res.Agent["MODEL_ANTHROPIC_HOST"]; ok {
		t.Error("cloud provider anthropic must not produce connection keys")
	}
	if _, ok := res.Agent["INTEGRATION_GITHUB_HOST"]; ok {
		t.Error("cloud provider github must not produce connection keys")
	}
}

func TestResolveEnvVars_EmptySpec(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	if len(res.Agent) != 0 {
		t.Errorf("empty spec should produce empty agent env, got %v", res.Agent)
	}
}

func TestResolveEnvVars_CredentialsWiredIntoAgent(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
	}
	creds := map[string]string{
		"MY_API_KEY": "secret123",
	}
	res := ResolveEnvVars(s, nil, creds, nil)
	assertEnv(t, res.Agent, "MY_API_KEY", "secret123")
	// Credentials must not leak into other (non-existent) containers.
	if len(res.Models) != 0 || len(res.Knowledge) != 0 {
		t.Error("credential must not create phantom component entries")
	}
}

// ─── Deferred placeholder values (deployment template use-case) ──────────────

func TestAgentConnectionKeys_DeferredPlaceholders(t *testing.T) {
	// Simulate the deployment template use-case: addrs hold ${...} references.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.llm": {
			Host:    "${models.llm.host}",
			Port:    "${models.llm.port}",
			URL:     "${models.llm.url}",
			BaseURL: "${models.llm.url}/api",
		},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "OLLAMA_HOST", "${models.llm.host}")
	assertEnv(t, env, "OLLAMA_PORT", "${models.llm.port}")
	assertEnv(t, env, "OLLAMA_URL", "${models.llm.url}")
	assertEnv(t, env, "OLLAMA_BASE_URL", "${models.llm.url}/api")
}

// ─── connectionKeySource / AllAgentAutoEnvKeys ───────────────────────────────

func TestAllAgentAutoEnvKeys_SelfHostedModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama", Models: []string{"llama3.2"}},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"OLLAMA_HOST", "OLLAMA_PORT", "OLLAMA_URL", "OLLAMA_BASE_URL", "OLLAMA_MODEL"} {
		assertAutoEnvMeta(t, meta, key, "connection", "ollama", "model")
	}
}

func TestAllAgentAutoEnvKeys_ContainerModeModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"embedder": {Container: &ContainerConfig{Image: "embed:1", Port: 8000}},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"MODEL_EMBEDDER_HOST", "MODEL_EMBEDDER_PORT", "MODEL_EMBEDDER_URL"} {
		assertAutoEnvMeta(t, meta, key, "connection", "embedder", "model")
	}
}

func TestAllAgentAutoEnvKeys_ContainerModeModel_NameUsedAsProvider(t *testing.T) {
	// Sanitized entry name feeds the provider field, not a builtin provider string.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"my_embed": {Container: &ContainerConfig{Image: "embed:1", Port: 8000}},
		},
	}
	meta := AllAgentAutoEnvKeys(s)
	assertAutoEnvMeta(t, meta, "MODEL_MY_EMBED_HOST", "connection", "my_embed", "model")
}

func TestAllAgentAutoEnvKeys_SelfHostedKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant"},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"QDRANT_HOST", "QDRANT_PORT", "QDRANT_URL"} {
		assertAutoEnvMeta(t, meta, key, "connection", "qdrant", "knowledge")
	}
}

func TestAllAgentAutoEnvKeys_ContainerModeKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"custom": {Container: &ContainerConfig{Image: "mydb:1", Port: 5432}},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"KNOWLEDGE_CUSTOM_HOST", "KNOWLEDGE_CUSTOM_PORT"} {
		assertAutoEnvMeta(t, meta, key, "connection", "custom", "knowledge")
	}
}

func TestAllAgentAutoEnvKeys_ContainerModeTool_NoProvider(t *testing.T) {
	// Tool with only a Container field — entry name is used as provider.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Integrations: map[string]Integration{
			"search": {Container: &ContainerConfig{Image: "search:1", Port: 3000}},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"INTEGRATION_SEARCH_HOST", "INTEGRATION_SEARCH_PORT", "INTEGRATION_SEARCH_URL"} {
		assertAutoEnvMeta(t, meta, key, "connection", "search", "integration")
	}
}

func TestAllAgentAutoEnvKeys_ContainerModeTool_WithProvider(t *testing.T) {
	// Tool referencing a non-cloud, non-custom provider — t.Provider is used.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Integrations: map[string]Integration{
			"scraper": {Provider: "browserbase"},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for _, key := range []string{"INTEGRATION_SCRAPER_HOST", "INTEGRATION_SCRAPER_PORT", "INTEGRATION_SCRAPER_URL"} {
		assertAutoEnvMeta(t, meta, key, "connection", "browserbase", "integration")
	}
}

func TestAllAgentAutoEnvKeys_CloudCredential(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"ai": {Provider: "anthropic"},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	m, ok := meta["ANTHROPIC_API_KEY"]
	if !ok {
		t.Fatal("ANTHROPIC_API_KEY missing from AllAgentAutoEnvKeys result")
	}
	if m.Source != "credential" {
		t.Errorf("Source = %q, want %q", m.Source, "credential")
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", m.Provider, "anthropic")
	}
	if m.Category != "model" {
		t.Errorf("Category = %q, want %q", m.Category, "model")
	}
}

func TestAllAgentAutoEnvKeys_NoEmptyProviderOrCategory(t *testing.T) {
	// All keys produced by a mixed spec must have non-empty Provider and Category.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm":      {Provider: "ollama", Models: []string{"llama3.2"}},
			"embedder": {Container: &ContainerConfig{Image: "embed:1", Port: 8000}},
		},
		Knowledge: map[string]Knowledge{
			"docs":   {Provider: "qdrant"},
			"custom": {Container: &ContainerConfig{Image: "mydb:1", Port: 5432}},
		},
		Integrations: map[string]Integration{
			"search":  {Container: &ContainerConfig{Image: "search:1", Port: 3000}},
			"scraper": {Provider: "browserbase"},
		},
	}
	meta := AllAgentAutoEnvKeys(s)

	for key, m := range meta {
		if m.Provider == "" {
			t.Errorf("key %q has empty Provider (source=%q, category=%q)", key, m.Source, m.Category)
		}
		if m.Category == "" {
			t.Errorf("key %q has empty Category (source=%q, provider=%q)", key, m.Source, m.Provider)
		}
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func assertEnv(t *testing.T, env map[string]string, key, want string) {
	t.Helper()
	got, ok := env[key]
	if !ok {
		t.Errorf("env[%q] missing (want %q)", key, want)
		return
	}
	if got != want {
		t.Errorf("env[%q] = %q, want %q", key, got, want)
	}
}

func assertAutoEnvMeta(t *testing.T, meta map[string]AgentEnvMeta, key, wantSource, wantProvider, wantCategory string) {
	t.Helper()
	m, ok := meta[key]
	if !ok {
		t.Errorf("AllAgentAutoEnvKeys: key %q missing", key)
		return
	}
	if m.Source != wantSource {
		t.Errorf("key %q: Source = %q, want %q", key, m.Source, wantSource)
	}
	if m.Provider != wantProvider {
		t.Errorf("key %q: Provider = %q, want %q", key, m.Provider, wantProvider)
	}
	if m.Category != wantCategory {
		t.Errorf("key %q: Category = %q, want %q", key, m.Category, wantCategory)
	}
}

// ─── Managed providers ──────────────────────────────────────────────────────

func TestCloudCredentialKeys_ManagedProviderSkipped(t *testing.T) {
	// anthropic-managed is a managed provider — it must NOT produce credential keys
	// because the server injects the API key, not the user.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"claude": {Provider: "anthropic-managed"},
		},
	}
	keys := CloudCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("managed provider should produce 0 credential keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_ManagedAndRegularCoexist(t *testing.T) {
	// When both anthropic (user-provided) and anthropic-managed (platform-provided)
	// are used, only the regular anthropic produces credential keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"user-claude":    {Provider: "anthropic"},
			"managed-claude": {Provider: "anthropic-managed"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	if _, ok := keys["ANTHROPIC_MANAGED_API_KEY"]; ok {
		t.Error("managed provider must not produce ANTHROPIC_MANAGED_API_KEY")
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key (only from regular anthropic), got %d: %v", len(keys), keys)
	}
}

func TestIsManagedProvider(t *testing.T) {
	if !IsManagedProvider("models", "anthropic-managed") {
		t.Error("anthropic-managed should be a managed provider")
	}
	if IsManagedProvider("models", "anthropic") {
		t.Error("anthropic should not be a managed provider")
	}
	if IsManagedProvider("models", "openai") {
		t.Error("openai should not be a managed provider")
	}
	if IsManagedProvider("models", "nonexistent") {
		t.Error("nonexistent should not be a managed provider")
	}
}

func assertCredKey(t *testing.T, keys map[string]CredentialMeta, key, wantCategory string, wantOptional bool) {
	t.Helper()
	m, ok := keys[key]
	if !ok {
		t.Errorf("credential key %q missing", key)
		return
	}
	if m.Category != wantCategory {
		t.Errorf("key %q: category = %q, want %q", key, m.Category, wantCategory)
	}
	if m.Optional != wantOptional {
		t.Errorf("key %q: Optional = %v, want %v", key, m.Optional, wantOptional)
	}
}
