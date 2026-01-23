package spec

// AstroSpec represents the complete astro.yml specification
type AstroSpec struct {
	Spec         string                  `yaml:"spec"`
	Agent        string                  `yaml:"agent"`
	Meta         Meta                    `yaml:"meta"`
	Container    Container               `yaml:"container"`
	Models       map[string]Model        `yaml:"models"`
	Knowledge    map[string]Knowledge    `yaml:"knowledge"`
	Tools        map[string]Tool         `yaml:"tools"`
	Integrations Integrations            `yaml:"integrations"`
	Interfaces   map[string]Interface    `yaml:"interfaces"`
	Injections   map[string]Injection    `yaml:"injections"`
}

type Meta struct {
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Owner       string   `yaml:"owner"`
}

type Container struct {
	Image        string       `yaml:"image,omitempty"`
	Build        *BuildConfig `yaml:"build,omitempty"`
	Healthcheck  *Healthcheck `yaml:"healthcheck,omitempty"`
}

type BuildConfig struct {
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	Target     string            `yaml:"target,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
}

type Healthcheck struct {
	Path     string `yaml:"path"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
}

type Model struct {
	Provider  string            `yaml:"provider"`
	Model     string            `yaml:"model"`
	Config    map[string]any    `yaml:"config,omitempty"`
	Container ContainerConfig   `yaml:"container"`
}

type Knowledge struct {
	Type      string            `yaml:"type"`
	Provider  string            `yaml:"provider"`
	Config    map[string]any    `yaml:"config,omitempty"`
	Embedding string            `yaml:"embedding,omitempty"`
	Container ContainerConfig   `yaml:"container"`
}

type Tool struct {
	Type      string            `yaml:"type"`
	Config    map[string]any    `yaml:"config,omitempty"`
	Container *ContainerConfig  `yaml:"container,omitempty"`
}

type ContainerConfig struct {
	Image      string       `yaml:"image,omitempty"`
	Build      *BuildConfig `yaml:"build,omitempty"`
	GPU        bool         `yaml:"gpu,omitempty"`
	Persistent bool         `yaml:"persistent,omitempty"`
	Port       int          `yaml:"port,omitempty"`
}

type Integrations struct {
	Models    map[string]IntegrationModel    `yaml:"models,omitempty"`
	Knowledge map[string]IntegrationKnowledge `yaml:"knowledge,omitempty"`
	Tools     map[string]IntegrationTool     `yaml:"tools,omitempty"`
}

type IntegrationModel struct {
	Provider string         `yaml:"provider"`
	Model    string         `yaml:"model,omitempty"`
	Config   map[string]any `yaml:"config,omitempty"`
}

type IntegrationKnowledge struct {
	Provider  string         `yaml:"provider"`
	Type      string         `yaml:"type,omitempty"`
	Config    map[string]any `yaml:"config,omitempty"`
	Embedding string         `yaml:"embedding,omitempty"`
}

type IntegrationTool struct {
	Provider string   `yaml:"provider"`
	Scopes   []string `yaml:"scopes,omitempty"`
	Config   map[string]any `yaml:"config,omitempty"`
}

type Interface struct {
	Type    string              `yaml:"type"`
	Config  map[string]any      `yaml:"config,omitempty"`
	Service *InterfaceService   `yaml:"service,omitempty"`
}

type InterfaceService struct {
	Name        string            `yaml:"name"`
	Build       *BuildConfig      `yaml:"build,omitempty"`
	Image       string            `yaml:"image,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

type Injection struct {
	Source     InjectionSource   `yaml:"source"`
	Trigger    InjectionTrigger  `yaml:"trigger"`
	Pipeline   []PipelineStep    `yaml:"pipeline"`
	Persistent bool              `yaml:"persistent,omitempty"`
}

type InjectionSource struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}

type InjectionTrigger struct {
	Type  string `yaml:"type"`
	Cron  string `yaml:"cron,omitempty"`
	Event string `yaml:"event,omitempty"`
}

type PipelineStep struct {
	Step   string         `yaml:"step"`
	Model  string         `yaml:"model,omitempty"`
	Target string         `yaml:"target,omitempty"`
	Config map[string]any `yaml:"config,omitempty"`
}
