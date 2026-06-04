package deployment

import (
	"encoding/json"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// TestValidateSpec verifies that ValidateSpec rejects specs with missing
// container image/build, invalid trigger types, missing cron schedule,
// invalid cron expressions, and missing credentials. Also verifies that
// a valid spec with all credentials provided passes validation.
func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name           string
		spec           *spec.AstroSpec
		creds          map[string]string
		interfaces     []string
		schedules      map[string]string
		wantValid      bool
		wantErrorField string // if not empty, expect an error with this field
		wantMissing    int    // expected count of missing credentials
	}{
		{
			name: "valid minimal spec",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "no version field is valid",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Agent: spec.Container{Image: "agent:latest"},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "missing container image and build",
			spec: &spec.AstroSpec{
				Name: "my-agent",
				Meta: spec.Meta{},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "agent",
		},
		{
			name: "container with build config is valid",
			spec: &spec.AstroSpec{
				Name: "my-agent",
				Meta: spec.Meta{},
				Agent: spec.Container{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
				},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "model container missing image and build",
			spec: &spec.AstroSpec{
				Agent:  spec.Container{Image: "agent:latest"},
				Models: map[string]spec.Model{"llm": {Container: &spec.ContainerConfig{}}},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "models.llm",
		},
		{
			name: "model provider mode skipped",
			spec: &spec.AstroSpec{
				Agent:  spec.Container{Image: "agent:latest"},
				Models: map[string]spec.Model{"llm": {Provider: "ollama"}},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "knowledge container missing image and build",
			spec: &spec.AstroSpec{
				Agent:     spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{"db": {Container: &spec.ContainerConfig{}}},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "knowledge.db",
		},
		{
			name: "knowledge provider mode skipped",
			spec: &spec.AstroSpec{
				Agent:     spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{"db": {Provider: "qdrant"}},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "integration container missing image and build",
			spec: &spec.AstroSpec{
				Agent:        spec.Container{Image: "agent:latest"},
				Integrations: map[string]spec.Integration{"search": {Container: &spec.ContainerConfig{}}},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "integrations.search",
		},
		{
			name: "integration provider mode skipped",
			spec: &spec.AstroSpec{
				Agent:        spec.Container{Image: "agent:latest"},
				Integrations: map[string]spec.Integration{"search": {Provider: "github"}},
			},
			creds:     map[string]string{"GITHUB_TOKEN": "ghp_test"},
			wantValid: true,
		},
		{
			name: "ingestion container missing image and build",
			spec: &spec.AstroSpec{
				Agent:     spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{"sync": {Container: spec.ContainerConfig{}, Trigger: spec.IngestionTrigger{Type: "manual"}}},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "ingestion.sync",
		},
		{
			name: "ingestion container with image passes",
			spec: &spec.AstroSpec{
				Agent:     spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{"sync": {Container: spec.ContainerConfig{Image: "worker:latest"}, Trigger: spec.IngestionTrigger{Type: "manual"}}},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "invalid trigger type",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"bad-trigger": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "invalid"},
					},
				},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "ingestion.bad-trigger.trigger.type",
		},
		{
			name: "schedule trigger without schedule expression",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"sync": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule"},
					},
				},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "ingestion.sync.trigger.schedule",
		},
		{
			name: "invalid cron expression",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"sync": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule"},
					},
				},
			},
			creds:          map[string]string{},
			schedules:      map[string]string{"sync": "not-a-cron"},
			wantValid:      false,
			wantErrorField: "ingestion.sync.trigger.schedule",
		},
		{
			name: "valid cron expression",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"sync": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule"},
					},
				},
			},
			creds:     map[string]string{},
			schedules: map[string]string{"sync": "0 * * * *"},
			wantValid: true,
		},
		{
			name: "missing credentials for anthropic model",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Models: map[string]spec.Model{
					"anthropic": {Provider: "anthropic"},
				},
			},
			creds:       map[string]string{},
			wantValid:   false,
			wantMissing: 1,
		},
		{
			name: "credentials provided for anthropic model",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Models: map[string]spec.Model{
					"anthropic": {Provider: "anthropic"},
				},
			},
			creds:     map[string]string{"ANTHROPIC_API_KEY": "sk-test"},
			wantValid: true,
		},
		{
			name: "custom provider with secret credentials provided",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Providers: map[string]spec.CustomProvider{
					"my-service": {
						Scope: []string{"integrations"},
						Variables: []spec.Input{
							{Name: "API_KEY", Datatype: "string", Secret: true, Description: "API key"},
							{Name: "SECRET", Datatype: "string", Secret: true, Description: "Shared secret"},
						},
					},
				},
				Integrations: map[string]spec.Integration{
					"jira": {Provider: "my-service"},
				},
			},
			creds:     map[string]string{"MY_SERVICE_API_KEY": "key1", "MY_SERVICE_SECRET": "s3cret"},
			wantValid: true,
		},
		{
			name: "custom provider with missing secret credentials",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Providers: map[string]spec.CustomProvider{
					"my-service": {
						Scope: []string{"integrations"},
						Variables: []spec.Input{
							{Name: "API_KEY", Datatype: "string", Secret: true, Description: "API key"},
							{Name: "SECRET", Datatype: "string", Secret: true, Description: "Shared secret"},
						},
					},
				},
				Integrations: map[string]spec.Integration{
					"jira": {Provider: "my-service"},
				},
			},
			creds:       map[string]string{},
			wantValid:   false,
			wantMissing: 2,
		},
		{
			name: "custom provider optional secret not required",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
				Providers: map[string]spec.CustomProvider{
					"my-service": {
						Scope: []string{"integrations"},
						Variables: []spec.Input{
							{Name: "API_KEY", Datatype: "string", Secret: true, Description: "API key"},
							{Name: "SECRET", Datatype: "string", Secret: true, Description: "Optional secret", Optional: true},
						},
					},
				},
				Integrations: map[string]spec.Integration{
					"jira": {Provider: "my-service"},
				},
			},
			creds:       map[string]string{"MY_SERVICE_API_KEY": "key1"},
			wantValid:   true,
			wantMissing: 0,
		},
		{
			name: "slack interface requires tokens",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
			},
			interfaces:  []string{"slack"},
			creds:       map[string]string{},
			wantValid:   false,
			wantMissing: 2, // SLACK_APP_TOKEN + SLACK_BOT_TOKEN
		},
		{
			name: "no interfaces means no interface creds",
			spec: &spec.AstroSpec{
				Name:  "my-agent",
				Meta:  spec.Meta{},
				Agent: spec.Container{Image: "agent:latest"},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
	}

	v := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.ValidateSpec(tt.spec, tt.creds, tt.interfaces, tt.schedules)

			if result.Valid != tt.wantValid {
				t.Errorf("Valid: expected %v, got %v (errors: %v)", tt.wantValid, result.Valid, result.Errors)
			}

			if tt.wantErrorField != "" {
				found := false
				for _, e := range result.Errors {
					if e.Field == tt.wantErrorField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error for field %q, got errors: %v", tt.wantErrorField, result.Errors)
				}
			}

			if tt.wantMissing > 0 && len(result.MissingVariables) != tt.wantMissing {
				t.Errorf("missing credentials: expected %d, got %d: %v",
					tt.wantMissing, len(result.MissingVariables), result.MissingVariables)
			}
		})
	}
}

// TestValidateSpec_ManagedProvider verifies that managed providers (e.g. anthropic-managed)
// do not require user-provided credentials, unlike regular cloud providers.
func TestValidateSpec_ManagedProvider(t *testing.T) {
	v := NewValidator()

	t.Run("anthropic-managed requires no credentials", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"claude": {Provider: "anthropic-managed"},
			},
		}
		result := v.ValidateSpec(s, map[string]string{}, nil, nil)
		if !result.Valid {
			t.Errorf("expected valid (managed provider needs no creds), got errors: %v", result.Errors)
		}
		if len(result.MissingVariables) != 0 {
			t.Errorf("expected 0 missing variables, got %v", result.MissingVariables)
		}
	})

	t.Run("regular anthropic still requires credentials", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"claude": {Provider: "anthropic"},
			},
		}
		result := v.ValidateSpec(s, map[string]string{}, nil, nil)
		if result.Valid {
			t.Error("expected invalid (anthropic needs creds)")
		}
		if len(result.MissingVariables) != 1 {
			t.Errorf("expected 1 missing variable, got %v", result.MissingVariables)
		}
	})

	t.Run("managed and regular together", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"managed-claude": {Provider: "anthropic-managed"},
				"user-openai":    {Provider: "openai"},
			},
		}
		// Only openai should require credentials
		result := v.ValidateSpec(s, map[string]string{}, nil, nil)
		if result.Valid {
			t.Error("expected invalid (openai needs creds)")
		}
		if len(result.MissingVariables) != 1 {
			t.Errorf("expected 1 missing variable (OPENAI_API_KEY), got %v", result.MissingVariables)
		}

		// Provide openai creds — should pass
		result = v.ValidateSpec(s, map[string]string{"OPENAI_API_KEY": "sk-test"}, nil, nil)
		if !result.Valid {
			t.Errorf("expected valid with openai creds provided, got errors: %v", result.Errors)
		}
	})
}

// TestGetRequiredCredentials_ManagedProviderExcluded verifies that managed providers
// don't appear in GetRequiredCredentials output.
func TestGetRequiredCredentials_ManagedProviderExcluded(t *testing.T) {
	v := NewValidator()

	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"claude": {Provider: "anthropic-managed"},
		},
	}
	creds := v.GetRequiredCredentials(s, nil)
	if len(creds) != 0 {
		keys := make([]string, 0, len(creds))
		for _, c := range creds {
			keys = append(keys, c.Key)
		}
		t.Errorf("managed provider should produce 0 credentials, got %v", keys)
	}
}

// TestGetRequiredCredentials verifies that GetRequiredCredentials returns the
// correct credential info for cloud providers in models/knowledge/tools,
// integrations, slack interfaces, and env prefix overrides.
func TestGetRequiredCredentials(t *testing.T) {
	v := NewValidator()

	t.Run("cloud providers in models knowledge tools", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"anthropic": {Provider: "anthropic"},
				"openai":    {Provider: "openai"},
				"google":    {Provider: "google"},
				"cohere":    {Provider: "cohere"},
			},
			Knowledge: map[string]spec.Knowledge{
				"pinecone": {Provider: "pinecone"},
			},
			Integrations: map[string]spec.Integration{
				"github": {Provider: "github"},
				"gitlab": {Provider: "gitlab"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}

		expected := []string{
			"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY",
			"COHERE_API_KEY", "PINECONE_API_KEY", "GITHUB_TOKEN", "GITLAB_TOKEN",
		}
		for _, key := range expected {
			if !credKeys[key] {
				t.Errorf("expected credential %s, got keys: %v", key, credKeys)
			}
		}
	})

	t.Run("gemini alias", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"gemini": {Provider: "gemini"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		if len(creds) != 1 || creds[0].Key != "GEMINI_API_KEY" {
			t.Errorf("gemini provider with name 'gemini' should produce GEMINI_API_KEY, got %v", creds)
		}
	})

	t.Run("unsupported model provider rejected", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"mistral": {Provider: "mistral"},
			},
		}

		result := v.ValidateSpec(s, map[string]string{}, nil, nil)
		if result.Valid {
			t.Error("expected validation to fail for unsupported provider")
		}
		found := false
		for _, e := range result.Errors {
			if e.Field == "models.mistral.provider" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected error on models.mistral.provider, got %v", result.Errors)
		}
	})

	t.Run("name-derived env vars from model", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"fallback": {Provider: "anthropic"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		if len(creds) != 1 || creds[0].Key != "ANTHROPIC_API_KEY" {
			t.Errorf("expected ANTHROPIC_API_KEY from provider 'anthropic', got %v", creds)
		}
	})

	t.Run("slack interface credentials", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
		}

		creds := v.GetRequiredCredentials(s, []string{"slack"})
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}

		if !credKeys["SLACK_APP_TOKEN"] || !credKeys["SLACK_BOT_TOKEN"] {
			t.Errorf("slack should require SLACK_APP_TOKEN and SLACK_BOT_TOKEN, got %v", credKeys)
		}
	})

	t.Run("different names same provider produce different keys", func(t *testing.T) {
		// Two entries share provider:anthropic and neither name matches the
		// provider. Per §8.1 (the resolver, which the deployer respects),
		// only the qualified keys are emitted — no bare ANTHROPIC_API_KEY,
		// since the deployer can't decide which entry owns it. Validator
		// must mirror the resolver: never ask the user for a credential
		// the deployer won't inject.
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"primary":  {Provider: "anthropic"},
				"fallback": {Provider: "anthropic"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}
		if len(creds) != 2 {
			t.Errorf("expected 2 credentials (qualified only, no bare), got %d: %v", len(creds), credKeys)
		}
		if !credKeys["ANTHROPIC_PRIMARY_API_KEY"] || !credKeys["ANTHROPIC_FALLBACK_API_KEY"] {
			t.Errorf("expected ANTHROPIC_PRIMARY_API_KEY and ANTHROPIC_FALLBACK_API_KEY, got %v", credKeys)
		}
		if credKeys["ANTHROPIC_API_KEY"] {
			t.Error("bare ANTHROPIC_API_KEY must NOT be emitted when no entry name matches the provider (matches §8.1 resolver semantic)")
		}
	})

	t.Run("name matching provider is primary and skips redundant key", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"anthropic": {Provider: "anthropic"},
				"fallback":  {Provider: "anthropic"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}
		// "anthropic" matches provider → bare key only (no ANTHROPIC_ANTHROPIC_API_KEY)
		// "fallback" → name-qualified key
		if len(creds) != 2 {
			t.Errorf("expected 2 credentials (bare + 1 name-qualified), got %d: %v", len(creds), credKeys)
		}
		if !credKeys["ANTHROPIC_API_KEY"] || !credKeys["ANTHROPIC_FALLBACK_API_KEY"] {
			t.Errorf("expected ANTHROPIC_API_KEY and ANTHROPIC_FALLBACK_API_KEY, got %v", credKeys)
		}
		if credKeys["ANTHROPIC_ANTHROPIC_API_KEY"] {
			t.Error("should not produce redundant ANTHROPIC_ANTHROPIC_API_KEY")
		}
	})

	t.Run("custom provider secret credentials", func(t *testing.T) {
		// Variable names are suffixes per §5; the full key is {UPPER(provider)}_{varName}.
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Meta:  spec.Meta{},
			Agent: spec.Container{Image: "agent:latest"},
			Providers: map[string]spec.CustomProvider{
				"my-service": {
					Scope: []string{"integrations"},
					Variables: []spec.Input{
						{Name: "API_KEY", Datatype: "string", Secret: true, Description: "API key for my-service"},
						{Name: "SECRET", Datatype: "string", Secret: true, Description: "Shared secret", Optional: true},
					},
				},
			},
			Integrations: map[string]spec.Integration{
				"jira": {Provider: "my-service"},
			},
		}

		creds := v.GetRequiredCredentials(s, nil)
		credKeys := make(map[string]CredentialInfo)
		for _, c := range creds {
			credKeys[c.Key] = c
		}

		if len(creds) != 2 {
			t.Fatalf("expected 2 credentials, got %d: %v", len(creds), credKeys)
		}
		if _, ok := credKeys["MY_SERVICE_API_KEY"]; !ok {
			t.Errorf("expected MY_SERVICE_API_KEY, got %v", credKeys)
		}
		if _, ok := credKeys["MY_SERVICE_SECRET"]; !ok {
			t.Errorf("expected MY_SERVICE_SECRET, got %v", credKeys)
		}
		if !credKeys["MY_SERVICE_SECRET"].Optional {
			t.Error("expected MY_SERVICE_SECRET to be optional")
		}
		if credKeys["MY_SERVICE_API_KEY"].Provider != "my-service" {
			t.Errorf("expected provider 'my-service', got %q", credKeys["MY_SERVICE_API_KEY"].Provider)
		}
	})

	t.Run("custom provider JSON round-trip", func(t *testing.T) {
		// Variable names are suffixes per §5; full key is {UPPER(provider)}_{varName}.
		rawSpec := map[string]interface{}{
			"spec": "package/v1",
			"name": "my-agent",
			"meta": map[string]interface{}{"description": "test"},
			"agent": map[string]interface{}{
				"image": "agent:latest",
			},
			"providers": map[string]interface{}{
				"my-service": map[string]interface{}{
					"scope": []interface{}{"integrations"},
					"variables": []interface{}{
						map[string]interface{}{"name": "API_KEY", "datatype": "string", "secret": true, "description": "API key"},
						map[string]interface{}{"name": "SECRET", "datatype": "string", "secret": true, "description": "Shared secret"},
					},
				},
			},
			"integrations": map[string]interface{}{
				"jira": map[string]interface{}{"provider": "my-service"},
			},
		}

		specJSON, err := json.Marshal(rawSpec)
		if err != nil {
			t.Fatalf("failed to marshal raw spec: %v", err)
		}

		var astroSpec spec.AstroSpec
		if err := json.Unmarshal(specJSON, &astroSpec); err != nil {
			t.Fatalf("failed to unmarshal into AstroSpec: %v", err)
		}

		provider, ok := astroSpec.Providers["my-service"]
		if !ok {
			t.Fatal("expected my-service provider after round-trip")
		}
		if len(provider.Variables) != 2 {
			t.Fatalf("expected 2 variables after round-trip, got %d", len(provider.Variables))
		}

		creds := v.GetRequiredCredentials(&astroSpec, nil)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}
		if !credKeys["MY_SERVICE_API_KEY"] || !credKeys["MY_SERVICE_SECRET"] {
			t.Errorf("expected MY_SERVICE_API_KEY and MY_SERVICE_SECRET, got %v", credKeys)
		}
	})

	t.Run("JSON round-trip preserves cloud providers in models and tools", func(t *testing.T) {
		rawSpec := map[string]interface{}{
			"spec": "package/v1",
			"name": "my-agent",
			"meta": map[string]interface{}{"version": "1.0"},
			"agent": map[string]interface{}{
				"image": "agent:latest",
			},
			"models": map[string]interface{}{
				"anthropic": map[string]interface{}{
					"provider": "anthropic",
				},
			},
			"integrations": map[string]interface{}{
				"github": map[string]interface{}{
					"provider": "github",
				},
			},
		}

		specJSON, err := json.Marshal(rawSpec)
		if err != nil {
			t.Fatalf("failed to marshal raw spec: %v", err)
		}

		var astroSpec spec.AstroSpec
		if err := json.Unmarshal(specJSON, &astroSpec); err != nil {
			t.Fatalf("failed to unmarshal into AstroSpec: %v", err)
		}

		if len(astroSpec.Models) != 1 {
			t.Fatalf("expected 1 model after round-trip, got %d", len(astroSpec.Models))
		}
		if len(astroSpec.Integrations) != 1 {
			t.Fatalf("expected 1 tool after round-trip, got %d", len(astroSpec.Integrations))
		}

		creds := v.GetRequiredCredentials(&astroSpec, nil)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}

		if !credKeys["ANTHROPIC_API_KEY"] {
			t.Errorf("expected ANTHROPIC_API_KEY, got %v", credKeys)
		}
		if !credKeys["GITHUB_TOKEN"] {
			t.Errorf("expected GITHUB_TOKEN, got %v", credKeys)
		}
		if len(creds) != 2 {
			t.Errorf("expected 2 credentials, got %d: %v", len(creds), credKeys)
		}
	})
}
