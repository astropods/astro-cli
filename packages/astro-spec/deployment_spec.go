package spec

import "sort"

// AstroDeploymentSpec represents the deployment/v1 or deployment-template/v1 specification.
// It is the intermediate artifact between the astro-spec (what the agent is)
// and infrastructure manifests (how it runs on a cluster).
type AstroDeploymentSpec struct {
	Spec          string                         `json:"spec" yaml:"spec"`
	Source        DeploymentSource               `json:"source" yaml:"source"`
	Target        DeploymentTarget               `json:"target" yaml:"target"`
	Agent         DeploymentAgent                `json:"agent" yaml:"agent"`
	Models        map[string]DeploymentModel     `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge     map[string]DeploymentKnowledge `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Tools         map[string]DeploymentTool      `json:"integrations,omitempty" yaml:"integrations,omitempty"`
	Ingestion     map[string]DeploymentIngestion `json:"ingestion,omitempty" yaml:"ingestion,omitempty"`
	Interfaces    *DeploymentInterfaces          `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Variables     map[string]Variable            `json:"variables,omitempty" yaml:"variables,omitempty"`
	Observability DeploymentObservability        `json:"observability" yaml:"observability"`
	Editable      []string                       `json:"editable,omitempty" yaml:"editable,omitempty"`
}

// Endpoint represents a named network endpoint on a component.
type Endpoint struct {
	Port     int             `json:"port" yaml:"port"`
	Protocol string          `json:"protocol,omitempty" yaml:"protocol,omitempty"` // http, grpc, tcp; default http
	Expose   *EndpointExpose `json:"expose,omitempty" yaml:"expose,omitempty"`
}

// EndpointExpose configures external access to an endpoint.
type EndpointExpose struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Domain  string `json:"domain,omitempty" yaml:"domain,omitempty"`
}

// Variable represents a user-fillable or provider-credential variable.
// Template fields (Default, Description, Datatype, DisplayAs, Options) are only
// present in deployment-template/v1 and must be stripped before deployment/v1.
type Variable struct {
	Value       string   `json:"value,omitempty" yaml:"value,omitempty"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
	Targets     []string `json:"targets,omitempty" yaml:"targets,omitempty"`
	Secret      bool     `json:"secret,omitempty" yaml:"secret,omitempty"`
	Optional    bool     `json:"optional,omitempty" yaml:"optional,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Datatype    string   `json:"datatype,omitempty" yaml:"datatype,omitempty"`
	DisplayAs   string   `json:"display-as,omitempty" yaml:"display-as,omitempty"`
	Options     []string `json:"options,omitempty" yaml:"options,omitempty"`
}

// DeploymentSource identifies the agent being deployed.
type DeploymentSource struct {
	Account  string `json:"account" yaml:"account"` // implementation-internal
	Name     string `json:"name" yaml:"name"`
	Build    string `json:"build" yaml:"build"`
	Registry string `json:"registry" yaml:"registry"`
}

// DeploymentTarget describes where to deploy.
type DeploymentTarget struct {
	Runtime      string `json:"runtime" yaml:"runtime"`
	Account      string `json:"account,omitempty" yaml:"account,omitempty"`
	DisplayName  string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty" yaml:"deployment_id,omitempty"`
}

// DeploymentAgent describes the main agent container.
type DeploymentAgent struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Distributed bool                `json:"distributed,omitempty" yaml:"distributed,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
}

// DeploymentModel describes a model sidecar container.
type DeploymentModel struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	GPU         *DeploymentGPU      `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	// implementation-internal fields
	Model      string `json:"model,omitempty" yaml:"model,omitempty"`
	Persistent bool   `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Provider   string `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// DeploymentKnowledge describes a knowledge store container.
type DeploymentKnowledge struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Persistent  bool                `json:"persistent" yaml:"persistent"`
	Volume      string              `json:"volume,omitempty" yaml:"volume,omitempty"` // mount path for persistent storage
	Storage     *StorageConfig      `json:"storage,omitempty" yaml:"storage,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"` // implementation-internal
}

// DeploymentTool describes a tool sidecar container.
type DeploymentTool struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
}

// DeploymentIngestion describes an ingestion job container.
type DeploymentIngestion struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Trigger     DeploymentTrigger   `json:"trigger" yaml:"trigger"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// DeploymentTrigger defines when an ingestion job runs.
type DeploymentTrigger struct {
	Type     string `json:"type" yaml:"type"`
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
}

// DeploymentInterfaces describes the messaging sidecar.
type DeploymentInterfaces struct {
	Adapters    []string            `json:"adapters" yaml:"adapters"`
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// DeploymentObservability controls the observability collector sidecar.
type DeploymentObservability struct {
	Enabled     bool                `json:"enabled" yaml:"enabled"`
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"`
	Image       string              `json:"image,omitempty" yaml:"image,omitempty"`         // implementation-internal
	Port        int                 `json:"port,omitempty" yaml:"port,omitempty"`           // implementation-internal
	Resources   DeploymentResources `json:"resources,omitempty" yaml:"resources,omitempty"` // implementation-internal
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	LogStream   string              `json:"log_stream,omitempty" yaml:"log_stream,omitempty"`
}

// DeploymentResources specifies CPU and memory requests/limits.
type DeploymentResources struct {
	CPU         string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory      string `json:"memory,omitempty" yaml:"memory,omitempty"`
	CPULimit    string `json:"cpu_limit,omitempty" yaml:"cpu_limit,omitempty"`
	MemoryLimit string `json:"memory_limit,omitempty" yaml:"memory_limit,omitempty"`
}

// DeploymentGPU specifies GPU requirements for a component.
type DeploymentGPU struct {
	VRAM    string `json:"vram,omitempty" yaml:"vram,omitempty"`
	Runtime string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Count   int    `json:"count,omitempty" yaml:"count,omitempty"`
}

// StorageConfig specifies PVC configuration for persistent components.
type StorageConfig struct {
	Size       string `json:"size" yaml:"size"`
	Class      string `json:"class,omitempty" yaml:"class,omitempty"`
	AccessMode string `json:"access_mode" yaml:"access_mode"`
}

// UpdateStrategy controls how changes are rolled out.
type UpdateStrategy struct {
	Strategy       string `json:"strategy" yaml:"strategy"`
	MaxUnavailable string `json:"max_unavailable,omitempty" yaml:"max_unavailable,omitempty"`
	MaxSurge       string `json:"max_surge,omitempty" yaml:"max_surge,omitempty"`
}

// Resource defaults by component tier.
var (
	StandardResources = DeploymentResources{
		CPU: "100m", Memory: "256Mi",
		CPULimit: "1", MemoryLimit: "1Gi",
	}
	GPUResources = DeploymentResources{
		CPU: "2", Memory: "8Gi",
		CPULimit: "4", MemoryLimit: "16Gi",
	}
	MessagingResources = DeploymentResources{
		CPU: "100m", Memory: "128Mi",
		CPULimit: "500m", MemoryLimit: "512Mi",
	}
	CollectorResources = DeploymentResources{
		CPU: "50m", Memory: "128Mi",
		CPULimit: "250m", MemoryLimit: "256Mi",
	}
)

// DefaultUpdateStrategy returns the default rolling update strategy.
func DefaultUpdateStrategy() UpdateStrategy {
	return UpdateStrategy{
		Strategy:       "rolling",
		MaxUnavailable: "25%",
		MaxSurge:       "25%",
	}
}

// DefaultStorageConfig returns the default PVC configuration.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Size:       "10Gi",
		AccessMode: "ReadWriteOnce",
	}
}

// PrimaryPort returns the primary port for a component's endpoints map.
// Prefers the "http" endpoint; otherwise returns the port of the first endpoint
// sorted alphabetically. Returns 0 if endpoints is nil or empty.
func PrimaryPort(endpoints map[string]Endpoint) int {
	if len(endpoints) == 0 {
		return 0
	}
	if ep, ok := endpoints["http"]; ok {
		return ep.Port
	}
	names := make([]string, 0, len(endpoints))
	for name := range endpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	return endpoints[names[0]].Port
}

// EndpointByName returns the named endpoint or nil if not found.
func EndpointByName(endpoints map[string]Endpoint, name string) *Endpoint {
	if ep, ok := endpoints[name]; ok {
		return &ep
	}
	return nil
}

// ExposedEndpoint returns the first endpoint that has expose.enabled=true.
// Prefers "http", then checks alphabetically.
func ExposedEndpoint(endpoints map[string]Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	if ep, ok := endpoints["http"]; ok && ep.Expose != nil && ep.Expose.Enabled {
		return &ep
	}
	names := make([]string, 0, len(endpoints))
	for name := range endpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ep := endpoints[name]
		if ep.Expose != nil && ep.Expose.Enabled {
			return &ep
		}
	}
	return nil
}

// SingleEndpoint builds an endpoints map with one entry.
func SingleEndpoint(name string, port int, protocol string) map[string]Endpoint {
	if port == 0 {
		port = 8080
	}
	if name == "" {
		name = "http"
	}
	return map[string]Endpoint{
		name: {Port: port, Protocol: protocol},
	}
}
