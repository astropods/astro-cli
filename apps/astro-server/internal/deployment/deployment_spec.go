package deployment

import (
	"sort"
	"time"

	spec "github.com/astropods/astro-spec"
)

// AstroDeploymentSpec represents the deployment/v1 or deployment-template/v1 specification.
// It is the intermediate artifact between the astro-spec (what the agent is)
// and infrastructure manifests (how it runs on a cluster).
type AstroDeploymentSpec struct {
	Spec          string                           `json:"spec" yaml:"spec"`
	Source        DeploymentSource                 `json:"source" yaml:"source"`
	Target        DeploymentTarget                 `json:"target" yaml:"target"`
	Agent         DeploymentAgent                  `json:"agent" yaml:"agent"`
	Models        map[string]DeploymentModel       `json:"models,omitempty" yaml:"models,omitempty"`
	Knowledge     map[string]DeploymentKnowledge   `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	Integrations  map[string]DeploymentIntegration `json:"integrations,omitempty" yaml:"integrations,omitempty"`
	Ingestion     map[string]DeploymentIngestion   `json:"ingestion,omitempty" yaml:"ingestion,omitempty"`
	Interfaces    *DeploymentInterfaces            `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Variables     map[string]Variable              `json:"variables,omitempty" yaml:"variables,omitempty"`
	Observability DeploymentObservability          `json:"observability" yaml:"observability"`
}

// DeploymentAuthorizationGrant is a single subject access grant. The adapter
// is implied by where the grant lives (interfaces.auth.web.grants vs
// interfaces.auth.slack.grants), not carried on the grant itself.
//
// Exactly one of Org, UserID, or Anyone must be set:
//   - Org    — any member of this organization (account) is allowed.
//   - UserID — this specific WorkOS user is allowed (web only).
//   - Anyone — anyone hitting the adapter is allowed.
type DeploymentAuthorizationGrant struct {
	Org    string `json:"org,omitempty" yaml:"org,omitempty"`
	UserID string `json:"user_id,omitempty" yaml:"user_id,omitempty"`
	Anyone bool   `json:"anyone,omitempty" yaml:"anyone,omitempty"`
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
// Template fields (Default, Description, Label, Placeholder, HelpURL, Datatype, DisplayAs, Options, Fields)
// are only present in deployment-template/v1 and must be stripped before deployment/v1.
type Variable struct {
	Value       string                   `json:"value,omitempty" yaml:"value,omitempty"`
	Ref         string                   `json:"ref,omitempty" yaml:"ref,omitempty"` // reference to an account variable by name (deploy-time only)
	Default     string                   `json:"default,omitempty" yaml:"default,omitempty"`
	Targets     []string                 `json:"targets,omitempty" yaml:"targets,omitempty"`
	Secret      bool                     `json:"secret,omitempty" yaml:"secret,omitempty"`
	Optional    bool                     `json:"optional,omitempty" yaml:"optional,omitempty"`
	Description string                   `json:"description,omitempty" yaml:"description,omitempty"`
	Label       string                   `json:"label,omitempty" yaml:"label,omitempty"`
	Placeholder string                   `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
	HelpURL     string                   `json:"help_url,omitempty" yaml:"help_url,omitempty"`
	Datatype    string                   `json:"datatype,omitempty" yaml:"datatype,omitempty"`
	DisplayAs   string                   `json:"display-as,omitempty" yaml:"display-as,omitempty"`
	Options     []string                 `json:"options,omitempty" yaml:"options,omitempty"`
	Fields      map[string]VariableField `json:"fields,omitempty" yaml:"fields,omitempty"` // sub-field schema for datatype=object
	// Deprecated, when non-empty, signals that the variable is on its way out
	// and explains what to use instead. The UI demotes deprecated variables
	// (strike-through label, "Deprecated" badge, tooltip carrying this
	// message) but keeps them editable so existing values can be cleared.
	Deprecated string `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	// Configured signals an inline secret is stored for this deployment but
	// its value is intentionally omitted from template API responses. A
	// finalized, signed deployment/v1 template may carry this as an opaque
	// "preserve the existing value" sentinel; the deploy handler resolves it
	// server-side and clears the flag before persistence.
	Configured bool `json:"configured,omitempty" yaml:"configured,omitempty"`
}

// VariableField describes a single sub-field within an object-typed variable.
// The field key matches the JSON property name in the serialized value.
type VariableField struct {
	Label       string `json:"label,omitempty" yaml:"label,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Placeholder string `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
	Datatype    string `json:"datatype,omitempty" yaml:"datatype,omitempty"` // "csv" for comma-separated arrays, "string", "boolean"
	Optional    bool   `json:"optional,omitempty" yaml:"optional,omitempty"`
	// Deprecated, when non-empty, signals that the sub-field is on its way
	// out and explains what to use instead. Same UI semantics as
	// Variable.Deprecated.
	Deprecated string `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// DeploymentSource identifies the agent being deployed.
type DeploymentSource struct {
	Account  string `json:"account" yaml:"account"` // implementation-internal
	Name     string `json:"name" yaml:"name"`
	Build    string `json:"build" yaml:"build"`
	Registry string `json:"registry" yaml:"registry"`
}

// DeploymentTarget describes where to deploy.
//
// ClusterID is an optional pin to a specific additional cluster registered
// in the `clusters` table. Empty means the deployment routes to the
// primary cluster (the one astro-server is configured against via env
// vars / kubeconfig).
type DeploymentTarget struct {
	Runtime      string `json:"runtime" yaml:"runtime"`
	Account      string `json:"account,omitempty" yaml:"account,omitempty"`
	DisplayName  string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty" yaml:"deployment_id,omitempty"`
	ClusterID    string `json:"cluster_id,omitempty" yaml:"cluster_id,omitempty"`
}

// DeploymentAgent describes the main agent container.
//
// When Volume is set the agent runs as a StatefulSet with a PVC sized by
// Storage (defaults applied if Storage is nil). An empty Volume means an
// ephemeral Deployment.
type DeploymentAgent struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Distributed bool                `json:"distributed,omitempty" yaml:"distributed,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Volume      string              `json:"volume,omitempty" yaml:"volume,omitempty"` // mount path; non-empty switches to StatefulSet + PVC
	Storage     *StorageConfig      `json:"storage,omitempty" yaml:"storage,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck   `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	// AIGateway mirrors AstroSpec.Agent.AIGateway: when true the deployer
	// mints an Astro AI Gateway virtual key and the applier injects
	// ASTRO_GATEWAY_URL + ASTRO_GATEWAY_API_KEY into the agent container.
	AIGateway bool `json:"astro_ai_gateway,omitempty" yaml:"astro_ai_gateway,omitempty"`
	// ResponseTimeout bounds how long the front-door proxy (Contour) waits for
	// this deployment's containers to send a complete response before returning
	// 504. Surfaced as the projectcontour.io/response-timeout annotation on every
	// tenant Ingress. A Go duration string ("15s", "2m") between 0 and
	// MaxResponseTimeout. Empty ⇒ DefaultResponseTimeout.
	ResponseTimeout string `json:"response_timeout,omitempty" yaml:"response_timeout,omitempty"`
}

// DeploymentModel describes a model sidecar container.
type DeploymentModel struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	GPU         *DeploymentGPU      `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck   `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
	// implementation-internal fields
	Model      string `json:"model,omitempty" yaml:"model,omitempty"`
	Persistent bool   `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Provider   string `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// DeploymentKnowledge describes a knowledge store container or a binding to a managed store.
// When Binding is set, all container fields are zero-valued — the managed store provides everything.
type DeploymentKnowledge struct {
	Image       string              `json:"image,omitempty" yaml:"image,omitempty"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Resources   DeploymentResources `json:"resources,omitzero" yaml:"resources"`
	Persistent  bool                `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	Volume      string              `json:"volume,omitempty" yaml:"volume,omitempty"` // mount path for persistent storage
	Storage     *StorageConfig      `json:"storage,omitempty" yaml:"storage,omitempty"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck   `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update,omitzero" yaml:"update"`
	Provider    string              `json:"provider,omitempty" yaml:"provider,omitempty"` // implementation-internal
	Binding     string              `json:"binding,omitempty" yaml:"binding,omitempty"`   // managed store ARN; when set, container fields are zero
}

// IsBound returns true if this knowledge entry is bound to a managed store.
func (dk DeploymentKnowledge) IsBound() bool { return dk.Binding != "" }

// DeploymentIntegration describes an integration sidecar container.
type DeploymentIntegration struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Replicas    int                 `json:"replicas" yaml:"replicas"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck   `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Update      UpdateStrategy      `json:"update" yaml:"update"`
}

// DeploymentIngestion describes an ingestion job container.
type DeploymentIngestion struct {
	Image       string              `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Resources   DeploymentResources `json:"resources" yaml:"resources"`
	Trigger     DeploymentTrigger   `json:"trigger" yaml:"trigger"`
	Environment map[string]string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck   `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
}

// DeploymentTrigger defines when an ingestion job runs.
type DeploymentTrigger struct {
	Type     string `json:"type" yaml:"type"`
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
}

// DeploymentInterfaces describes the messaging sidecar.
type DeploymentInterfaces struct {
	Adapters    []string                  `json:"adapters" yaml:"adapters"`
	Image       string                    `json:"image" yaml:"image"`
	Endpoints   map[string]Endpoint       `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Resources   DeploymentResources       `json:"resources" yaml:"resources"`
	Environment map[string]string         `json:"environment,omitempty" yaml:"environment,omitempty"`
	Healthcheck *spec.Healthcheck         `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`
	Auth        *DeploymentInterfacesAuth `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// WebPublic reports whether the messaging web (chat) surface opts into the
// open (no-OIDC) cohort. Nil-safe across the interfaces/auth/web chain.
func (i *DeploymentInterfaces) WebPublic() bool {
	return i != nil && i.Auth != nil && i.Auth.Web != nil && i.Auth.Web.Public
}

// CustomPublic reports whether the agent's custom interface opts into the open
// (no-OIDC) cohort. Nil-safe across the interfaces/auth/custom chain.
func (i *DeploymentInterfaces) CustomPublic() bool {
	return i != nil && i.Auth != nil && i.Auth.Custom != nil && i.Auth.Custom.Public
}

// CustomGrants returns the custom-interface grants, or nil. Nil-safe.
func (i *DeploymentInterfaces) CustomGrants() []DeploymentAuthorizationGrant {
	if i == nil || i.Auth == nil || i.Auth.Custom == nil {
		return nil
	}
	return i.Auth.Custom.Grants
}

// DeploymentInterfacesAuth controls authentication and authorization for the
// messaging interfaces. Grants live under the adapter they apply to.
// A request is allowed iff a matching grant exists; there is no default-allow.
type DeploymentInterfacesAuth struct {
	// Web controls auth + access for the web adapter. Nil means no web config.
	Web *DeploymentWebAuth `json:"web,omitempty" yaml:"web,omitempty"`

	// Slack controls access for the slack adapter. Nil means no slack grants.
	Slack *DeploymentSlackAuth `json:"slack,omitempty" yaml:"slack,omitempty"`

	// Custom controls access for the agent's own custom web interface (the UI
	// it serves itself). Distinct from Web, which is the platform's messaging
	// web chat. Nil means no custom-interface config.
	Custom *DeploymentCustomAuth `json:"custom,omitempty" yaml:"custom,omitempty"`
}

// DeploymentCustomAuth configures the agent's custom web interface.
type DeploymentCustomAuth struct {
	// Public routes the custom interface to the open (no-OIDC) ingress cohort
	// (agents.public.<domain>), so the front-door ALB does not require sign-in.
	Public bool `json:"public,omitempty" yaml:"public,omitempty"`
	// Grants enumerate who may use the custom interface. Recorded under the
	// "custom" adapter but NOT enforced by the platform today — the agent's own
	// server is responsible for authorization. Stored for visibility/future use.
	Grants []DeploymentAuthorizationGrant `json:"grants,omitempty" yaml:"grants,omitempty"`
}

// DeploymentWebAuth configures the web adapter: ingress authentication
// (Type = "oidc") and access grants.
type DeploymentWebAuth struct {
	// Type of authentication: "oidc" uses server-level OIDC config;
	// "oidc-custom" is reserved for future per-deployment credentials.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`

	// Public routes the messaging web (chat) ingress to the open (no-OIDC)
	// cohort (agents.public.<domain>), bypassing the front-door ALB sign-in.
	// The OIDC identity header is then absent, so authorization must rely on
	// an "anyone" grant — validated at deploy.
	Public bool `json:"public,omitempty" yaml:"public,omitempty"`

	// Grants enumerate who can talk to the deployment via web. May include
	// account, user, and anyone subjects.
	Grants []DeploymentAuthorizationGrant `json:"grants,omitempty" yaml:"grants,omitempty"`
}

// DeploymentSlackAuth configures the slack adapter's access grants. Slack
// identity is opaque so only account-scoped grants are meaningful here;
// user_id and anyone are rejected at deploy.
type DeploymentSlackAuth struct {
	Grants []DeploymentAuthorizationGrant `json:"grants,omitempty" yaml:"grants,omitempty"`
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
		CPU: "100m", Memory: "1Gi",
		CPULimit: "100m", MemoryLimit: "1Gi",
	}
	GPUResources = DeploymentResources{
		CPU: "2", Memory: "8Gi",
		CPULimit: "4", MemoryLimit: "16Gi",
	}
	MessagingResources = DeploymentResources{
		CPU: "100m", Memory: "256Mi",
		CPULimit: "100m", MemoryLimit: "256Mi",
	}
	CollectorResources = DeploymentResources{
		CPU: "50m", Memory: "128Mi",
		CPULimit: "50m", MemoryLimit: "128Mi",
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

// DefaultAgentStorageSize is the disk size every agent gets by default. Disk is
// always provisioned (no opt-in), so this is a modest baseline rather than the
// larger DefaultStorageConfig size used for explicitly-requested volumes. 5Gi
// aligns with the smallest tier offered in the deploy UI's storage slider.
const DefaultAgentStorageSize = "5Gi"

// DefaultResponseTimeout is the front-door (Contour) upstream response timeout
// applied to every tenant Ingress when a deployment doesn't override it. Matches
// Envoy's stock per-route default. Overridable per deployment via the advanced
// config, up to MaxResponseTimeout.
const DefaultResponseTimeout = "15s"

// MaxResponseTimeout caps the per-deployment response-timeout override. Values
// above this are rejected at deploy time. Keeps a single slow/hung upstream from
// pinning a front-door Envoy worker for minutes.
const MaxResponseTimeout = 2 * time.Minute

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

// TemplateRequest is the input to the interactive POST deployment-template endpoint.
// All fields are optional — an empty body produces the same result as the legacy GET.
type TemplateRequest struct {
	Build        string                   `json:"build,omitempty"`
	DeploymentID string                   `json:"deployment_id,omitempty"`
	Revision     int                      `json:"revision,omitempty"`
	Interfaces   *TemplateInterfaces      `json:"interfaces,omitempty"`
	Variables    map[string]VariableInput `json:"variables,omitempty"`
	Models       map[string]string        `json:"models,omitempty"`    // model selection name → chosen identifier
	Schedules    map[string]string        `json:"schedules,omitempty"` // ingestion name → cron expression
	Bindings     *TemplateBindings        `json:"bindings,omitempty"`
	Provisioning *TemplateProvisioning    `json:"provisioning,omitempty"` // per-component compute/volume overrides
	Finalize     bool                     `json:"finalize,omitempty"`     // when true, response includes an HMAC signature for deploy
}

// TemplateProvisioning carries per-component compute and storage overrides.
// In v1 only the agent container is tunable; models and knowledge stores
// follow tier defaults and may be opened up later if there is demand.
// Nil/zero fields cause the server to fall back to the agent's
// astropods.yml declaration, then tier defaults.
type TemplateProvisioning struct {
	Agent *ComponentProvisioning `json:"agent,omitempty"`
}

// ComponentProvisioning groups the user-tunable infra knobs for one component.
type ComponentProvisioning struct {
	Compute *ComponentCompute `json:"compute,omitempty"`
	Volume  *ComponentVolume  `json:"volume,omitempty"`
	// ResponseTimeout overrides the front-door ingress upstream response timeout
	// for this deployment. A Go duration string ("15s", "2m") between 0 and
	// MaxResponseTimeout. Empty falls back to DefaultResponseTimeout.
	ResponseTimeout string `json:"response_timeout,omitempty"`
}

// ComponentCompute is the user-facing compute shape: one CPU value and one
// memory value, no separate request/limit. The server expands this into
// K8s requests + limits with request == limit, putting the pod in the
// Guaranteed QoS class.
type ComponentCompute struct {
	CPU    string `json:"cpu,omitempty"`    // e.g. "500m", "1", "2"
	Memory string `json:"memory,omitempty"` // e.g. "512Mi", "1Gi", "4Gi"
}

// ComponentVolume describes a mount path and the PVC backing it.
type ComponentVolume struct {
	Mount   string         `json:"mount,omitempty"`
	Storage *StorageConfig `json:"storage,omitempty"`
}

// VariableInput carries a user-supplied value or account-variable ref for a single variable.
type VariableInput struct {
	Value string `json:"value,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

// TemplateResponse is the response from the interactive POST deployment-template endpoint.
type TemplateResponse struct {
	Spec         string               `json:"spec"`                  // "deployment-template/v1"
	Template     AstroDeploymentSpec  `json:"template"`              // deployment/v1 — directly postable to /deploy
	Variables    map[string]Variable  `json:"variables,omitempty"`   // promoted variable schema for the UI
	Models       []ModelSelection     `json:"models,omitempty"`      // promoted gateway model selectors for the UI
	Interfaces   TemplateInterfaces   `json:"interfaces"`            // user-editable interface config (adapters + auth + grants)
	Schedules    map[string]string    `json:"schedules"`             // ingestion name → cron expression
	Bindings     *ResolvedBindings    `json:"bindings,omitempty"`    // resolved binding metadata for the UI
	Provisioning TemplateProvisioning `json:"provisioning,omitzero"` // resolved compute/volume values (echo of input + defaults)
	Validation   TemplateValidation   `json:"validation"`            // validity + field-level errors
	Signature    string               `json:"signature,omitempty"`   // HMAC-SHA256 of the template spec; deploy endpoint verifies this
}

// TemplateInterfaces carries the user-editable subset of DeploymentInterfaces
// at the response root. The full DeploymentInterfaces (image, resources, etc.)
// lives inside Template for the deploy POST.
type TemplateInterfaces struct {
	Adapters []string                  `json:"adapters"` // selected adapters (always present, may be empty)
	Auth     *DeploymentInterfacesAuth `json:"auth,omitempty"`
}

// TemplateBindings carries binding inputs from the client (entry name → store ARN).
type TemplateBindings struct {
	Knowledge map[string]string `json:"knowledge,omitempty"` // knowledge entry name → store ARN
}

// KnowledgeBindingInfo describes a resolved knowledge store binding in the template response.
type KnowledgeBindingInfo struct {
	ARN      string `json:"arn"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

// ResolvedBindings carries resolved binding metadata in the template response.
type ResolvedBindings struct {
	Knowledge map[string]KnowledgeBindingInfo `json:"knowledge,omitempty"`
}

// TemplateValidation carries the current validity state and any field-level errors.
type TemplateValidation struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
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
