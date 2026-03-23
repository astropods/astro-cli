package spec

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// unquotedAtName matches a top-level `name:` value starting with an unquoted @.
var unquotedAtName = regexp.MustCompile(`(?m)^name:\s+@`)

var validDatatypes = map[string]bool{
	"string": true, "boolean": true, "number": true, "array": true, "object": true,
}

var validDisplayAs = map[string]bool{
	"short-text": true, "long-text": true, "select": true,
}

// ParseFile reads and parses an spec file from the given path
func ParseFile(path string) (*AstroSpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	return Parse(data)
}

// Parse parses the spec content from bytes
func Parse(data []byte) (*AstroSpec, error) {
	var spec AstroSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		if unquotedAtName.Match(data) {
			return nil, fmt.Errorf("failed to parse spec YAML: the @ character in the name field must be quoted, e.g. name: \"@org/agent\"")
		}
		return nil, fmt.Errorf("failed to parse spec YAML: %w", err)
	}

	return &spec, nil
}

// ParseString parses the spec content from a string
func ParseString(content string) (*AstroSpec, error) {
	return Parse([]byte(content))
}

// ParseSpec reads and parses an spec file with validation
func ParseSpec(path string) (*AstroSpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	var spec AstroSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		if unquotedAtName.Match(data) {
			return nil, fmt.Errorf("failed to parse spec: the @ character in the name field must be quoted, e.g. name: \"@org/agent\"")
		}
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
	if spec.Agent.Build != nil && spec.Agent.Image != "" {
		return nil, fmt.Errorf("agent: image and build are mutually exclusive")
	}

	// Validate build configs
	if spec.Agent.Build != nil {
		if err := validateBuildConfig("agent.build", spec.Agent.Build); err != nil {
			return nil, err
		}
	}

	// Validate top-level inputs
	for name, input := range spec.Inputs {
		if err := validateInput(fmt.Sprintf("inputs.%s", name), input); err != nil {
			return nil, err
		}
	}

	// Validate agent inputs
	for i, input := range spec.Agent.Inputs {
		if err := validateInput(fmt.Sprintf("agent.inputs[%d]", i), input); err != nil {
			return nil, err
		}
	}

	// Validate custom providers
	validScopeValues := map[string]bool{"models": true, "knowledge": true, "integrations": true}
	for name, provider := range spec.Providers {
		if len(provider.Scope) == 0 {
			return nil, fmt.Errorf("providers.%s: scope is required and must contain at least one of: models, knowledge, integrations", name)
		}
		for _, s := range provider.Scope {
			if !validScopeValues[s] {
				return nil, fmt.Errorf("providers.%s: invalid scope value %q (must be one of: models, knowledge, integrations)", name, s)
			}
		}
		if len(provider.Variables) == 0 {
			return nil, fmt.Errorf("providers.%s: variables is required and must contain at least one entry", name)
		}
		for i, v := range provider.Variables {
			if err := validateInput(fmt.Sprintf("providers.%s.variables[%d]", name, i), v); err != nil {
				return nil, err
			}
		}
	}

	// Validate knowledge entries
	for name, k := range spec.Knowledge {
		if k.Provider != "" && k.Container != nil {
			return nil, fmt.Errorf("knowledge %q: provider and container are mutually exclusive", name)
		}
		if k.Provider == "" && k.Container == nil {
			return nil, fmt.Errorf("knowledge %q: either provider or container is required", name)
		}
		// Validate custom provider scope
		if k.Provider != "" {
			if cp, ok := spec.Providers[k.Provider]; ok {
				if !scopeContains(cp.Scope, "knowledge") {
					return nil, fmt.Errorf("knowledge %q: provider %q does not allow scope %q", name, k.Provider, "knowledge")
				}
			}
		}
		if k.Container != nil && k.Container.Build != nil {
			if err := validateBuildConfig(fmt.Sprintf("knowledge.%s.container.build", name), k.Container.Build); err != nil {
				return nil, err
			}
		}
		if k.Container != nil && k.Container.GPU != nil {
			if k.Container.GPU.Runtime != "" && k.Container.GPU.Runtime != "cuda" && k.Container.GPU.Runtime != "rocm" {
				return nil, fmt.Errorf("knowledge.%s.container.gpu.runtime: must be one of cuda or rocm", name)
			}
		}
		for i, input := range k.Inputs {
			if err := validateInput(fmt.Sprintf("knowledge.%s.inputs[%d]", name, i), input); err != nil {
				return nil, err
			}
		}
	}

	// Validate model entries
	for name, m := range spec.Models {
		if m.Provider != "" && m.Container != nil {
			return nil, fmt.Errorf("model %q: provider and container are mutually exclusive", name)
		}
		if m.Provider == "" && m.Container == nil {
			return nil, fmt.Errorf("model %q: either provider or container is required", name)
		}
		if len(m.Models) > 0 && m.Model != "" {
			return nil, fmt.Errorf("model %q: models and model are mutually exclusive", name)
		}
		// Validate custom provider scope
		if m.Provider != "" {
			if cp, ok := spec.Providers[m.Provider]; ok {
				if !scopeContains(cp.Scope, "models") {
					return nil, fmt.Errorf("model %q: provider %q does not allow scope %q", name, m.Provider, "models")
				}
			}
		}
		if m.Container != nil && m.Container.Build != nil {
			if err := validateBuildConfig(fmt.Sprintf("models.%s.container.build", name), m.Container.Build); err != nil {
				return nil, err
			}
		}
		if m.Container != nil && m.Container.GPU != nil {
			if m.Container.GPU.Runtime != "" && m.Container.GPU.Runtime != "cuda" && m.Container.GPU.Runtime != "rocm" {
				return nil, fmt.Errorf("models.%s.container.gpu.runtime: must be one of cuda or rocm", name)
			}
		}
		for i, input := range m.Inputs {
			if err := validateInput(fmt.Sprintf("models.%s.inputs[%d]", name, i), input); err != nil {
				return nil, err
			}
		}
	}

	// Validate tool entries
	for name, t := range spec.Tools {
		if t.Provider != "" && t.Container != nil {
			return nil, fmt.Errorf("tool %q: provider and container are mutually exclusive", name)
		}
		if t.Provider == "" && t.Container == nil {
			return nil, fmt.Errorf("tool %q: either provider or container is required", name)
		}
		// Validate custom provider scope
		if t.Provider != "" {
			if cp, ok := spec.Providers[t.Provider]; ok {
				if !scopeContains(cp.Scope, "integrations") {
					return nil, fmt.Errorf("tool %q: provider %q does not allow scope %q", name, t.Provider, "integrations")
				}
			}
		}
		if t.Container != nil && t.Container.Build != nil {
			if err := validateBuildConfig(fmt.Sprintf("integrations.%s.container.build", name), t.Container.Build); err != nil {
				return nil, err
			}
		}
		if t.Container != nil && t.Container.GPU != nil {
			if t.Container.GPU.Runtime != "" && t.Container.GPU.Runtime != "cuda" && t.Container.GPU.Runtime != "rocm" {
				return nil, fmt.Errorf("integrations.%s.container.gpu.runtime: must be one of cuda or rocm", name)
			}
		}
		for i, input := range t.Inputs {
			if err := validateInput(fmt.Sprintf("integrations.%s.inputs[%d]", name, i), input); err != nil {
				return nil, err
			}
		}
	}

	// Validate ingestion entries
	validTriggerTypes := map[string]bool{"schedule": true, "startup": true, "manual": true, "webhook": true}
	for name, ing := range spec.Ingestion {
		if !validTriggerTypes[ing.Trigger.Type] {
			return nil, fmt.Errorf("ingestion.%s.trigger.type: must be one of schedule, startup, manual, webhook", name)
		}
		if ing.Container.Build != nil {
			if err := validateBuildConfig(fmt.Sprintf("ingestion.%s.container.build", name), ing.Container.Build); err != nil {
				return nil, err
			}
		}
		for i, input := range ing.Inputs {
			if err := validateInput(fmt.Sprintf("ingestion.%s.inputs[%d]", name, i), input); err != nil {
				return nil, err
			}
		}
	}

	return &spec, nil
}

func validateInput(path string, input Input) error {
	if input.Name == "" {
		return fmt.Errorf("%s: name is required", path)
	}
	if !validDatatypes[input.Datatype] {
		return fmt.Errorf("%s: datatype must be one of string, boolean, number, array, object (got %q)", path, input.Datatype)
	}
	if input.DisplayAs != "" && !validDisplayAs[input.DisplayAs] {
		return fmt.Errorf("%s: display-as must be one of short-text, long-text, select (got %q)", path, input.DisplayAs)
	}
	if input.DisplayAs == "select" && len(input.Options) == 0 {
		return fmt.Errorf("%s: options must be present and non-empty when display-as is select", path)
	}
	return nil
}

func validateBuildConfig(path string, b *BuildConfig) error {
	if b.Context == "" {
		return fmt.Errorf("%s.context is required", path)
	}
	if b.Dockerfile == "" {
		return fmt.Errorf("%s.dockerfile is required", path)
	}
	return nil
}

// SecretDefaultViolations returns the names of all secret inputs that still
// carry a non-empty default value. These must be stripped before registration
// to avoid storing credentials in the registry.
func SecretDefaultViolations(s *AstroSpec) []string {
	var violations []string

	check := func(location, name, def string, secret bool) {
		if secret && def != "" {
			violations = append(violations, location+"."+name)
		}
	}

	for key, inp := range s.Inputs {
		check("inputs."+key, inp.Name, inp.Default, inp.Secret)
	}
	for _, inp := range s.Agent.Inputs {
		check("agent.inputs", inp.Name, inp.Default, inp.Secret)
	}
	for name, m := range s.Models {
		for _, inp := range m.Inputs {
			check("models."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, k := range s.Knowledge {
		for _, inp := range k.Inputs {
			check("knowledge."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, t := range s.Tools {
		for _, inp := range t.Inputs {
			check("tools."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, ing := range s.Ingestion {
		for _, inp := range ing.Inputs {
			check("ingestion."+name+".inputs", inp.Name, inp.Default, inp.Secret)
		}
	}
	for name, prov := range s.Providers {
		for _, v := range prov.Variables {
			check("providers."+name+".variables", v.Name, v.Default, v.Secret)
		}
	}

	return violations
}

func scopeContains(scope []string, value string) bool {
	for _, s := range scope {
		if s == value {
			return true
		}
	}
	return false
}
