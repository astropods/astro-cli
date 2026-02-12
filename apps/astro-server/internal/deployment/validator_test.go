package deployment

import (
	"testing"

	"github.com/postman/astro/packages/astro-spec"
)

// TestValidateSpec verifies that ValidateSpec rejects specs with missing agent
// name, missing version, missing container image/build, invalid trigger types,
// missing cron schedule, invalid cron expressions, and missing credentials. Also
// verifies that a valid spec with all credentials provided passes validation.
func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name           string
		spec           *spec.AstroSpec
		creds          map[string]string
		wantValid      bool
		wantErrorField string // if not empty, expect an error with this field
		wantMissing    int    // expected count of missing credentials
	}{
		{
			name: "valid minimal spec",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "missing agent name",
			spec: &spec.AstroSpec{
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "agent",
		},
		{
			name: "missing version",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Container: spec.Container{Image: "agent:latest"},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "meta.version",
		},
		{
			name: "missing container image and build",
			spec: &spec.AstroSpec{
				Agent: "my-agent",
				Meta:  spec.Meta{Version: "1.0"},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "container",
		},
		{
			name: "container with build config is valid",
			spec: &spec.AstroSpec{
				Agent: "my-agent",
				Meta:  spec.Meta{Version: "1.0"},
				Container: spec.Container{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
				},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "invalid trigger type",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
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
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
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
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"sync": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule", Schedule: "not-a-cron"},
					},
				},
			},
			creds:          map[string]string{},
			wantValid:      false,
			wantErrorField: "ingestion.sync.trigger.schedule",
		},
		{
			name: "valid cron expression",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Ingestion: map[string]spec.Ingestion{
					"sync": {
						Container: spec.ContainerConfig{Image: "worker:latest"},
						Trigger:   spec.IngestionTrigger{Type: "schedule", Schedule: "0 * * * *"},
					},
				},
			},
			creds:     map[string]string{},
			wantValid: true,
		},
		{
			name: "missing credentials for anthropic model",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Integrations: spec.Integrations{
					Models: []spec.IntegrationModel{
						{Name: "claude", Provider: "anthropic"},
					},
				},
			},
			creds:       map[string]string{},
			wantValid:   false,
			wantMissing: 1,
		},
		{
			name: "credentials provided for anthropic model",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Integrations: spec.Integrations{
					Models: []spec.IntegrationModel{
						{Name: "claude", Provider: "anthropic"},
					},
				},
			},
			creds:     map[string]string{"ANTHROPIC_API_KEY": "sk-test"},
			wantValid: true,
		},
		{
			name: "slack interface requires tokens",
			spec: &spec.AstroSpec{
				Agent:     "my-agent",
				Meta:      spec.Meta{Version: "1.0"},
				Container: spec.Container{Image: "agent:latest"},
				Interfaces: map[string]spec.Interface{
					"slack": {Type: "slack"},
				},
			},
			creds:       map[string]string{},
			wantValid:   false,
			wantMissing: 2, // SLACK_APP_TOKEN + SLACK_BOT_TOKEN
		},
	}

	v := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.ValidateSpec(tt.spec, tt.creds)

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

			if tt.wantMissing > 0 && len(result.MissingCredentials) != tt.wantMissing {
				t.Errorf("missing credentials: expected %d, got %d: %v",
					tt.wantMissing, len(result.MissingCredentials), result.MissingCredentials)
			}
		})
	}
}

// TestGetRequiredCredentials verifies that GetRequiredCredentials returns the
// correct credential info for known providers (anthropic, openai, google/gemini,
// cohere, pinecone, github, gitlab), unknown providers, self-hosted providers
// (no credential), slack interfaces, and env prefix overrides.
func TestGetRequiredCredentials(t *testing.T) {
	v := NewValidator()

	t.Run("known providers", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{Name: "claude", Provider: "anthropic"},
					{Name: "gpt", Provider: "openai"},
					{Name: "gemini", Provider: "google"},
					{Name: "cohere", Provider: "cohere"},
				},
				Knowledge: []spec.IntegrationKnowledge{
					{Name: "pinecone-store", Provider: "pinecone"},
				},
				Tools: []spec.IntegrationTool{
					{Name: "gh", Provider: "github"},
					{Name: "gl", Provider: "gitlab"},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
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
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{Name: "gem", Provider: "gemini"},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
		if len(creds) != 1 || creds[0].Key != "GOOGLE_API_KEY" {
			t.Errorf("gemini should map to GOOGLE_API_KEY, got %v", creds)
		}
	})

	t.Run("self-hosted produces no credential", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{Name: "local-model", Provider: "self-hosted"},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
		if len(creds) != 0 {
			t.Errorf("self-hosted should not require credentials, got %v", creds)
		}
	})

	t.Run("unknown provider generates generic key", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{Name: "custom", Provider: "mistral"},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
		if len(creds) != 1 || creds[0].Key != "MISTRAL_API_KEY" {
			t.Errorf("unknown provider should generate {PROVIDER}_API_KEY, got %v", creds)
		}
	})

	t.Run("env prefix overrides key", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{
						Name:     "claude",
						Provider: "anthropic",
						Env:      &spec.IntegrationEnv{Prefix: "MY_"},
					},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
		if len(creds) != 1 || creds[0].Key != "MY_ANTHROPIC_API_KEY" {
			t.Errorf("expected MY_ANTHROPIC_API_KEY with prefix, got %v", creds)
		}
	})

	t.Run("slack interface credentials", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Interfaces: map[string]spec.Interface{
				"slack": {Type: "slack"},
			},
		}

		creds := v.GetRequiredCredentials(s)
		credKeys := make(map[string]bool)
		for _, c := range creds {
			credKeys[c.Key] = true
		}

		if !credKeys["SLACK_APP_TOKEN"] || !credKeys["SLACK_BOT_TOKEN"] {
			t.Errorf("slack should require SLACK_APP_TOKEN and SLACK_BOT_TOKEN, got %v", credKeys)
		}
	})

	t.Run("deduplicates same provider", func(t *testing.T) {
		s := &spec.AstroSpec{
			Agent:     "my-agent",
			Meta:      spec.Meta{Version: "1.0"},
			Container: spec.Container{Image: "agent:latest"},
			Integrations: spec.Integrations{
				Models: []spec.IntegrationModel{
					{Name: "claude-3", Provider: "anthropic"},
					{Name: "claude-4", Provider: "anthropic"},
				},
			},
		}

		creds := v.GetRequiredCredentials(s)
		if len(creds) != 1 {
			t.Errorf("duplicate providers should be deduplicated, got %d credentials", len(creds))
		}
	})
}
