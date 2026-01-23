package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseSpec reads and parses an astro.yml file
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
	if spec.Agent == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if spec.Container.Build == nil && spec.Container.Image == "" {
		return nil, fmt.Errorf("container.build or container.image is required")
	}

	return &spec, nil
}
