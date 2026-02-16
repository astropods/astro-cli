// Package spec provides shared type definitions for the Astro platform spec (astro.yml).
// This package is used by both astro-cli and astro-server to ensure consistent parsing.
package spec

// AstroSpec represents the complete astro.yml specification
type AstroSpec struct {
	Spec         string                 `json:"spec" yaml:"spec"`
	Name         string                 `json:"name" yaml:"name"`
	Meta         Meta                   `json:"meta" yaml:"meta"`
	Agent        Container              `json:"agent" yaml:"agent"`
	Models       map[string]Model       `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge    map[string]Knowledge   `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Tools        map[string]Tool        `json:"tools,omitempty" yaml:"tools,omitempty"`
	Integrations map[string]Integration `json:"integrations,omitempty" yaml:"integrations,omitempty"`
	Ingestion    map[string]Ingestion   `json:"ingestion,omitempty" yaml:"ingestion,omitempty"`
	Dev          *Dev                   `json:"dev,omitempty" yaml:"dev,omitempty"`
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
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
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
	return ContainerConfig{
		Image: prov.Image,
		Port:  prov.DefaultPort,
	}
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
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
}

// GPUConfig is a scheduling hint declaring that a container needs GPU resources.
// VRAM (e.g. "24Gi") tells the server how much GPU memory the workload needs.
// Runtime is "cuda" (default) or "rocm".
type GPUConfig struct {
	VRAM    string `json:"vram,omitempty" yaml:"vram,omitempty"`
	Runtime string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
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
	Provider    string             `json:"provider" yaml:"provider"`
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Config      map[string]any     `json:"config,omitempty" yaml:"config,omitempty"`
	Credentials []CustomCredential `json:"credentials,omitempty" yaml:"credentials,omitempty"`
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
	Interfaces []string          `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Schedules  map[string]string `json:"schedules,omitempty" yaml:"schedules,omitempty"`
}

// Ingestion represents a data ingestion job
// Just a container that runs on a trigger
type Ingestion struct {
	Container ContainerConfig   `json:"container" yaml:"container"`
	Trigger   IngestionTrigger  `json:"trigger" yaml:"trigger"`
}

type IngestionTrigger struct {
	Type string `json:"type" yaml:"type"` // "schedule" | "manual" | "startup" | "webhook"
}
