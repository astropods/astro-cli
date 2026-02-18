package spec

// AstroDeploymentSpec represents the deployment/v1 specification.
// It is the intermediate artifact between the astro-spec (what the agent is)
// and infrastructure manifests (how it runs on a cluster).
type AstroDeploymentSpec struct {
	Spec          string                           `json:"spec" yaml:"spec"`
	Source        DeploymentSource                 `json:"source" yaml:"source"`
	Target        DeploymentTarget                 `json:"target" yaml:"target"`
	Agent         DeploymentAgent                  `json:"agent" yaml:"agent"`
	Models        map[string]DeploymentModel       `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge     map[string]DeploymentKnowledge   `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Tools         map[string]DeploymentTool        `json:"tools,omitempty" yaml:"tools,omitempty"`
	Ingestion     map[string]DeploymentIngestion   `json:"ingestion,omitempty" yaml:"ingestion,omitempty"`
	Interfaces    *DeploymentInterfaces            `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Credentials   map[string]DeploymentCredential  `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Observability DeploymentObservability           `json:"observability" yaml:"observability"`
	Editable      []string                         `json:"editable,omitempty" yaml:"editable,omitempty"`
}

// DeploymentSource identifies the agent being deployed.
type DeploymentSource struct {
	Account  string `json:"account" yaml:"account"`
	Name     string `json:"name" yaml:"name"`
	Build    string `json:"build" yaml:"build"`
	Registry string `json:"registry" yaml:"registry"`
}

// DeploymentTarget describes where to deploy.
type DeploymentTarget struct {
	Runtime   string `json:"runtime" yaml:"runtime"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

// DeploymentAgent describes the main agent container.
type DeploymentAgent struct {
	Image       string              `json:"image" yaml:"image"`
	Port        int                 `json:"port" yaml:"port"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	Expose      ExposeConfig        `json:"expose" yaml:"expose"`
}

// DeploymentModel describes a model sidecar container.
type DeploymentModel struct {
	Image       string              `json:"image" yaml:"image"`
	Port        int                 `json:"port" yaml:"port"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	GPU         *DeploymentGPU      `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	ModelName   string              `json:"model_name,omitempty" yaml:"model_name,omitempty"` // Provider-specific model to pull (e.g., "llama3.2")
	Persistent  bool                `json:"persistent,omitempty" yaml:"persistent,omitempty"` // Whether model storage needs persistence (PVC)
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"`     // Provider type (e.g., "ollama")
}

// DeploymentKnowledge describes a knowledge store container.
type DeploymentKnowledge struct {
	Image       string              `json:"image" yaml:"image"`
	Port        int                 `json:"port" yaml:"port"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Persistent  bool                `json:"persistent" yaml:"persistent"`
	Storage     *StorageConfig      `json:"storage,omitempty" yaml:"storage,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// DeploymentTool describes a tool sidecar container.
type DeploymentTool struct {
	Image       string              `json:"image" yaml:"image"`
	Port        int                 `json:"port" yaml:"port"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
}

// DeploymentIngestion describes an ingestion job container.
type DeploymentIngestion struct {
	Image       string              `json:"image" yaml:"image"`
	Port        int                 `json:"port,omitempty" yaml:"port,omitempty"`
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
	Port        int                 `json:"port" yaml:"port"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck        `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Expose      ExposeConfig        `json:"expose" yaml:"expose"`
}

// DeploymentCredential holds a single credential entry.
type DeploymentCredential struct {
	Value       string `json:"value" yaml:"value"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Optional    bool   `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// DeploymentObservability controls the observability collector sidecar.
type DeploymentObservability struct {
	Enabled     bool                `json:"enabled" yaml:"enabled"`
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"`
	Image       string              `json:"image,omitempty" yaml:"image,omitempty"`
	Port        int                 `json:"port,omitempty" yaml:"port,omitempty"`
	Resources   DeploymentResources `json:"resources,omitempty" yaml:"resources,omitempty"`
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

// ExposeConfig controls external access to a component.
type ExposeConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Domain  string `json:"domain,omitempty" yaml:"domain,omitempty"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
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
