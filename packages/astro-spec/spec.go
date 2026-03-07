// Package spec provides shared type definitions for the Astro platform spec (astropods.yml).
// This package is used by both astro-cli and astro-server to ensure consistent parsing.
package spec

import "strings"

// AstroSpec represents the complete Astro specification
type AstroSpec struct {
	Spec      string                    `json:"spec" yaml:"spec" jsonschema:"description=Spec version. Must be package/v1"`
	Name      string                    `json:"name" yaml:"name" jsonschema:"description=Unique agent name"`
	Meta      Meta                      `json:"meta" yaml:"meta"`
	Agent     Container                 `json:"agent" yaml:"agent" jsonschema:"description=Main agent container"`
	Models    map[string]Model          `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Model sidecar containers"`
	Knowledge map[string]Knowledge      `json:"knowledge,omitempty" yaml:"knowledge,omitempty" jsonschema:"description=Knowledge store containers"`
	Tools     map[string]Tool           `json:"integrations,omitempty" yaml:"integrations,omitempty" jsonschema:"description=Integration sidecar containers"`
	Providers map[string]CustomProvider `json:"providers,omitempty" yaml:"providers,omitempty" jsonschema:"description=Custom provider definitions"`
	Inputs    map[string]Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into every container"`
	Ingestion map[string]Ingestion      `json:"ingestion,omitempty" yaml:"ingestion,omitempty" jsonschema:"description=Data ingestion pipelines"`
	Dev       *Dev                      `json:"dev,omitempty" yaml:"dev,omitempty" jsonschema:"description=Local development overrides"`
}

type Meta struct {
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Visibility  string   `json:"visibility,omitempty" yaml:"visibility,omitempty" jsonschema:"description=Agent visibility: public or private,enum=public,enum=private"`
}

type Container struct {
	Image       string       `json:"image,omitempty" yaml:"image,omitempty"`
	Build       *BuildConfig `json:"build,omitempty" yaml:"build,omitempty"`
	Distributed bool         `json:"distributed,omitempty" yaml:"distributed,omitempty" jsonschema:"description=Whether the agent supports multi-replica deployment"`
	Healthcheck *Healthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Inputs      []Input      `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the agent container"`
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
	Path     string   `json:"path,omitempty" yaml:"path,omitempty"`         // HTTP path for health check (auto-generates test command)
	Interval string   `json:"interval,omitempty" yaml:"interval,omitempty"` // How often to check (default: 10s)
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`   // Time to wait for response (default: 5s)
	Retries  int      `json:"retries,omitempty" yaml:"retries,omitempty"`   // Number of retries before unhealthy (default: 3)
}

// Input declares a user-supplied value prompted at deploy time and injected as an env var.
// The name is used directly as the env var key in the target container.
type Input struct {
	Name        string   `json:"name" yaml:"name" jsonschema:"description=Env var key injected into the target container"`
	Datatype    string   `json:"datatype" yaml:"datatype" jsonschema:"description=Value type,enum=string,enum=boolean,enum=number,enum=array,enum=object"`
	Secret      bool     `json:"secret,omitempty" yaml:"secret,omitempty" jsonschema:"description=If true, stored securely and never logged"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	DisplayAs   string   `json:"display-as,omitempty" yaml:"display-as,omitempty" jsonschema:"description=UI rendering hint,enum=short-text,enum=long-text,enum=select"`
	Options     []string `json:"options,omitempty" yaml:"options,omitempty" jsonschema:"description=Allowed values; required when display-as is select"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
	Optional    bool     `json:"optional,omitempty" yaml:"optional,omitempty" jsonschema:"description=If true, may be omitted at deploy time"`
}

// CustomProvider extends the platform's built-in provider registry.
// It declares the variables it requires so the platform can prompt at deploy time.
type CustomProvider struct {
	Scope     []string       `json:"scope" yaml:"scope" jsonschema:"description=Sections that may reference this provider"`
	Variables []Input        `json:"variables" yaml:"variables" jsonschema:"description=Variables this provider requires from the user"`
	Config    map[string]any `json:"config,omitempty" yaml:"config,omitempty" jsonschema:"description=Provider-specific configuration"`
}

type Model struct {
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Platform-managed provider (e.g. ollama) or custom provider name"`
	Models    []string         `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Model identifiers to make available (e.g. [llama3.2, mistral]). Only meaningful for self-hosted providers."`
	Model     string           `json:"model,omitempty" yaml:"model,omitempty" jsonschema:"description=Deprecated: use models instead. Single model identifier."`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty" jsonschema:"description=Custom container config (alternative to provider)"`
	Inputs    []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the model container"`
}

// ResolvedModels returns the effective list of model identifiers,
// merging the deprecated Model field into Models.
func (m Model) ResolvedModels() []string {
	if len(m.Models) > 0 {
		return m.Models
	}
	if m.Model != "" {
		return []string{m.Model}
	}
	return nil
}

// IsProviderMode returns true when the model entry uses a platform-managed provider.
func (m Model) IsProviderMode() bool {
	return m.Provider != ""
}

// DeploysContainer reports whether this model entry deploys a sidecar container.
// Returns false for cloud providers (credentials only) and custom providers.
func (m Model) DeploysContainer(customProviders map[string]CustomProvider) bool {
	if m.Container != nil {
		return true
	}
	if _, isCustom := customProviders[m.Provider]; isCustom {
		return false
	}
	return m.Provider != "" && !IsCloudModelProvider(m.Provider)
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
	// Inject model names and default env from provider
	models := m.ResolvedModels()
	if len(models) > 0 || len(prov.DefaultEnv) > 0 {
		cc.Environment = make(map[string]string)
		for k, v := range prov.DefaultEnv {
			cc.Environment[k] = v
		}
		if len(models) > 0 && prov.EnvPrefix != "" {
			cc.Environment[prov.EnvPrefix+"_MODEL"] = strings.Join(models, ",")
		}
	}
	return cc
}

type Knowledge struct {
	Provider   string           `json:"provider,omitempty" yaml:"provider,omitempty"`
	Container  *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
	Persistent bool             `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Inputs     []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the knowledge container"`
}

// IsProviderMode returns true when the knowledge entry uses a platform-managed provider.
func (k Knowledge) IsProviderMode() bool {
	return k.Provider != ""
}

// DeploysContainer reports whether this knowledge entry deploys a sidecar container.
func (k Knowledge) DeploysContainer(customProviders map[string]CustomProvider) bool {
	if k.Container != nil {
		return true
	}
	if _, isCustom := customProviders[k.Provider]; isCustom {
		return false
	}
	return k.Provider != "" && !IsCloudKnowledgeProvider(k.Provider)
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
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Platform provider (e.g. github) or custom provider name"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
	Inputs    []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the integration container"`
}

// IsProviderMode returns true when the tool entry uses a cloud provider.
func (t Tool) IsProviderMode() bool {
	return t.Provider != ""
}

// DeploysContainer reports whether this tool entry deploys a sidecar container.
func (t Tool) DeploysContainer(customProviders map[string]CustomProvider) bool {
	if t.Container != nil {
		return true
	}
	if _, isCustom := customProviders[t.Provider]; isCustom {
		return false
	}
	return t.Provider != "" && !IsCloudToolProvider(t.Provider)
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

// Dev provides local development overrides read by `astro dev`.
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

// Ingestion represents a data ingestion job — a container that runs on a trigger.
type Ingestion struct {
	Container ContainerConfig  `json:"container" yaml:"container"`
	Trigger   IngestionTrigger `json:"trigger" yaml:"trigger"`
	Inputs    []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the ingestion container"`
}

type IngestionTrigger struct {
	Type string `json:"type" yaml:"type" jsonschema:"description=When the ingestion runs,enum=schedule,enum=manual,enum=startup,enum=webhook"`
}
