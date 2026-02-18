package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseFile reads and parses an astro.yml file from the given path
func ParseFile(path string) (*AstroSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	return Parse(data)
}

// Parse parses astro.yml content from bytes
func Parse(data []byte) (*AstroSpec, error) {
	var spec AstroSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec YAML: %w", err)
	}

	return &spec, nil
}

// ParseString parses astro.yml content from a string
func ParseString(content string) (*AstroSpec, error) {
	return Parse([]byte(content))
}

// ParseSpec reads and parses an astro.yml file with validation
func ParseSpec(path string) (*AstroSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	var spec AstroSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}

	// Validate required fields
	if spec.Spec == "" {
		return nil, fmt.Errorf("spec version is required")
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if spec.Agent.Build == nil && spec.Agent.Image == "" {
		return nil, fmt.Errorf("agent.build or agent.image is required")
	}

	// Validate knowledge entries: provider and container are mutually exclusive
	for name, k := range spec.Knowledge {
		if k.Provider != "" && k.Container != nil {
			return nil, fmt.Errorf("knowledge %q: provider and container are mutually exclusive", name)
		}
		if k.Provider == "" && k.Container == nil {
			return nil, fmt.Errorf("knowledge %q: either provider or container is required", name)
		}
	}

	// Validate model entries: provider and container are mutually exclusive
	for name, m := range spec.Models {
		if m.Provider != "" && m.Container != nil {
			return nil, fmt.Errorf("model %q: provider and container are mutually exclusive", name)
		}
		if m.Provider == "" && m.Container == nil {
			return nil, fmt.Errorf("model %q: either provider or container is required", name)
		}
	}

	// Validate tool entries: provider and container are mutually exclusive
	for name, t := range spec.Tools {
		if t.Provider != "" && t.Container != nil {
			return nil, fmt.Errorf("tool %q: provider and container are mutually exclusive", name)
		}
		if t.Provider == "" && t.Container == nil {
			return nil, fmt.Errorf("tool %q: either provider or container is required", name)
		}
	}

	// Validate integrations: must have at least one credential
	for name, integration := range spec.Integrations {
		if len(integration.Credentials) == 0 {
			return nil, fmt.Errorf("integration %q: at least one credential is required", name)
		}
	}

	return &spec, nil
}
