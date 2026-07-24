// Package spec provides shared type definitions for the Astro platform spec (astropods.yml).
// This package is used by both astro-cli and astro-server to ensure consistent parsing.
package spec

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// AstroSpec represents the complete Astro specification
type AstroSpec struct {
	Spec         string                    `json:"spec" yaml:"spec" jsonschema:"description=Spec version. Must be package/v1"`
	Name         string                    `json:"name" yaml:"name" jsonschema:"description=Unique agent name"`
	Meta         Meta                      `json:"meta,omitempty" yaml:"meta,omitempty"`
	Agent        Container                 `json:"agent" yaml:"agent" jsonschema:"description=Main agent container"`
	Models       map[string]Model          `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Model sidecar containers"`
	Knowledge    map[string]Knowledge      `json:"knowledge,omitempty" yaml:"knowledge,omitempty" jsonschema:"description=Knowledge store containers"`
	Integrations map[string]Integration    `json:"integrations,omitempty" yaml:"integrations,omitempty" jsonschema:"description=Integration sidecar containers"`
	Providers    map[string]CustomProvider `json:"providers,omitempty" yaml:"providers,omitempty" jsonschema:"description=Custom provider definitions"`
	Inputs       map[string]Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into every container"`
	Ingestion    map[string]Ingestion      `json:"ingestion,omitempty" yaml:"ingestion,omitempty" jsonschema:"description=Data ingestion pipelines"`
	Dev          *Dev                      `json:"dev,omitempty" yaml:"dev,omitempty" jsonschema:"description=Local development overrides"`
}

type Meta struct {
	Visibility string `json:"visibility,omitempty" yaml:"visibility,omitempty" jsonschema:"description=Agent visibility: public or private,enum=public,enum=private"`
}

type Container struct {
	Image       string       `json:"image,omitempty" yaml:"image,omitempty"`
	Build       *BuildConfig `json:"build,omitempty" yaml:"build,omitempty"`
	Distributed bool         `json:"distributed,omitempty" yaml:"distributed,omitempty" jsonschema:"description=Whether the agent supports multi-replica deployment"`
	Interfaces  *Interfaces  `json:"interfaces,omitempty" yaml:"interfaces,omitempty" jsonschema:"description=Agent capabilities: frontend and/or messaging"`
	Healthcheck *Healthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Inputs      []Input      `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the agent container"`
	// AIGateway opts the agent into the Astro AI Gateway. DEPRECATED at the spec
	// level in favor of a model with `provider: gateway` — still honored (injects
	// ASTRO_GATEWAY_URL + ASTRO_GATEWAY_API_KEY, agent picks the model at call
	// time), but a gateway model entry also lets the deployer pick the model at
	// deploy time. The boolean and a gateway model entry are mutually exclusive.
	// (Not marked //Deprecated: so internal backward-compat reads don't trip SA1019.)
	AIGateway bool `json:"astro_ai_gateway,omitempty" yaml:"astro_ai_gateway,omitempty" jsonschema:"description=Deprecated: use a model with provider: gateway. Enables Astro AI Gateway access (injects ASTRO_GATEWAY_URL and ASTRO_GATEWAY_API_KEY)"`
}

// Interfaces declares agent interface capabilities.
// When nil (omitted), the platform defaults to messaging enabled.
// When present, both fields default to false.
type Interfaces struct {
	Frontend  bool `json:"frontend,omitempty" yaml:"frontend,omitempty" jsonschema:"description=Agent serves its own web interface on port 80"`
	Messaging bool `json:"messaging,omitempty" yaml:"messaging,omitempty" jsonschema:"description=Agent supports the messaging protocol"`
}

// HasFrontend reports whether the agent serves its own web frontend.
func (c Container) HasFrontend() bool {
	return c.Interfaces != nil && c.Interfaces.Frontend
}

// HasMessaging reports whether the agent supports the messaging protocol.
// Returns true when interfaces is omitted (backward compat).
func (c Container) HasMessaging() bool {
	if c.Interfaces == nil {
		return true
	}
	return c.Interfaces.Messaging
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
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Cloud model provider (e.g. anthropic/openai) or custom provider name"`
	Models    []string         `json:"models,omitempty" yaml:"models,omitempty" jsonschema:"description=Model identifiers to make available"`
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

// IsGateway reports whether this model routes through the Astro AI Gateway.
func (m Model) IsGateway() bool {
	return IsGatewayModelProvider(m.Provider)
}

// UsesGateway reports whether the agent uses the Astro AI Gateway — either via
// the deprecated agent.astro_ai_gateway boolean or a model with provider: gateway.
func (s *AstroSpec) UsesGateway() bool {
	if s.Agent.AIGateway {
		return true
	}
	for _, m := range s.Models {
		if m.IsGateway() {
			return true
		}
	}
	return false
}

// DeploysContainer reports whether this model entry deploys a sidecar container.
// Only container-mode models deploy a sidecar; provider-mode models are cloud
// (credentials only) or custom (no container).
func (m Model) DeploysContainer(customProviders map[string]CustomProvider) bool {
	return m.Container != nil
}

// ResolvedContainer returns the effective ContainerConfig for a container-mode
// model. Provider-mode models never deploy a container, so this returns a zero
// value for them.
func (m Model) ResolvedContainer() ContainerConfig {
	if m.Container != nil {
		return *m.Container
	}
	return ContainerConfig{}
}

type Knowledge struct {
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
	Inputs    []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the knowledge container"`
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
// container block (container mode). Persistent is derived from Volume so the
// invariant Persistent ⇔ Volume != "" holds in both modes.
func (k Knowledge) ResolvedContainer() ContainerConfig {
	if k.Container != nil {
		c := *k.Container
		c.Persistent = c.Volume != ""
		return c
	}
	prov := GetProvider(k.Provider)
	return ContainerConfig{
		Image:      prov.Image,
		Port:       prov.DefaultPort,
		Volume:     prov.MountPath,
		Persistent: prov.MountPath != "",
	}
}

type Integration struct {
	Provider  string           `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"description=Platform provider (e.g. github) or custom provider name"`
	Container *ContainerConfig `json:"container,omitempty" yaml:"container,omitempty"`
	Inputs    []Input          `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=User-supplied inputs injected into the integration container"`
}

// IsProviderMode returns true when the integration entry uses a cloud provider.
func (t Integration) IsProviderMode() bool {
	return t.Provider != ""
}

// DeploysContainer reports whether this tool entry deploys a sidecar container.
func (t Integration) DeploysContainer(customProviders map[string]CustomProvider) bool {
	if t.Container != nil {
		return true
	}
	if _, isCustom := customProviders[t.Provider]; isCustom {
		return false
	}
	return t.Provider != "" && !IsCloudIntegrationProvider(t.Provider)
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
	Persistent  bool              `json:"-" yaml:"-"` // derived; set by ResolvedContainer
	Port        int               `json:"port,omitempty" yaml:"port,omitempty"`
	Volume      string            `json:"volume,omitempty" yaml:"volume,omitempty" jsonschema:"description=Mount path for persistent data volume inside the container"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *Healthcheck      `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// HasGPU returns true when the container requires GPU resources.
func (c ContainerConfig) HasGPU() bool {
	return c.GPU != nil
}

// Dev provides local development overrides read by `astro dev`.
type Dev struct {
	Interfaces *DevInterfaces    `json:"interfaces,omitempty" yaml:"interfaces,omitempty" jsonschema:"description=Local dev configuration for frontend and messaging interfaces"`
	Schedules  map[string]string `json:"schedules,omitempty" yaml:"schedules,omitempty" jsonschema:"description=Cron schedules for ingestion jobs during dev"`
	Command    string            `json:"command,omitempty" yaml:"command,omitempty" jsonschema:"description=Start command for the agent (default: bun --watch run start)"`
	Overrides  *DevOverrides     `json:"overrides,omitempty" yaml:"overrides,omitempty" jsonschema:"description=Image overrides for local dev services"`
}

// DevInterfaces configures interfaces for local development.
// Supports both the legacy format (string array: [slack, web]) and the
// structured format ({frontend: {port: 3000}, messaging: {adapters: [slack]}}).
// The legacy format is treated as messaging.adapters.
type DevInterfaces struct {
	Frontend  *DevFrontend  `json:"frontend,omitempty" yaml:"frontend,omitempty" jsonschema:"description=Local dev configuration for the agent frontend"`
	Messaging *DevMessaging `json:"messaging,omitempty" yaml:"messaging,omitempty" jsonschema:"description=Local dev configuration for messaging"`
}

// JSONSchema returns a schema that accepts both the legacy string-array format
// (["web", "slack"]) and the structured object format ({frontend: ..., messaging: ...}).
func (DevInterfaces) JSONSchema() *jsonschema.Schema {
	portProps := jsonschema.NewProperties()
	portProps.Set("port", &jsonschema.Schema{
		Type:        "integer",
		Description: "Port the agent serves on locally. Platform proxies port 80 to this. Default: 80",
	})

	slackConfigProps := jsonschema.NewProperties()
	slackConfigProps.Set("actionable_reactions", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Emoji names that trigger agent behavior (e.g. ticket). When omitted no reactions are forwarded.",
	})
	slackConfigProps.Set("allowed_channel_ids", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Slack channel IDs allowed to interact with the agent. Empty means allow all channels.",
	})
	slackConfigProps.Set("allowed_user_ids", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Slack user IDs allowed to interact with the agent. Empty means allow all users.",
	})
	slackConfigProps.Set("observe_channel_ids", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Slack channel IDs whose non-mention messages are forwarded to the agent instead of dropped.",
	})
	slackConfigProps.Set("socket_mode", &jsonschema.Schema{
		Type:        "boolean",
		Description: "Use Slack Socket Mode for real-time events. Default: true",
	})
	slackConfigProps.Set("auto_thread", &jsonschema.Schema{
		Type:        "boolean",
		Description: "Automatically thread bot replies. Default: true",
	})

	messagingProps := jsonschema.NewProperties()
	messagingProps.Set("adapters", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Messaging adapters to enable locally (e.g. slack)",
	})
	messagingProps.Set("slack", &jsonschema.Schema{
		Type:                 "object",
		Properties:           slackConfigProps,
		AdditionalProperties: jsonschema.FalseSchema,
		Description:          "Slack-specific adapter configuration",
	})

	structuredProps := jsonschema.NewProperties()
	structuredProps.Set("frontend", &jsonschema.Schema{
		Type:                 "object",
		Properties:           portProps,
		AdditionalProperties: jsonschema.FalseSchema,
		Description:          "Local dev configuration for the agent frontend",
	})
	structuredProps.Set("messaging", &jsonschema.Schema{
		Type:                 "object",
		Properties:           messagingProps,
		AdditionalProperties: jsonschema.FalseSchema,
		Description:          "Local dev configuration for messaging",
	})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:        "array",
				Items:       &jsonschema.Schema{Type: "string"},
				Description: "Legacy format: list of adapter names (treated as messaging.adapters)",
			},
			{
				Type:                 "object",
				Properties:           structuredProps,
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
		Description: "Local dev configuration for frontend and messaging interfaces",
	}
}

// UnmarshalYAML supports the legacy string-array format for backward compatibility.
func (d *DevInterfaces) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try legacy format first: [slack, web]
	var legacy []string
	if err := unmarshal(&legacy); err == nil {
		d.Messaging = &DevMessaging{Adapters: legacy}
		return nil
	}
	// Structured format
	type devInterfacesAlias DevInterfaces
	var alias devInterfacesAlias
	if err := unmarshal(&alias); err != nil {
		return err
	}
	*d = DevInterfaces(alias)
	return nil
}

// UnmarshalJSON supports the legacy string-array format for backward compatibility.
// This mirrors UnmarshalYAML so that specs stored as JSON (e.g. in the agent index)
// can be deserialized back into AstroSpec correctly.
func (d *DevInterfaces) UnmarshalJSON(data []byte) error {
	// Try legacy format first: ["slack", "web"]
	var legacy []string
	if err := json.Unmarshal(data, &legacy); err == nil {
		d.Messaging = &DevMessaging{Adapters: legacy}
		return nil
	}
	// Structured format
	type devInterfacesAlias DevInterfaces
	var alias devInterfacesAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*d = DevInterfaces(alias)
	return nil
}

// DevFrontend configures the agent's frontend for local development.
type DevFrontend struct {
	Port int `json:"port,omitempty" yaml:"port,omitempty" jsonschema:"description=Port the agent serves on locally. Platform proxies port 80 to this. Default: 80"`
}

// DevMessaging configures the messaging sidecar for local development.
type DevMessaging struct {
	Adapters []string            `json:"adapters,omitempty" yaml:"adapters,omitempty" jsonschema:"description=Messaging adapters to enable locally (e.g. slack)"`
	Slack    *SlackAdapterConfig `json:"slack,omitempty" yaml:"slack,omitempty" jsonschema:"description=Slack-specific adapter configuration"`
	LogLevel string              `json:"log_level,omitempty" yaml:"log_level,omitempty" jsonschema:"description=Log level for the messaging sidecar. One of debug|info|warn|error. Default: info"`
}

// SlackAdapterConfig holds behavioral settings for the Slack messaging adapter.
// Shared between the dev (compose builder) and deployment (template generator) paths.
// Serialized as JSON into the SLACK_CONFIG env var for the messaging sidecar.
//
// Extra captures any keys present under dev.interfaces.messaging.slack that
// aren't named fields above, so new messaging-sidecar options can be passed
// through without changes here.
type SlackAdapterConfig struct {
	ActionableReactions []string       `json:"actionable_reactions,omitempty" yaml:"actionable_reactions,omitempty" jsonschema:"description=Emoji names that trigger agent behavior (e.g. ticket). When omitted no reactions are forwarded."`
	AllowedChannelIDs   []string       `json:"allowed_channel_ids,omitempty" yaml:"allowed_channel_ids,omitempty" jsonschema:"description=Slack channel IDs allowed to interact with the agent. Empty means allow all channels."`
	AllowedUserIDs      []string       `json:"allowed_user_ids,omitempty" yaml:"allowed_user_ids,omitempty" jsonschema:"description=Slack user IDs allowed to interact with the agent. Empty means allow all users."`
	ObserveChannelIDs   []string       `json:"observe_channel_ids,omitempty" yaml:"observe_channel_ids,omitempty" jsonschema:"description=Slack channel IDs whose non-mention messages are forwarded to the agent instead of dropped."`
	SocketMode          *bool          `json:"socket_mode,omitempty" yaml:"socket_mode,omitempty" jsonschema:"description=Use Slack Socket Mode for real-time events. Default: true"`
	AutoThread          *bool          `json:"auto_thread,omitempty" yaml:"auto_thread,omitempty" jsonschema:"description=Automatically thread bot replies. Default: true"`
	Extra               map[string]any `json:"-" yaml:",inline" jsonschema:"-"`
}

var slackAdapterKnownKeys = map[string]struct{}{
	"actionable_reactions": {},
	"allowed_channel_ids":  {},
	"allowed_user_ids":     {},
	"observe_channel_ids":  {},
	"socket_mode":          {},
	"auto_thread":          {},
}

// MarshalJSON flattens Extra into the top-level JSON object alongside the
// named fields. Named fields win on key conflict.
func (s SlackAdapterConfig) MarshalJSON() ([]byte, error) {
	type alias SlackAdapterConfig
	known, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extra) == 0 {
		return known, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		merged = make(map[string]json.RawMessage, len(s.Extra))
	}
	for k, v := range s.Extra {
		if _, exists := merged[k]; exists {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		merged[k] = raw
	}
	return json.Marshal(merged)
}

// UnmarshalJSON pulls out the named fields and routes any remaining keys into
// Extra so they round-trip back through MarshalJSON unchanged.
func (s *SlackAdapterConfig) UnmarshalJSON(data []byte) error {
	type alias SlackAdapterConfig
	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*s = SlackAdapterConfig(aux)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if _, known := slackAdapterKnownKeys[k]; known {
			continue
		}
		if s.Extra == nil {
			s.Extra = make(map[string]any, len(raw))
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return err
		}
		s.Extra[k] = val
	}
	return nil
}

// HasMessagingAdapters reports whether dev messaging adapters are configured.
func (d *Dev) HasMessagingAdapters() bool {
	return d != nil && d.Interfaces != nil && d.Interfaces.Messaging != nil && len(d.Interfaces.Messaging.Adapters) > 0
}

// MessagingAdapters returns the dev messaging adapter names, or nil.
func (d *Dev) MessagingAdapters() []string {
	if d == nil || d.Interfaces == nil || d.Interfaces.Messaging == nil {
		return nil
	}
	return d.Interfaces.Messaging.Adapters
}

// SlackConfig returns the Slack adapter configuration, or nil when not configured.
func (d *Dev) SlackConfig() *SlackAdapterConfig {
	if d == nil || d.Interfaces == nil || d.Interfaces.Messaging == nil {
		return nil
	}
	return d.Interfaces.Messaging.Slack
}

// MessagingLogLevel returns the dev messaging sidecar log level, or empty when unset.
func (d *Dev) MessagingLogLevel() string {
	if d == nil || d.Interfaces == nil || d.Interfaces.Messaging == nil {
		return ""
	}
	return d.Interfaces.Messaging.LogLevel
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
