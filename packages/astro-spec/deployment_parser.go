package spec

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseDeploymentSpec parses a deployment spec from YAML or JSON bytes.
func ParseDeploymentSpec(data []byte) (*AstroDeploymentSpec, error) {
	var ds AstroDeploymentSpec

	// Try YAML first (superset of JSON)
	if err := yaml.Unmarshal(data, &ds); err != nil {
		// Fall back to JSON
		if jsonErr := json.Unmarshal(data, &ds); jsonErr != nil {
			return nil, fmt.Errorf("failed to parse deployment spec: %w", err)
		}
	}

	if err := validateDeploymentSpec(&ds); err != nil {
		return nil, err
	}

	return &ds, nil
}

// SerializeDeploymentSpec serializes a deployment spec to YAML.
func SerializeDeploymentSpec(ds *AstroDeploymentSpec) ([]byte, error) {
	data, err := yaml.Marshal(ds)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize deployment spec: %w", err)
	}
	return data, nil
}

func validateDeploymentSpec(ds *AstroDeploymentSpec) error {
	if ds.Spec != "deployment/v1" {
		return fmt.Errorf("unsupported deployment spec version: %q (expected \"deployment/v1\")", ds.Spec)
	}
	if ds.Source.Name == "" {
		return fmt.Errorf("source.name is required")
	}
	if ds.Source.Build == "" {
		return fmt.Errorf("source.build is required")
	}
	if ds.Source.Registry == "" {
		return fmt.Errorf("source.registry is required")
	}
	if ds.Agent.Image == "" {
		return fmt.Errorf("agent.image is required")
	}
	if ds.Agent.Port == 0 {
		return fmt.Errorf("agent.port is required")
	}

	for name, m := range ds.Models {
		if m.Image == "" {
			return fmt.Errorf("models.%s.image is required", name)
		}
		if m.Port == 0 {
			return fmt.Errorf("models.%s.port is required", name)
		}
	}
	for name, k := range ds.Knowledge {
		if k.Image == "" {
			return fmt.Errorf("knowledge.%s.image is required", name)
		}
		if k.Port == 0 {
			return fmt.Errorf("knowledge.%s.port is required", name)
		}
		if k.Persistent && k.Storage == nil {
			return fmt.Errorf("knowledge.%s: storage is required when persistent is true", name)
		}
	}
	for name, t := range ds.Tools {
		if t.Image == "" {
			return fmt.Errorf("tools.%s.image is required", name)
		}
		if t.Port == 0 {
			return fmt.Errorf("tools.%s.port is required", name)
		}
	}
	for name, ing := range ds.Ingestion {
		if ing.Image == "" {
			return fmt.Errorf("ingestion.%s.image is required", name)
		}
		if ing.Trigger.Type == "" {
			return fmt.Errorf("ingestion.%s.trigger.type is required", name)
		}
		if ing.Trigger.Type == "webhook" && ing.Port == 0 {
			return fmt.Errorf("ingestion.%s.port is required for webhook triggers", name)
		}
	}

	return nil
}

// StripCredentialValues returns a copy of the deployment spec with all credential
// values removed. Used before storing the resolved spec.
func StripCredentialValues(ds *AstroDeploymentSpec) *AstroDeploymentSpec {
	// Shallow copy
	stripped := *ds

	if len(ds.Credentials) > 0 {
		stripped.Credentials = make(map[string]DeploymentCredential, len(ds.Credentials))
		for k, v := range ds.Credentials {
			stripped.Credentials[k] = DeploymentCredential{
				Description: v.Description,
				Optional:    v.Optional,
			}
		}
	}

	return &stripped
}
