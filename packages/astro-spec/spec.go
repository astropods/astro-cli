// Package spec provides shared type definitions for the Astro platform spec (astro.yml).
// This package is used by both astro-cli and astro-server to ensure consistent parsing.
package spec

// AstroSpec represents the complete astro.yml specification
type AstroSpec struct {
	Spec         string                 `json:"spec" yaml:"spec"`
	Agent        string                 `json:"agent" yaml:"agent"`
	Meta         Meta                   `json:"meta" yaml:"meta"`
	Container    Container              `json:"container" yaml:"container"`
	Models       map[string]Model       `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge    map[string]Knowledge   `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Tools        map[string]Tool        `json:"tools,omitempty" yaml:"tools,omitempty"`
	Integrations Integrations           `json:"integrations,omitempty" yaml:"integrations,omitempty"`
	Interfaces   map[string]Interface   `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Ingestion    map[string]Ingestion   `json:"ingestion,omitempty" yaml:"ingestion,omitempty"`
}

type Meta struct {
	Version     string   `json:"version" yaml:"version"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Owner       string   `json:"owner,omitempty" yaml:"owner,omitempty"`
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
	Provider  string          `json:"provider" yaml:"provider"`
	Model     string          `json:"model,omitempty" yaml:"model,omitempty"`
	Config    map[string]any  `json:"config,omitempty" yaml:"config,omitempty"`
	Container ContainerConfig `json:"container" yaml:"container"`
}

type Knowledge struct {
	Type      string          `json:"type" yaml:"type"`
	Provider  string          `json:"provider,omitempty" yaml:"provider,omitempty"`
	Config    map[string]any  `json:"config,omitempty" yaml:"config,omitempty"`
	Embedding string          `json:"embedding,omitempty" yaml:"embedding,omitempty"`
	Container ContainerConfig `json:"container" yaml:"container"`
}

type Tool struct {
	Type      string           `json:"type" yaml:"type"`
	Config    map[string]any   `json:"config,omitempty" yaml:"config,omitempty"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
}

type ContainerConfig struct {
	Image       string            `json:"image,omitempty" yaml:"image,omitempty"`
	Build       *BuildConfig      `json:"build,omitempty" yaml:"build,omitempty"`
	GPU         bool              `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Persistent  bool              `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Port        int               `json:"port,omitempty" yaml:"port,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck      `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

type Integrations struct {
	Models    []IntegrationModel     `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge []IntegrationKnowledge `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Tools     []IntegrationTool      `json:"tools,omitempty" yaml:"tools,omitempty"`
}

type IntegrationEnv struct {
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

type IntegrationModel struct {
	Name     string          `json:"name" yaml:"name"`
	Provider string          `json:"provider" yaml:"provider"`
	Model    string          `json:"model,omitempty" yaml:"model,omitempty"`
	Config   map[string]any  `json:"config,omitempty" yaml:"config,omitempty"`
	Env      *IntegrationEnv `json:"env,omitempty" yaml:"env,omitempty"`
}

type IntegrationKnowledge struct {
	Name      string          `json:"name" yaml:"name"`
	Provider  string          `json:"provider" yaml:"provider"`
	Type      string          `json:"type,omitempty" yaml:"type,omitempty"`
	Config    map[string]any  `json:"config,omitempty" yaml:"config,omitempty"`
	Embedding string          `json:"embedding,omitempty" yaml:"embedding,omitempty"`
	Env       *IntegrationEnv `json:"env,omitempty" yaml:"env,omitempty"`
}

type IntegrationTool struct {
	Name     string          `json:"name" yaml:"name"`
	Provider string          `json:"provider" yaml:"provider"`
	Config   map[string]any  `json:"config,omitempty" yaml:"config,omitempty"`
	Env      *IntegrationEnv `json:"env,omitempty" yaml:"env,omitempty"`
}

type Interface struct {
	Type    string            `json:"type" yaml:"type"`
	Config  map[string]any    `json:"config,omitempty" yaml:"config,omitempty"`
	Service *InterfaceService `json:"service,omitempty" yaml:"service,omitempty"`
}

type InterfaceService struct {
	Name        string            `json:"name" yaml:"name"`
	Build       *BuildConfig      `json:"build,omitempty" yaml:"build,omitempty"`
	Image       string            `json:"image,omitempty" yaml:"image,omitempty"`
	Ports       []string          `json:"ports,omitempty" yaml:"ports,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// Ingestion represents a data ingestion job
// Just a container that runs on a trigger
type Ingestion struct {
	Container ContainerConfig   `json:"container" yaml:"container"`
	Trigger   IngestionTrigger  `json:"trigger" yaml:"trigger"`
}

type IngestionTrigger struct {
	Type     string `json:"type" yaml:"type"`                         // "schedule" | "manual" | "startup" | "webhook"
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty"` // cron expression for schedule type
}
