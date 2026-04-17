package spec

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseDeploymentSpec parses a deployment spec from YAML or JSON bytes.
// Accepts both "deployment/v1" and "deployment-template/v1" spec versions.
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

var (
	validEndpointProtocols  = map[string]bool{"http": true, "grpc": true, "tcp": true}
	validVariableDatatypes  = map[string]bool{"string": true, "boolean": true, "number": true, "array": true, "object": true}
	validVariableDisplayAs  = map[string]bool{"short-text": true, "long-text": true, "select": true}
	validGPURuntimes        = map[string]bool{"cuda": true, "rocm": true}
	validStorageAccessModes = map[string]bool{"ReadWriteOnce": true, "ReadWriteMany": true}
	validUpdateStrategies   = map[string]bool{"rolling": true, "recreate": true}
	validTriggerTypes       = map[string]bool{"schedule": true, "startup": true, "manual": true, "webhook": true}
)

func validateDeploymentSpec(ds *AstroDeploymentSpec) error {
	isTemplate := ds.Spec == "deployment-template/v1"
	isFulfilled := ds.Spec == "deployment/v1"

	// Rule 1
	if !isTemplate && !isFulfilled {
		return fmt.Errorf("unsupported deployment spec version: %q (expected \"deployment/v1\" or \"deployment-template/v1\")", ds.Spec)
	}

	// Rule 2
	if ds.Source.Name == "" {
		return fmt.Errorf("source.name is required")
	}
	if ds.Source.Build == "" {
		return fmt.Errorf("source.build is required")
	}
	if ds.Source.Registry == "" {
		return fmt.Errorf("source.registry is required")
	}

	// Rule 3: target.runtime must be "kubernetes" when set
	if ds.Target.Runtime != "" && ds.Target.Runtime != "kubernetes" {
		return fmt.Errorf("target.runtime must be \"kubernetes\", got %q", ds.Target.Runtime)
	}

	// Rule 5
	if ds.Agent.Image == "" {
		return fmt.Errorf("agent.image is required")
	}
	if len(ds.Agent.Endpoints) == 0 {
		return fmt.Errorf("agent.endpoints is required (at least one endpoint)")
	}
	if err := validateEndpoints("agent", "", ds.Agent.Endpoints); err != nil {
		return err
	}

	// Rule 18: if agent.distributed is false/absent, replicas must be 1
	if !ds.Agent.Distributed && ds.Agent.Replicas > 1 {
		return fmt.Errorf("agent.replicas must be 1 when agent.distributed is false")
	}

	// Rule 17: agent update strategy
	if err := validateUpdateStrategy("agent", ds.Agent.Update); err != nil {
		return err
	}

	// Rule 6, 6a, 14, 15, 17
	for name, m := range ds.Models {
		if m.Image == "" {
			return fmt.Errorf("models.%s.image is required", name)
		}
		if len(m.Endpoints) == 0 {
			return fmt.Errorf("models.%s.endpoints is required (at least one endpoint)", name)
		}
		if err := validateEndpoints("models", name, m.Endpoints); err != nil {
			return err
		}
		if m.GPU != nil {
			if err := validateGPU(fmt.Sprintf("models.%s", name), m.GPU); err != nil {
				return err
			}
		}
		if err := validateUpdateStrategy(fmt.Sprintf("models.%s", name), m.Update); err != nil {
			return err
		}
	}

	// Rule 6, 6a, 7, 14, 16, 17
	for name, k := range ds.Knowledge {
		if k.Image == "" {
			return fmt.Errorf("knowledge.%s.image is required", name)
		}
		if len(k.Endpoints) == 0 {
			return fmt.Errorf("knowledge.%s.endpoints is required (at least one endpoint)", name)
		}
		if err := validateEndpoints("knowledge", name, k.Endpoints); err != nil {
			return err
		}
		if k.Persistent && k.Storage == nil {
			return fmt.Errorf("knowledge.%s: storage is required when persistent is true", name)
		}
		if k.Storage != nil {
			if err := validateStorage(fmt.Sprintf("knowledge.%s", name), k.Storage); err != nil {
				return err
			}
		}
		if err := validateUpdateStrategy(fmt.Sprintf("knowledge.%s", name), k.Update); err != nil {
			return err
		}
	}

	// Rule 6, 6a, 14, 17
	for name, t := range ds.Integrations {
		if t.Image == "" {
			return fmt.Errorf("integrations.%s.image is required", name)
		}
		if len(t.Endpoints) == 0 {
			return fmt.Errorf("integrations.%s.endpoints is required (at least one endpoint)", name)
		}
		if err := validateEndpoints("tools", name, t.Endpoints); err != nil {
			return err
		}
		if err := validateUpdateStrategy(fmt.Sprintf("integrations.%s", name), t.Update); err != nil {
			return err
		}
	}

	// Rule 8, 9, 10, 14
	for name, ing := range ds.Ingestion {
		if ing.Image == "" {
			return fmt.Errorf("ingestion.%s.image is required", name)
		}
		if ing.Trigger.Type == "" {
			return fmt.Errorf("ingestion.%s.trigger.type is required", name)
		}
		// Rule 8: trigger.type must be a valid value
		if !validTriggerTypes[ing.Trigger.Type] {
			return fmt.Errorf("ingestion.%s.trigger.type %q is invalid (must be schedule, startup, manual, or webhook)", name, ing.Trigger.Type)
		}
		// Rule 9: schedule type needs a schedule
		if ing.Trigger.Type == "webhook" && len(ing.Endpoints) == 0 {
			return fmt.Errorf("ingestion.%s.endpoints is required for webhook triggers", name)
		}
		// Rule 10: non-schedule trigger must not have schedule field
		if ing.Trigger.Type != "schedule" && ing.Trigger.Schedule != "" {
			return fmt.Errorf("ingestion.%s.trigger.schedule must not be set when trigger.type is %q", name, ing.Trigger.Type)
		}
		if len(ing.Endpoints) > 0 {
			if err := validateEndpoints("ingestion", name, ing.Endpoints); err != nil {
				return err
			}
		}
	}

	// Interfaces
	if ds.Interfaces != nil {
		// Rule 11 (fulfilled only): adapters non-empty, image non-empty
		if isFulfilled {
			if len(ds.Interfaces.Adapters) == 0 {
				return fmt.Errorf("interfaces.adapters must not be empty when interfaces is present")
			}
			if ds.Interfaces.Image == "" {
				return fmt.Errorf("interfaces.image is required")
			}
		}
		if len(ds.Interfaces.Endpoints) > 0 {
			if err := validateEndpoints("interfaces", "", ds.Interfaces.Endpoints); err != nil {
				return err
			}
		}
		for _, adapter := range ds.Interfaces.Adapters {
			if adapter == "web" {
				// web adapter requires an http endpoint with expose configured
				ep := ExposedEndpoint(ds.Interfaces.Endpoints)
				httpEp := EndpointByName(ds.Interfaces.Endpoints, "http")
				if ep == nil && (httpEp == nil || httpEp.Port == 0) {
					return fmt.Errorf("interfaces: an exposed http endpoint is required when the web adapter is enabled")
				}
			}
		}
	}

	// Variables
	if isTemplate {
		if err := validateVariablesTemplate(ds); err != nil {
			return err
		}
	}
	if isFulfilled {
		if err := validateVariablesFulfilled(ds); err != nil {
			return err
		}
	}

	// deployment/v1-specific (Rules 20, 21)
	if isFulfilled {
		if len(ds.Editable) > 0 {
			return fmt.Errorf("deployment/v1 must not contain the editable field")
		}
	}

	return nil
}

// validateEndpoints checks endpoint ports, protocol enums, and no duplicate ports (Rule 6, 6a, 14).
func validateEndpoints(section, name string, endpoints map[string]Endpoint) error {
	prefix := section
	if name != "" {
		prefix = section + "." + name
	}
	seen := make(map[int]string, len(endpoints))
	for epName, ep := range endpoints {
		if ep.Port == 0 {
			return fmt.Errorf("%s.endpoints.%s.port is required", prefix, epName)
		}
		// Rule 6a: protocol enum
		if ep.Protocol != "" && !validEndpointProtocols[ep.Protocol] {
			return fmt.Errorf("%s.endpoints.%s.protocol %q is invalid (must be http, grpc, or tcp)", prefix, epName, ep.Protocol)
		}
		// Rule 14: no duplicate ports within same component
		if prev, exists := seen[ep.Port]; exists {
			return fmt.Errorf("%s.endpoints: duplicate port %d on endpoints %q and %q", prefix, ep.Port, prev, epName)
		}
		seen[ep.Port] = epName
	}
	return nil
}

// validateGPU checks GPU runtime enum (Rule 15).
func validateGPU(prefix string, gpu *DeploymentGPU) error {
	if gpu.Runtime != "" && !validGPURuntimes[gpu.Runtime] {
		return fmt.Errorf("%s.gpu.runtime %q is invalid (must be cuda or rocm)", prefix, gpu.Runtime)
	}
	return nil
}

// validateStorage checks storage access_mode enum (Rule 16).
func validateStorage(prefix string, storage *StorageConfig) error {
	if storage.AccessMode != "" && !validStorageAccessModes[storage.AccessMode] {
		return fmt.Errorf("%s.storage.access_mode %q is invalid (must be ReadWriteOnce or ReadWriteMany)", prefix, storage.AccessMode)
	}
	return nil
}

// validateUpdateStrategy checks update strategy enum (Rule 17).
func validateUpdateStrategy(prefix string, update UpdateStrategy) error {
	if update.Strategy != "" && !validUpdateStrategies[update.Strategy] {
		return fmt.Errorf("%s.update.strategy %q is invalid (must be rolling or recreate)", prefix, update.Strategy)
	}
	return nil
}

// validateVariablesTemplate validates variables in deployment-template/v1 (Rules 12b, 12c, 12d).
func validateVariablesTemplate(ds *AstroDeploymentSpec) error {
	for key, v := range ds.Variables {
		// Rule 12c: datatype enum
		if v.Datatype != "" && !validVariableDatatypes[v.Datatype] {
			return fmt.Errorf("variables.%s: datatype %q is invalid (must be string, boolean, number, array, or object)", key, v.Datatype)
		}
		// Rule 12d: display-as enum
		if v.DisplayAs != "" && !validVariableDisplayAs[v.DisplayAs] {
			return fmt.Errorf("variables.%s: display-as %q is invalid (must be short-text, long-text, or select)", key, v.DisplayAs)
		}
		// Rule 12b: select display-as requires options
		if v.DisplayAs == "select" && len(v.Options) == 0 {
			return fmt.Errorf("variables.%s: options is required when display-as is \"select\"", key)
		}
	}
	return nil
}

// validateVariablesFulfilled validates variables in deployment/v1 (Rules 12, 12a, 21).
func validateVariablesFulfilled(ds *AstroDeploymentSpec) error {
	// Build valid target set for Rule 12a
	validTargets := map[string]bool{"agent": true, "ingestion": true}
	for ingName := range ds.Ingestion {
		validTargets["ingestion."+ingName] = true
	}
	if ds.Interfaces != nil {
		for _, adapter := range ds.Interfaces.Adapters {
			validTargets["interface."+adapter] = true
		}
	}

	for key, v := range ds.Variables {
		// Rule 21: no template-only fields
		if v.Default != "" {
			return fmt.Errorf("variables.%s: default field is not allowed in deployment/v1", key)
		}
		if v.Description != "" {
			return fmt.Errorf("variables.%s: description field is not allowed in deployment/v1", key)
		}
		if v.Datatype != "" {
			return fmt.Errorf("variables.%s: datatype field is not allowed in deployment/v1", key)
		}
		if v.DisplayAs != "" {
			return fmt.Errorf("variables.%s: display-as field is not allowed in deployment/v1", key)
		}
		if len(v.Options) > 0 {
			return fmt.Errorf("variables.%s: options field is not allowed in deployment/v1", key)
		}
		// Rule 12: non-optional must have a value or a ref
		if !v.Optional && v.Value == "" && v.Ref == "" {
			return fmt.Errorf("variables.%s.value: required variable has no value", key)
		}
		// Rule 12e: value and ref are mutually exclusive
		if v.Value != "" && v.Ref != "" {
			return fmt.Errorf("variables.%s: cannot set both value and ref", key)
		}
		// Rule 12a: targets must be non-empty with valid values
		if len(v.Targets) == 0 {
			return fmt.Errorf("variables.%s.targets: must not be empty", key)
		}
		for _, target := range v.Targets {
			if !validTargets[target] && !isValidInterfaceTarget(target) {
				return fmt.Errorf("variables.%s.targets: %q is invalid (must be agent, ingestion, ingestion.<name>, or interface.<adapter>)", key, target)
			}
		}
	}
	return nil
}

// isValidInterfaceTarget accepts any "interface.<adapter>" string as valid
// (adapter names are arbitrary; structural check is done elsewhere).
func isValidInterfaceTarget(target string) bool {
	return strings.HasPrefix(target, "interface.") && len(target) > len("interface.")
}

// StripSecretVariableValues returns a copy of the deployment spec with all secret
// variable values removed. Used before storing the resolved spec.
func StripSecretVariableValues(ds *AstroDeploymentSpec) *AstroDeploymentSpec {
	stripped := *ds

	if len(ds.Variables) > 0 {
		stripped.Variables = make(map[string]Variable, len(ds.Variables))
		for k, v := range ds.Variables {
			if v.Secret {
				v.Value = ""
			}
			stripped.Variables[k] = v
		}
	}

	return &stripped
}
