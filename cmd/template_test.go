package cmd

import (
	"testing"

	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
)

// TestAstroYml_TemplatePassesValidation renders astropods.yml from each scaffold
// template and runs it through the full validateSpecFile pipeline (JSON schema +
// semantic). This is the same path as `ast validate` / `ast-dev push`, so any
// required field missing from a template will be caught here.
func TestAstroYml_TemplatePassesValidation(t *testing.T) {
	tests := []struct {
		template string
		config   scaffold.ScaffoldConfig
	}{
		{
			template: "mastra",
			config: scaffold.ScaffoldConfig{
				Name: "my-agent", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{},
			},
		},
		{
			template: "langchain",
			config: scaffold.ScaffoldConfig{
				Name: "my-agent", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			paths, err := scaffold.GetTemplatePaths(tt.template)
			if err != nil {
				t.Fatalf("GetTemplatePaths: %v", err)
			}
			rendered, err := scaffold.RenderTemplate(paths.AstroYml, tt.config)
			if err != nil {
				t.Fatalf("RenderTemplate: %v", err)
			}
			specPath := writeSpecFile(t, rendered)
			captureStdout(t, func() {
				if _, err := validateSpecFile(specPath); err != nil {
					t.Errorf("validateSpecFile failed for %s template:\n%s\nerror: %v", tt.template, rendered, err)
				}
			})
		})
	}
}
