package spec

import "fmt"

// ComponentKind identifies the spec section a buildable component belongs to.
type ComponentKind string

const (
	ComponentAgent       ComponentKind = "agent"
	ComponentModel       ComponentKind = "model"
	ComponentKnowledge   ComponentKind = "knowledge"
	ComponentIntegration ComponentKind = "integration"
	ComponentIngestion   ComponentKind = "ingestion"
)

// Component represents one buildable unit extracted from an AstroSpec.
// Only components with a build block are included.
type Component struct {
	Kind      ComponentKind // spec section this component belongs to
	Name      string        // map key within the spec section (empty for agent)
	ImageName string        // full image-name segment, e.g. "my-agent-integration-search"
	Build     *BuildConfig  // always non-nil (only buildable components are returned)
}

// Suffix returns a short identifier suitable for K8s job names.
// Examples: "agent", "model-llm", "integration-search".
func (c Component) Suffix() string {
	if c.Kind == ComponentAgent {
		return "agent"
	}
	return string(c.Kind) + "-" + c.Name
}

// CollectComponents returns every component in the spec that has a build block.
// Components without a build block are omitted. The returned list uses canonical
// naming: {agent}-integration-{name} for integrations (matching the spec key),
// {agent}-model-{name} for models, etc.
func CollectComponents(s *AstroSpec, agentName string) []Component {
	var out []Component

	if s.Agent.Build != nil {
		out = append(out, Component{
			Kind:      ComponentAgent,
			ImageName: agentName,
			Build:     s.Agent.Build,
		})
	}

	for name, model := range s.Models {
		if model.Container != nil && model.Container.Build != nil {
			out = append(out, Component{
				Kind:      ComponentModel,
				Name:      name,
				ImageName: fmt.Sprintf("%s-model-%s", agentName, name),
				Build:     model.Container.Build,
			})
		}
	}

	for name, knowledge := range s.Knowledge {
		c := knowledge.ResolvedContainer()
		if c.Build != nil {
			out = append(out, Component{
				Kind:      ComponentKnowledge,
				Name:      name,
				ImageName: fmt.Sprintf("%s-knowledge-%s", agentName, name),
				Build:     c.Build,
			})
		}
	}

	for name, integration := range s.Integrations {
		if integration.Container != nil && integration.Container.Build != nil {
			out = append(out, Component{
				Kind:      ComponentIntegration,
				Name:      name,
				ImageName: fmt.Sprintf("%s-integration-%s", agentName, name),
				Build:     integration.Container.Build,
			})
		}
	}

	for name, ingestion := range s.Ingestion {
		if ingestion.Container.Build != nil {
			out = append(out, Component{
				Kind:      ComponentIngestion,
				Name:      name,
				ImageName: fmt.Sprintf("%s-ingestion-%s", agentName, name),
				Build:     ingestion.Container.Build,
			})
		}
	}

	return out
}

// TransformSpecForRegistry rewrites a raw YAML spec map: it replaces every
// build block with the corresponding image reference and normalizes the name
// field. Only sections that have a build block are transformed; image-only
// sections are left unchanged.
//
// imageRefFn is called with each component's canonical image name and must
// return the fully qualified image reference (e.g. "registry.io/acct/name:tag").
func TransformSpecForRegistry(specObj map[string]any, agentName string, imageRefFn func(imageName string) string) map[string]any {
	if _, ok := specObj["name"].(string); ok {
		specObj["name"] = agentName
	}

	// agent.build → agent.image
	if agent, ok := specObj["agent"].(map[string]any); ok {
		if _, hasBuild := agent["build"]; hasBuild {
			delete(agent, "build")
			agent["image"] = imageRefFn(agentName)
		}
	}

	// models.*.container.build → models.*.container.image
	if models, ok := specObj["models"].(map[string]any); ok {
		for name, data := range models {
			if model, ok := data.(map[string]any); ok {
				if container, ok := model["container"].(map[string]any); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = imageRefFn(fmt.Sprintf("%s-model-%s", agentName, name))
					}
				}
			}
		}
	}

	// knowledge.*.container.build → knowledge.*.container.image
	if knowledge, ok := specObj["knowledge"].(map[string]any); ok {
		for name, data := range knowledge {
			if item, ok := data.(map[string]any); ok {
				if container, ok := item["container"].(map[string]any); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = imageRefFn(fmt.Sprintf("%s-knowledge-%s", agentName, name))
					}
				}
			}
		}
	}

	// integrations.*.container.build → integrations.*.container.image
	if integrations, ok := specObj["integrations"].(map[string]any); ok {
		for name, data := range integrations {
			if item, ok := data.(map[string]any); ok {
				if container, ok := item["container"].(map[string]any); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = imageRefFn(fmt.Sprintf("%s-integration-%s", agentName, name))
					}
				}
			}
		}
	}

	// ingestion.*.container.build → ingestion.*.container.image
	if ingestion, ok := specObj["ingestion"].(map[string]any); ok {
		for name, data := range ingestion {
			if item, ok := data.(map[string]any); ok {
				if container, ok := item["container"].(map[string]any); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = imageRefFn(fmt.Sprintf("%s-ingestion-%s", agentName, name))
					}
				}
			}
		}
	}

	return specObj
}

// StripSecretDefaults removes default values from all secret inputs across the
// raw YAML spec map so that credentials are not stored in the registry.
func StripSecretDefaults(specObj map[string]any) {
	// Top-level inputs (map of input objects)
	if inputs, ok := specObj["inputs"].(map[string]any); ok {
		for _, inputData := range inputs {
			stripSecretInputDefault(inputData)
		}
	}

	// Agent inputs (list)
	if agent, ok := specObj["agent"].(map[string]any); ok {
		stripSecretInputList(agent["inputs"])
	}

	// Models/knowledge/integrations/ingestion — each entry may have an inputs list
	for _, section := range []string{"models", "knowledge", "integrations", "ingestion"} {
		if entries, ok := specObj[section].(map[string]any); ok {
			for _, entryData := range entries {
				if entry, ok := entryData.(map[string]any); ok {
					stripSecretInputList(entry["inputs"])
				}
			}
		}
	}

	// Providers — variables list
	if providers, ok := specObj["providers"].(map[string]any); ok {
		for _, provData := range providers {
			if prov, ok := provData.(map[string]any); ok {
				stripSecretInputList(prov["variables"])
			}
		}
	}
}

// stripSecretInputList strips defaults from a YAML list of inputs ([]any).
func stripSecretInputList(v any) {
	list, ok := v.([]any)
	if !ok {
		return
	}
	for _, item := range list {
		stripSecretInputDefault(item)
	}
}

// stripSecretInputDefault removes the "default" field from a single input map
// if it has secret: true.
func stripSecretInputDefault(v any) {
	input, ok := v.(map[string]any)
	if !ok {
		return
	}
	if secret, _ := input["secret"].(bool); secret {
		delete(input, "default")
	}
}
