// Package spec provides shared type definitions for the Astro platform spec (astroai.yml).
// This package is used by both astro-cli and astro-server to ensure consistent parsing.
package spec

// AstroSpec represents the complete astroai.yml specification
type AstroSpec struct {
	Spec         string                 `json:"spec" yaml:"spec" jsonschema:"description=Spec version (e.g. astro/v1)"`
	Name         string                 `json:"name" yaml:"name" jsonschema:"description=Unique agent name"`
	Meta         Meta                   `json:"meta" yaml:"meta"`
	Agent        Container              `json:"agent" yaml:"agent" jsonschema:"description=Main agent container"`
	Models       map[string]Model       `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Model sidecar containers"`
	Knowledge    map[string]Knowledge   `json:"knowledge,omitempty" yaml:"knowledge,omitempty" jsonschema:"description=Knowledge store containers"`
	Tools        map[string]Tool        `json:"tools,omitempty" yaml:"tools,omitempty" jsonschema:"description=Tool sidecar containers"`
	Integrations map[string]Integration `json:"integrations,omitempty" yaml:"integrations,omitempty" jsonschema:"description=Cloud integrations"`
	Ingestion    map[string]Ingestion   `json:"ingestion,omitempty" yaml:"ingestion,omitempty" jsonschema:"description=Data ingestion pipelines"`
	Dev          *Dev                   `json:"dev,omitempty" yaml:"dev,omitempty" jsonschema:"description=Local development overrides"`
}

type Meta struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type Container struct {
	Image       string       `json:"image,omitempty" yaml:"image,omitempty"`
	Build       *BuildConfig `json:"build,omitempty" yaml:"build,omitempty"`
	Healthcheck *Healthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

type BuildConfig struct {
	Context    string            `json:"context" yaml:"context"`
	Dockerfile string            `json:"dockerfile" yaml:"dockerfile"`
	Target     string            `json:"target,omitempty" yaml:"target,omitempty"`
	Args       map[string]string `json:"args,omitempty" yaml:"args,omitempty"`
	Secrets    []BuildSecret     `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

type BuildSecret struct {
	ID  string `json:"id" yaml:"id"`
	Env string `json:"env,omitempty" yaml:"env,omitempty"`
}

type Healthcheck struct {
	Test     []string `json:"test,omitempty" yaml:"test,omitempty"`         // Custom health check command (e.g., ["CMD", "redis-cli", "ping"])
	Path     string   `json:"path,omitempty" yaml:"path,omitempty"`         // HTTP path for health check (legacy, auto-generates test command)
	Interval string   `json:"interval,omitempty" yaml:"interval,omitempty"` // How often to check (default: 10s)
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`   // Time to wait for response (default: 5s)
	Retries  int      `json:"retries,omitempty" yaml:"retries,omitempty"`   // Number of retries before unhealthy (default: 3)
}

type Model struct {
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Platform-managed provider (e.g. ollama)"`
	Model     string           `json:"model,omitempty" yaml:"model,omitempty" jsonschema:"description=Provider-specific model name (e.g. llama3.2)"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty" jsonschema:"description=Custom container config (alternative to provider)"`
}

// IsProviderMode returns true when the model entry uses a platform-managed provider.
func (m Model) IsProviderMode() bool {
	return m.Provider != ""
}

// ResolvedContainer returns the effective ContainerConfig — either built from
// the model provider registry (provider mode) or passed through from the user's
// container block (container mode).
func (m Model) ResolvedContainer() ContainerConfig {
	if m.Container != nil {
		return *m.Container
	}
	prov := GetModelProvider(m.Provider)
	cc := ContainerConfig{
		Image: prov.Image,
		Port:  prov.DefaultPort,
	}
	// Inject model name and default env from provider
	if m.Model != "" || len(prov.DefaultEnv) > 0 {
		cc.Environment = make(map[string]string)
		for k, v := range prov.DefaultEnv {
			cc.Environment[k] = v
		}
		if m.Model != "" && prov.EnvPrefix != "" {
			cc.Environment[prov.EnvPrefix+"_MODEL"] = m.Model
		}
	}
	return cc
}

type Knowledge struct {
	Provider   string           `json:"provider,omitempty" yaml:"provider,omitempty"`
	Container  *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
	Persistent bool             `json:"persistent,omitempty" yaml:"persistent,omitempty"`
}

// IsProviderMode returns true when the knowledge entry uses a platform-managed provider.
func (k Knowledge) IsProviderMode() bool {
	return k.Provider != ""
}

// ResolvedContainer returns the effective ContainerConfig — either built from
// the provider registry (provider mode) or passed through from the user's
// container block (container mode). Knowledge.Persistent is merged into the result.
func (k Knowledge) ResolvedContainer() ContainerConfig {
	if k.Container != nil {
		c := *k.Container
		c.Persistent = c.Persistent || k.Persistent
		return c
	}
	// Provider mode: build from registry
	prov := GetProvider(k.Provider)
	return ContainerConfig{
		Image:      prov.Image,
		Port:       prov.DefaultPort,
		Persistent: k.Persistent,
	}
}

type Tool struct {
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Cloud provider (e.g. github)"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
}

// IsProviderMode returns true when the tool entry uses a cloud provider.
func (t Tool) IsProviderMode() bool {
	return t.Provider != ""
}

// GPUConfig is a scheduling hint declaring that a container needs GPU resources.
// VRAM (e.g. "24Gi") tells the server how much GPU memory the workload needs.
// Runtime is "cuda" (default) or "rocm".
type GPUConfig struct {
	VRAM    string `json:"vram,omitempty" yaml:"vram,omitempty" jsonschema:"description=GPU memory required (e.g. 24Gi)"`
	Runtime string `json:"runtime,omitempty" yaml:"runtime,omitempty" jsonschema:"description=GPU runtime,enum=cuda,enum=rocm"`
}

type ContainerConfig struct {
	Image       string            `json:"image,omitempty" yaml:"image,omitempty"`
	Build       *BuildConfig      `json:"build,omitempty" yaml:"build,omitempty"`
	GPU         *GPUConfig        `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Persistent  bool              `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Port        int               `json:"port,omitempty" yaml:"port,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck      `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// HasGPU returns true when the container requires GPU resources.
func (c ContainerConfig) HasGPU() bool {
	return c.GPU != nil
}

type Integration struct {
	Config      map[string]any     `json:"config,omitempty" yaml:"config,omitempty" jsonschema:"description=Provider-specific configuration"`
	Credentials []CustomCredential `json:"credentials,omitempty" yaml:"credentials,omitempty" jsonschema:"description=Credential requirements"`
}

type CustomCredential struct {
	Suffix      string `json:"suffix" yaml:"suffix"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Optional    bool   `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// Dev provides local development overrides read by `astro dev`.
// Interfaces and schedules are deployment concerns; the dev section
// supplies them for local runs so they don't live in the main spec.
type Dev struct {
	Interfaces []string          `json:"interfaces,omitempty" yaml:"interfaces,omitempty" jsonschema:"description=Messaging interfaces to enable locally (e.g. slack)"`
	Schedules  map[string]string `json:"schedules,omitempty" yaml:"schedules,omitempty" jsonschema:"description=Cron schedules for ingestion jobs during dev"`
	Command    string            `json:"command,omitempty" yaml:"command,omitempty" jsonschema:"description=Start command for the agent (default: bun --watch run start)"`
	Overrides  *DevOverrides     `json:"overrides,omitempty" yaml:"overrides,omitempty" jsonschema:"description=Image overrides for local dev services"`
}

// DevOverrides allows overriding default images for local dev services.
type DevOverrides struct {
	MessagingImage  string `json:"messagingImage,omitempty" yaml:"messagingImage,omitempty" jsonschema:"description=Custom image for the messaging sidecar"`
	PlaygroundImage string `json:"playgroundImage,omitempty" yaml:"playgroundImage,omitempty" jsonschema:"description=Custom image for the playground UI"`
}

// Ingestion represents a data ingestion job
// Just a container that runs on a trigger
type Ingestion struct {
	Container ContainerConfig  `json:"container" yaml:"container"`
	Trigger   IngestionTrigger `json:"trigger" yaml:"trigger"`
}

type IngestionTrigger struct {
	Type string `json:"type" yaml:"type" jsonschema:"description=When the ingestion runs,enum=schedule,enum=manual,enum=startup,enum=webhook"`
}
