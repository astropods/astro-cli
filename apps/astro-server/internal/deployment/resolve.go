package deployment

import (
	"fmt"
	"sort"
	"strings"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// Role identifies a logical container slot in a deployment. One row per
// (deployment, role, env_name) in deployment_build_env.
//
// Format: "<kind>" for kinds with a single instance per deployment, or
// "<kind>:<name>" for kinds that can have multiple instances.
type Role string

// Well-known single-instance roles.
const (
	RoleAgent     Role = "agent"
	RoleMessaging Role = "messaging"
	RoleCollector Role = "collector"
)

// KnowledgeRole returns the role for a named knowledge store.
// Each store in a deployment is its own role.
func KnowledgeRole(name string) Role { return Role("knowledge:" + name) }

// IngestionRole returns the role for a named ingestion job.
// Each ingestion entry in a deployment is its own role.
func IngestionRole(name string) Role { return Role("ingestion:" + name) }

func (r Role) String() string { return string(r) }

// EnvSource categorises where a Resolution came from. Carried into the
// API response and surfaced in the UI's Variables tab as the source badge.
// The applier ignores it; it's metadata for humans.
type EnvSource string

const (
	// EnvSourceUserVar — value came from the user's deploy-form input.
	EnvSourceUserVar EnvSource = "user_var"
	// EnvSourcePlatformMeta — derived from the deployment record itself
	// (ASTRO_AGENT_NAME, _BUILD, _HOST, _URL).
	EnvSourcePlatformMeta EnvSource = "platform_meta"
	// EnvSourceServiceURL — resolved from a ${knowledge.X.host}/.port/.url
	// reference. Plain values, never secret.
	EnvSourceServiceURL EnvSource = "service_url"
	// EnvSourceKnowledgeCred — resolved from a ${knowledge.X.credentials.Y}
	// reference. Always secret.
	EnvSourceKnowledgeCred EnvSource = "knowledge_cred"
	// EnvSourceAuthToken — platform-issued JWT (e.g. ASTRO_AUTHZ_TOKEN).
	EnvSourceAuthToken EnvSource = "auth_token"
	// EnvSourceAdapterConfig — inlined adapter configuration
	// (e.g. SLACK_CONFIG JSON). Driven by interfaces.environment.
	EnvSourceAdapterConfig EnvSource = "adapter_config"
	// EnvSourceDerived — escape hatch for synthesised values that don't
	// fit another category (e.g. OTEL_EXPORTER_OTLP_ENDPOINT computed
	// from collector coordinates).
	EnvSourceDerived EnvSource = "derived"
)

func (s EnvSource) String() string { return string(s) }

// Resolution is one resolved env entry destined for one container.
// It corresponds 1:1 with a row in deployment_build_env.
type Resolution struct {
	Role     Role
	EnvName  string
	Value    string // plaintext; encrypted at persistence time
	IsSecret bool
	Source   EnvSource

	// Provenance for user_var rows. Zero values on platform-emitted rows.
	UserVarName   string
	AccountVarRef string
	Optional      bool
}

// ResolveOptions provides everything Resolve needs beyond the deployment
// spec itself. All fields are inputs the caller has already gathered.
//
// This is a pure-function context: no DB, no K8s, no encryptor. Callers
// load these from their respective stores and pass them in.
type ResolveOptions struct {
	Namespace string // for service DNS

	// BoundKnowledge maps knowledge entry name → resolved store info.
	// For self-hosted stores the host comes from the deployment's own
	// service DNS; for bound (managed) stores it comes from the binding.
	BoundKnowledge map[string]BoundKnowledgeInfo

	// BoundCredentials maps "<knowledgeName>.<attr>" → credential value.
	// Used to resolve ${knowledge.X.credentials.Y} references for
	// self-hosted and bound stores alike.
	BoundCredentials map[string]string

	// AccountVarRefs maps user variable name → the original
	// ${account.var.X} reference if the user picked a linked account
	// variable. Empty for unlinked entries.
	AccountVarRefs map[string]string

	// Platform-issued tokens.
	AuthToken         string // ASTRO_AUTHZ_TOKEN — agent + messaging
	LangfuseAuthToken string // collector
	LangfuseBaseURL   string // collector

	// ExternalAgentHost is the public hostname assigned to the agent's
	// frontend ingress, when one exists. Surfaces to the agent as
	// ASTRO_EXTERNAL_AGENT_URL=https://<host>.
	ExternalAgentHost string

	// DeploymentID is needed for collector env (ASTRO_DEPLOYMENT_ID).
	DeploymentID string
}

// Resolve walks the deployment spec and emits one Resolution per
// (role, env_name) tuple. Pure function — no I/O.
//
// The result is the complete set of env rows for the deployment: every
// container's projected ConfigMap and Secret derives from this slice,
// filtered by Role.
func Resolve(ds *spec.AstroDeploymentSpec, opts ResolveOptions) ([]Resolution, error) {
	if ds == nil {
		return nil, fmt.Errorf("deployment spec is nil")
	}

	// Re-use the existing component lookup for ${...host}/.port/.url resolution.
	rctx := ResolveContext{
		Namespace:        opts.Namespace,
		AgentName:        ds.Source.Name,
		BuildID:          ds.Source.Build,
		BoundKnowledge:   opts.BoundKnowledge,
		BoundCredentials: opts.BoundCredentials,
	}
	lookup := buildComponentLookup(ds, rctx)

	var out []Resolution

	out = append(out, resolveUserVars(ds, opts)...)
	out = append(out, resolveAgentRole(ds, opts, lookup, rctx)...)
	out = append(out, resolveMessagingRole(ds, opts, lookup, rctx)...)
	out = append(out, resolveCollectorRole(ds, opts)...)
	for name := range ds.Knowledge {
		out = append(out, resolveKnowledgeRole(ds, opts, name)...)
	}
	for name, ing := range ds.Ingestion {
		out = append(out, resolveIngestionRole(ds, opts, lookup, rctx, name, ing)...)
	}

	// Stable order for deterministic test assertions and diff cleanliness.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].EnvName < out[j].EnvName
	})

	return out, nil
}

// UserVarResolutions returns just the user_var rows for a deployment
// spec — the subset of Resolve's output that depends only on
// ds.Variables + AccountVarRefs. Used by the deploy handler to write
// user-supplied variables synchronously alongside the deployment
// record, before the applier (which produces platform-emitted rows)
// ever runs.
//
// Nil credentials, tokens, etc. are fine: those drive non-user_var
// rows that this function intentionally skips.
func UserVarResolutions(ds *spec.AstroDeploymentSpec, varRefs map[string]string) []Resolution {
	return resolveUserVars(ds, ResolveOptions{AccountVarRefs: varRefs})
}

// resolveUserVars walks ds.Variables and emits user_var rows for every
// (variable, target_role) pair. Targets dictates which roles see which
// variables — this is what makes cross-role leaks impossible.
func resolveUserVars(ds *spec.AstroDeploymentSpec, opts ResolveOptions) []Resolution {
	var out []Resolution
	for name, v := range ds.Variables {
		roles := rolesForTargets(v.Targets, ds)
		if len(roles) == 0 {
			continue
		}
		ref := opts.AccountVarRefs[name]
		for _, role := range roles {
			out = append(out, Resolution{
				Role:          role,
				EnvName:       name,
				Value:         v.Value,
				IsSecret:      v.Secret,
				Source:        EnvSourceUserVar,
				UserVarName:   name,
				AccountVarRef: ref,
				Optional:      v.Optional,
			})
		}
	}
	return out
}

// rolesForTargets converts a variable's Targets list into the concrete
// container roles that should receive a row.
//
//	"agent"                 → RoleAgent
//	"interface.<adapter>"   → RoleMessaging  (one row, regardless of how
//	                                          many adapter targets the var has)
//	"ingestion"             → IngestionRole(<name>) for every declared
//	                          ingestion (the bare form is a wildcard).
//	"ingestion.<name>"      → IngestionRole(<name>) only — narrows the
//	                          fan-out to one specific ingestion.
func rolesForTargets(targets []string, ds *spec.AstroDeploymentSpec) []Role {
	seen := map[Role]bool{}
	add := func(r Role) {
		if !seen[r] {
			seen[r] = true
		}
	}
	for _, t := range targets {
		switch {
		case t == "agent":
			add(RoleAgent)
		case strings.HasPrefix(t, "interface."):
			add(RoleMessaging)
		case t == "ingestion":
			for name := range ds.Ingestion {
				add(IngestionRole(name))
			}
		case strings.HasPrefix(t, "ingestion."):
			name := strings.TrimPrefix(t, "ingestion.")
			if _, ok := ds.Ingestion[name]; ok {
				add(IngestionRole(name))
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]Role, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// resolveAgentRole emits Resolutions for the user's main "app" container.
//
// The agent reads:
//   - platform metadata (ASTRO_AGENT_NAME / _BUILD / _HOST / _URL)
//   - the auth token (ASTRO_AUTHZ_TOKEN)
//   - knowledge-store connection coordinates (HOST/PORT/URL per store) —
//     non-secret service URLs derived from declared providers
//   - knowledge-store credentials (USER/PASSWORD/DB per store) — secret,
//     with per-store renaming (POSTGRES_USERS_USER for the second store)
//   - the OTel collector endpoint (when observability is enabled)
//   - the messaging gRPC address (when interfaces are configured)
//   - any custom entries the user wrote in ds.Agent.Environment that aren't
//     covered by the auto-emitted rows above (resolved via the existing
//     ${...} pipeline)
//
// User variables with Targets containing "agent" are emitted by
// resolveUserVars and not duplicated here.
func resolveAgentRole(ds *spec.AstroDeploymentSpec, opts ResolveOptions, lookup map[string]componentInfo, rctx ResolveContext) []Resolution {
	var out []Resolution

	// --- Platform meta ---
	out = append(out,
		Resolution{Role: RoleAgent, EnvName: "ASTRO_AGENT_NAME",
			Value: ds.Source.Name, Source: EnvSourcePlatformMeta},
	)
	if ds.Source.Build != "" {
		out = append(out, Resolution{Role: RoleAgent, EnvName: "ASTRO_AGENT_BUILD",
			Value: ds.Source.Build, Source: EnvSourcePlatformMeta})
	}
	agentSvc := GenerateAgentResourceName(ds.Source.Name, "agent")
	agentHost := GenerateServiceDNS(agentSvc, opts.Namespace)
	agentPort := spec.PrimaryPort(ds.Agent.Endpoints)
	if agentPort == 0 {
		agentPort = 8080
	}
	out = append(out,
		Resolution{Role: RoleAgent, EnvName: "ASTRO_AGENT_HOST",
			Value: agentHost, Source: EnvSourcePlatformMeta},
		Resolution{Role: RoleAgent, EnvName: "ASTRO_AGENT_URL",
			Value:  fmt.Sprintf("http://%s:%d", agentHost, agentPort),
			Source: EnvSourcePlatformMeta},
	)
	if opts.ExternalAgentHost != "" && spec.ExposedEndpoint(ds.Agent.Endpoints) != nil {
		out = append(out, Resolution{
			Role: RoleAgent, EnvName: "ASTRO_EXTERNAL_AGENT_URL",
			Value:  "https://" + opts.ExternalAgentHost,
			Source: EnvSourcePlatformMeta,
		})
	}

	// --- Auth token ---
	if opts.AuthToken != "" {
		out = append(out, Resolution{
			Role: RoleAgent, EnvName: "ASTRO_AUTHZ_TOKEN",
			Value: opts.AuthToken, IsSecret: true, Source: EnvSourceAuthToken,
		})
	}

	// --- OTel collector endpoint ---
	if ds.Observability.Enabled {
		collectorSvc := GenerateAgentResourceName(ds.Source.Name, "collector")
		collectorHost := GenerateServiceDNS(collectorSvc, opts.Namespace)
		otlpPort := ds.Observability.Port
		if otlpPort == 0 {
			otlpPort = 4318
		}
		out = append(out, Resolution{
			Role: RoleAgent, EnvName: "OTEL_EXPORTER_OTLP_ENDPOINT",
			Value:  fmt.Sprintf("http://%s:%d", collectorHost, otlpPort),
			Source: EnvSourceDerived,
		})
	}

	// --- Messaging gRPC address ---
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		grpcPort := 0
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc"); ep != nil {
			grpcPort = ep.Port
		}
		if grpcPort == 0 {
			grpcPort = spec.PrimaryPort(ds.Interfaces.Endpoints)
		}
		if grpcPort == 0 {
			grpcPort = 9090
		}
		msgSvc := GenerateAgentResourceName(ds.Source.Name, "messaging")
		msgHost := GenerateServiceDNS(msgSvc, opts.Namespace)
		out = append(out, Resolution{
			Role: RoleAgent, EnvName: "GRPC_SERVER_ADDR",
			Value:  fmt.Sprintf("%s:%d", msgHost, grpcPort),
			Source: EnvSourceServiceURL,
		})
	}

	// --- Knowledge store connection coordinates + credentials ---
	out = append(out, resolveAgentKnowledgeRows(ds, opts, lookup)...)

	// --- Custom user-written entries in ds.Agent.Environment ---
	// After template.go is cleaned up to stop auto-injecting platform/
	// service/credential refs, this block handles only what the user
	// explicitly wrote. During the transitional period it may overlap
	// with platform-emitted rows above; in that case the platform row
	// wins (we skip the agent-env entry by env-name).
	skip := envNamesIn(out, RoleAgent)
	for envName, value := range ds.Agent.Environment {
		if skip[envName] {
			continue
		}
		resolved := resolveValue(value, lookup, ds, rctx)
		if referencesSecret(value, ds) {
			// User-written ref to a secret variable or a knowledge cred.
			// Surface as knowledge_cred when that's the underlying ref;
			// otherwise as user_var (the variable will already have its
			// own row via resolveUserVars, so we end up dropping this entry
			// — but we keep the path explicit for clarity).
			if hasKnowledgeCredRef(value) {
				out = append(out, Resolution{
					Role: RoleAgent, EnvName: envName,
					Value: resolved, IsSecret: true,
					Source: EnvSourceKnowledgeCred,
				})
			}
			// Otherwise: variable refs are already covered by resolveUserVars.
			continue
		}
		// Plain literal or non-secret resolved ref.
		out = append(out, Resolution{
			Role: RoleAgent, EnvName: envName,
			Value: resolved, Source: EnvSourceDerived,
		})
	}

	return out
}

// resolveAgentKnowledgeRows emits the connection-coord + credential rows
// the agent needs for each declared knowledge store. Per-store renaming
// (POSTGRES_USERS_USER for the second postgres store) is applied here.
func resolveAgentKnowledgeRows(ds *spec.AstroDeploymentSpec, opts ResolveOptions, lookup map[string]componentInfo) []Resolution {
	if len(ds.Knowledge) == 0 {
		return nil
	}
	var out []Resolution

	// Group entries by provider EnvPrefix so we can detect duplicates and
	// pick a primary per group (matches the existing logic in template.go
	// + spec_applier.knowledgeCredEnvVars).
	type entry struct {
		name     string
		provider string
		prefix   string
	}
	groups := map[string][]entry{}
	names := make([]string, 0, len(ds.Knowledge))
	for n := range ds.Knowledge {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		k := ds.Knowledge[n]
		if k.Provider == "" {
			continue
		}
		prov := spec.GetProvider(k.Provider)
		if prov.EnvPrefix == "" {
			continue
		}
		groups[prov.EnvPrefix] = append(groups[prov.EnvPrefix], entry{
			name: n, provider: k.Provider, prefix: prov.EnvPrefix,
		})
	}

	for prefix, group := range groups {
		groupNames := make([]string, 0, len(group))
		for _, e := range group {
			groupNames = append(groupNames, e.name)
		}
		primary := PickPrimaryName(append([]string(nil), groupNames...), group[0].provider)
		isDup := len(group) > 1
		for _, e := range group {
			isPrimary := e.name == primary
			prov := spec.GetProvider(e.provider)
			info, hasInfo := lookup["knowledge."+e.name]

			// Connection coords: HOST / PORT / URL — service_url, not secret.
			if hasInfo {
				for _, key := range ProviderEnvKeys(prefix, e.name, e.provider, "HOST", isDup, isPrimary) {
					out = append(out, Resolution{
						Role: RoleAgent, EnvName: key,
						Value: info.Host, Source: EnvSourceServiceURL,
					})
				}
				primaryEpName := primaryEndpointFromInfo(info.Endpoints)
				if primaryEpName != "" {
					ep := info.Endpoints[primaryEpName]
					for _, key := range ProviderEnvKeys(prefix, e.name, e.provider, "PORT", isDup, isPrimary) {
						out = append(out, Resolution{
							Role: RoleAgent, EnvName: key,
							Value:  fmt.Sprintf("%d", ep.Port),
							Source: EnvSourceServiceURL,
						})
					}
					if prov.URLScheme != "" {
						scheme := prov.URLScheme
						for _, key := range ProviderEnvKeys(prefix, e.name, e.provider, "URL", isDup, isPrimary) {
							out = append(out, Resolution{
								Role: RoleAgent, EnvName: key,
								Value:  fmt.Sprintf("%s://%s:%d", scheme, info.Host, ep.Port),
								Source: EnvSourceServiceURL,
							})
						}
					}
				}
			}

			// Credentials: USER / PASSWORD / DB — knowledge_cred, secret.
			for _, cred := range prov.BindCredentials {
				suffix := strings.ToUpper(cred.Attr)
				if cred.Attr == "database" {
					suffix = "DB"
				}
				credKey := e.name + "." + cred.Attr
				val, ok := opts.BoundCredentials[credKey]
				if !ok {
					continue
				}
				for _, key := range ProviderEnvKeys(prefix, e.name, e.provider, suffix, isDup, isPrimary) {
					out = append(out, Resolution{
						Role: RoleAgent, EnvName: key,
						Value: val, IsSecret: true,
						Source: EnvSourceKnowledgeCred,
					})
				}
			}
		}
	}
	return out
}

// resolveMessagingRole emits Resolutions for the messaging sidecar.
// Mirrors the env wiring done by buildMessagingContainer today.
func resolveMessagingRole(ds *spec.AstroDeploymentSpec, opts ResolveOptions, lookup map[string]componentInfo, rctx ResolveContext) []Resolution {
	if ds.Interfaces == nil || len(ds.Interfaces.Adapters) == 0 {
		return nil
	}
	var out []Resolution

	// Resolve grpc + http (web) ports the same way the applier picks them.
	grpcPort := 0
	if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc"); ep != nil {
		grpcPort = ep.Port
	}
	if grpcPort == 0 {
		grpcPort = spec.PrimaryPort(ds.Interfaces.Endpoints)
	}
	if grpcPort == 0 {
		grpcPort = 9090
	}
	webPort := 0
	if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil {
		webPort = ep.Port
	}
	if webPort == 0 {
		webPort = 8090
	}
	agentPort := spec.PrimaryPort(ds.Agent.Endpoints)
	if agentPort == 0 {
		agentPort = 8080
	}
	// Push webPort off any port it would collide with on the messaging
	// pod or its host: agent app port (same pod), grpc port (same
	// container). The shift is +10 from the conflicting port.
	for webPort == agentPort || webPort == grpcPort {
		webPort = webPort + 10
	}

	slack, web := false, false
	for _, a := range ds.Interfaces.Adapters {
		switch a {
		case "slack":
			slack = true
		case "web":
			web = true
		}
	}

	// Hardcoded knobs the messaging binary needs.
	out = append(out,
		Resolution{Role: RoleMessaging, EnvName: "GRPC_ENABLED", Value: "true", Source: EnvSourceDerived},
		Resolution{Role: RoleMessaging, EnvName: "GRPC_LISTEN_ADDR",
			Value: fmt.Sprintf(":%d", grpcPort), Source: EnvSourceDerived},
		Resolution{Role: RoleMessaging, EnvName: "STORAGE_TYPE", Value: "memory", Source: EnvSourceDerived},
		Resolution{Role: RoleMessaging, EnvName: "DEPLOYMENT_MODE", Value: "all", Source: EnvSourceDerived},
	)
	if slack {
		out = append(out, Resolution{Role: RoleMessaging, EnvName: "SLACK_ENABLED",
			Value: "true", Source: EnvSourceDerived})
	}
	if web {
		out = append(out,
			Resolution{Role: RoleMessaging, EnvName: "WEB_ENABLED",
				Value: "true", Source: EnvSourceDerived},
			Resolution{Role: RoleMessaging, EnvName: "WEB_LISTEN_ADDR",
				Value: fmt.Sprintf(":%d", webPort), Source: EnvSourceDerived},
			Resolution{Role: RoleMessaging, EnvName: "WEB_SERVE_PLAYGROUND",
				Value: "true", Source: EnvSourceDerived},
		)
	}

	// Auth token.
	if opts.AuthToken != "" {
		out = append(out, Resolution{
			Role: RoleMessaging, EnvName: "ASTRO_AUTHZ_TOKEN",
			Value: opts.AuthToken, IsSecret: true, Source: EnvSourceAuthToken,
		})
	}

	// interfaces.environment entries that AREN'T already user_var rows.
	// (User variables with Targets="interface.*" are emitted by
	// resolveUserVars; we skip those here to avoid duplicates.)
	skip := envNamesIn(out, RoleMessaging)
	for envName, value := range ds.Interfaces.Environment {
		if skip[envName] {
			continue
		}
		// Variables already handled — don't emit a second row for the same name.
		if isVariableRef(value, ds) {
			continue
		}
		resolved := resolveValue(value, lookup, ds, rctx)
		out = append(out, Resolution{
			Role: RoleMessaging, EnvName: envName,
			Value: resolved, Source: EnvSourceAdapterConfig,
		})
	}

	return out
}

// resolveCollectorRole emits Resolutions for the OTel collector sidecar.
func resolveCollectorRole(ds *spec.AstroDeploymentSpec, opts ResolveOptions) []Resolution {
	if !ds.Observability.Enabled {
		return nil
	}
	var out []Resolution
	out = append(out,
		Resolution{Role: RoleCollector, EnvName: "ASTRO_AGENT_NAME",
			Value: ds.Source.Name, Source: EnvSourcePlatformMeta},
	)
	if ds.Source.Build != "" {
		out = append(out, Resolution{Role: RoleCollector, EnvName: "ASTRO_AGENT_VERSION",
			Value: ds.Source.Build, Source: EnvSourcePlatformMeta})
	}
	if opts.DeploymentID != "" {
		out = append(out, Resolution{Role: RoleCollector, EnvName: "ASTRO_DEPLOYMENT_ID",
			Value: opts.DeploymentID, Source: EnvSourcePlatformMeta})
	}
	if opts.LangfuseAuthToken != "" {
		out = append(out, Resolution{Role: RoleCollector, EnvName: "LANGFUSE_AUTH_TOKEN",
			Value: opts.LangfuseAuthToken, IsSecret: true, Source: EnvSourceDerived})
	}
	if opts.LangfuseBaseURL != "" {
		out = append(out, Resolution{Role: RoleCollector, EnvName: "LANGFUSE_BASE_URL",
			Value: opts.LangfuseBaseURL, Source: EnvSourceDerived})
	}
	return out
}

// resolveKnowledgeRole emits Resolutions for a knowledge container's own
// env. Today this is just the per-store cred Secret entries
// (POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB; or REDIS_PASSWORD).
//
// The values come from the per-store cred Secret in BoundCredentials,
// keyed as "<knowledgeName>.<attr>" — same source the agent reads from.
func resolveKnowledgeRole(ds *spec.AstroDeploymentSpec, opts ResolveOptions, name string) []Resolution {
	k := ds.Knowledge[name]
	if k.Provider == "" {
		return nil
	}
	if k.IsBound() {
		// Bound stores aren't deployed by us — no container, no rows.
		return nil
	}
	prov := spec.GetProvider(k.Provider)
	if prov.EnvPrefix == "" {
		return nil
	}
	role := KnowledgeRole(name)
	var out []Resolution
	for _, cred := range prov.BindCredentials {
		credKey := name + "." + cred.Attr
		val, ok := opts.BoundCredentials[credKey]
		if !ok {
			continue
		}
		// Knowledge containers consume the literal upstream key names
		// (POSTGRES_USER, not the prefixed renamed POSTGRES_USERS_USER).
		envName := strings.ToUpper(prov.EnvPrefix + "_" + cred.Attr)
		if cred.Attr == "database" {
			envName = strings.ToUpper(prov.EnvPrefix + "_DB")
		}
		out = append(out, Resolution{
			Role: role, EnvName: envName, Value: val,
			IsSecret: true, Source: EnvSourceKnowledgeCred,
		})
	}
	return out
}

// resolveIngestionRole emits Resolutions for an ingestion job/cron container.
// Today ingestion containers reuse the agent's ConfigMap+Secret bundle
// wholesale, so they get whatever the user has tagged with target
// "ingestion" plus the platform-emitted entries (under the new model,
// scoped to this role).
func resolveIngestionRole(ds *spec.AstroDeploymentSpec, opts ResolveOptions, lookup map[string]componentInfo, rctx ResolveContext, name string, ing spec.DeploymentIngestion) []Resolution {
	role := IngestionRole(name)
	var out []Resolution

	// Platform meta. Same as agent.
	out = append(out,
		Resolution{Role: role, EnvName: "ASTRO_AGENT_NAME",
			Value: ds.Source.Name, Source: EnvSourcePlatformMeta},
	)
	if ds.Source.Build != "" {
		out = append(out, Resolution{Role: role, EnvName: "ASTRO_AGENT_BUILD",
			Value: ds.Source.Build, Source: EnvSourcePlatformMeta})
	}

	// Custom env entries the user wrote in this ingestion's environment.
	// User variables with target "ingestion" are emitted via resolveUserVars.
	skip := envNamesIn(out, role)
	for envName, value := range ing.Environment {
		if skip[envName] {
			continue
		}
		if isVariableRef(value, ds) {
			continue // user_var row already created elsewhere
		}
		resolved := resolveValue(value, lookup, ds, rctx)
		out = append(out, Resolution{
			Role: role, EnvName: envName,
			Value: resolved, Source: EnvSourceDerived,
		})
	}
	return out
}

// envNamesIn returns the set of env names already emitted for a given
// role in `rs`. Used to avoid double-emitting a name when a later
// step would otherwise overlap an earlier one.
func envNamesIn(rs []Resolution, role Role) map[string]bool {
	out := map[string]bool{}
	for _, r := range rs {
		if r.Role == role {
			out[r.EnvName] = true
		}
	}
	return out
}

// hasKnowledgeCredRef reports whether `value` contains a
// ${knowledge.X.credentials.Y} reference.
func hasKnowledgeCredRef(value string) bool {
	for _, ref := range spec.ParseReferences(value) {
		if ref.Kind == spec.RefKnowledge && ref.Endpoint == "credentials" {
			return true
		}
	}
	return false
}

// isVariableRef reports whether `value` is a single `${variables.X}`
// reference. Used to suppress duplicate rows for variables already
// emitted by resolveUserVars.
func isVariableRef(value string, ds *spec.AstroDeploymentSpec) bool {
	refs := spec.ParseReferences(value)
	if len(refs) != 1 {
		return false
	}
	if refs[0].Kind != spec.RefVariable {
		return false
	}
	if _, ok := ds.Variables[refs[0].Name]; !ok {
		return false
	}
	return true
}

// primaryEndpointFromInfo picks the canonical endpoint name from a
// component's resolved endpoint map. Prefers "http" then "grpc"; falls
// back to any single entry. (template.go has a sibling helper that does
// the same against a spec.Endpoint map; this one operates on resolved
// componentEndpointInfo.)
func primaryEndpointFromInfo(eps map[string]componentEndpointInfo) string {
	if _, ok := eps["http"]; ok {
		return "http"
	}
	if _, ok := eps["grpc"]; ok {
		return "grpc"
	}
	for n := range eps {
		return n
	}
	return ""
}
