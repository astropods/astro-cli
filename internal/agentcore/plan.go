// Package agentcore translates a parsed astropods.yml into the two-sided
// deployment config for the AWS Bedrock AgentCore Runtime invoke-per-turn model:
// an AgentCore CreateAgentRuntime request for the agent box, plus the EKS-side
// patch (messaging transport switch, VPC reachability) for everything that stays
// in the cluster.
//
// Build is a pure, offline translation: no AWS calls, no cluster mutations. It is
// the CLI port of the standalone AgentCore wrapper's plan stage, mapped onto the
// shared github.com/astropods/astro-spec types instead of a bespoke spec package.
package agentcore

import (
	"fmt"
	"sort"
	"strings"

	spec "github.com/astropods/astro-spec"
)

// Capabilities is what the AgentCore backend can give the agent box. The wrapper
// checks a spec against these up front and rejects specs that need more.
type Capabilities struct {
	PersistentDisk bool // durable volume — true via S3 Files/EFS mount
	Replicas       bool // operator-pinned replica count — false (serverless)
	WebIngress     bool // agent-served web UI at a URL — false (invoke-only)
}

// AgentCoreCaps are the fixed capabilities of the AgentCore Runtime backend.
func AgentCoreCaps() Capabilities {
	return Capabilities{PersistentDisk: true, Replicas: false, WebIngress: false}
}

// Options are the deploy-time knobs that aren't in the spec: the resolved image,
// VPC wiring, and how in-cluster dependency names map to VPC-resolvable ones.
// They default to placeholders so `plan`/`--dry-run` renders without AWS details.
type Options struct {
	Region         string
	ImageURI       string   // resolved ARM64 ECR image ref
	ImageArch      string   // "arm64" expected; anything else fails INV4
	ExecutionRole  string   // IAM execution role ARN
	Subnets        []string // VPC subnets (supported AZs)
	SecurityGroups []string // e.g. sg-agentcore-runtime
	// MessagingEndpoint is the VPC-resolvable messaging gRPC address the agent
	// dials (only used by the fallback gRPC transport; kept consistent).
	MessagingEndpoint string
	// DependencyHosts maps an in-cluster host (or *.svc name) to a
	// VPC-resolvable name (internal LB / private Route 53). Applied to any env
	// value that contains the key as a substring.
	DependencyHosts map[string]string
	// OTelEndpoint is the VPC-resolvable OTLP collector endpoint.
	OTelEndpoint string
	// IdleTimeoutSeconds / MaxLifetimeSeconds bound the runtime session.
	IdleTimeoutSeconds int
	MaxLifetimeSeconds int
	// RuntimeName overrides the AgentCore runtime name (else derived from the
	// spec name). Must satisfy [a-zA-Z][a-zA-Z0-9_]{0,47}.
	RuntimeName string
}

// Plan is the full offline translation output.
type Plan struct {
	Name      string             `json:"name"`
	AgentCore CreateAgentRuntime `json:"agentCore"`
	EKS       EKSPatch           `json:"eks"`
	Rewrites  []EnvRewrite       `json:"envRewrites"`
	// SecretsNeeded lists the env var names emitted as `@SECRET:` placeholders
	// (declared secret inputs + cloud-provider credentials). A real deploy must
	// supply a value for each via --secret/--secrets-file before it will create
	// the runtime; a dry-run leaves them as placeholders (never values).
	SecretsNeeded []string `json:"secretsNeeded,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// CreateAgentRuntime is the (subset of the) bedrock-agentcore-control request.
type CreateAgentRuntime struct {
	AgentRuntimeName  string             `json:"agentRuntimeName"`
	Protocol          string             `json:"protocol"` // "HTTP"
	Container         ContainerConfig    `json:"container"`
	NetworkMode       string             `json:"networkMode"` // "VPC"
	NetworkConfig     NetworkConfig      `json:"networkConfiguration"`
	Env               map[string]string  `json:"environment"`
	RoleArn           string             `json:"roleArn"`
	InboundAuth       string             `json:"inboundAuth"` // "SIGV4" for POC
	SessionConfig     SessionConfig      `json:"sessionConfiguration"`
	FilesystemConfigs []FilesystemConfig `json:"filesystemConfigurations,omitempty"`
}

type ContainerConfig struct {
	ImageURI string `json:"imageUri"`
	Port     int    `json:"port"` // 8080
}

type NetworkConfig struct {
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups"`
}

type SessionConfig struct {
	IdleTimeoutSeconds int `json:"idleRuntimeSessionTimeoutSeconds"`
	MaxLifetimeSeconds int `json:"maxLifetimeSeconds"`
}

type FilesystemConfig struct {
	Type      string `json:"type"` // "s3FilesAccessPoint"
	MountPath string `json:"mountPath"`
}

// EKSPatch is what changes on the cluster side; everything else stays as today.
type EKSPatch struct {
	MessagingEnv    map[string]string `json:"messagingEnv"`
	PrivateDNS      []DNSRecord       `json:"privateDnsRecords"`
	SecurityGroupIn []SGRule          `json:"securityGroupIngress"`
	VPCEndpoints    []string          `json:"vpcEndpoints"`
}

type DNSRecord struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Note   string `json:"note"`
}

type SGRule struct {
	FromSG string `json:"fromSecurityGroup"`
	Port   int    `json:"port"`
	Note   string `json:"note"`
}

// EnvRewrite records one in-cluster→VPC address substitution (for auditability
// and the "no .svc.cluster.local leaks" invariant).
type EnvRewrite struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// RejectionError is a capability-based up-front rejection with a clear reason.
type RejectionError struct{ Reason string }

func (e *RejectionError) Error() string { return e.Reason }

// Build produces the offline Plan, or a *RejectionError if the spec hard-requires
// a capability AgentCore can't honor (frontend web UI, pinned replicas, non-arm64).
func Build(s *spec.AstroSpec, opts Options) (*Plan, error) {
	caps := AgentCoreCaps()

	if s.Agent.HasFrontend() && !caps.WebIngress {
		return nil, &RejectionError{Reason: fmt.Sprintf(
			"agent.interfaces.frontend=true requires WebIngress, which AgentCore Runtime does not "+
				"support (invoke-only). Front the web UI with a BFF, or run this agent on EKS. (%s)", s.Name)}
	}
	if s.Agent.Distributed && !caps.Replicas {
		return nil, &RejectionError{Reason: fmt.Sprintf(
			"agent.distributed=true pins replicas, which AgentCore Runtime does not support "+
				"(serverless, per-session). (%s)", s.Name)}
	}
	if opts.ImageArch != "" && opts.ImageArch != "arm64" {
		return nil, &RejectionError{Reason: fmt.Sprintf(
			"image architecture %q is not arm64; AgentCore Runtime requires linux/arm64 (INV4)", opts.ImageArch)}
	}

	p := &Plan{Name: s.Name}

	// --- Build the agent's resolved env, then rewrite in-cluster addresses. ---
	env := map[string]string{
		"ASTRO_RUNTIME": "agentcore", // selects adapter-core's AgentCore transport
	}
	secrets := map[string]bool{} // env names emitted as @SECRET: placeholders
	// Inputs become env vars (value = default; secrets are referenced, not inlined).
	for _, in := range sortedInputs(s.Inputs) {
		if in.Secret {
			env[in.Name] = "@SECRET:" + in.Name // resolved at deploy from --secret
			secrets[in.Name] = true
		} else {
			env[in.Name] = in.Default
		}
	}
	if opts.MessagingEndpoint != "" {
		env["GRPC_SERVER_ADDR"] = opts.MessagingEndpoint
	}
	if opts.OTelEndpoint != "" {
		env["OTEL_EXPORTER_OTLP_ENDPOINT"] = opts.OTelEndpoint
	}
	// Components contribute env by kind:
	//  - container-mode or self-hosted provider (neo4j/qdrant/…) → a *_HOST the
	//    wrapper rewrites to a VPC-resolvable name.
	//  - cloud provider (openai/anthropic/github/…) → a credential @SECRET: env.
	addComponentEnv(env, secrets, "MODEL", modelComponents(s.Models))
	addComponentEnv(env, secrets, "KNOWLEDGE", knowledgeComponents(s.Knowledge))
	addComponentEnv(env, secrets, "", integrationComponents(s.Integrations))

	p.SecretsNeeded = sortedBoolKeys(secrets)
	if len(p.SecretsNeeded) > 0 {
		p.Warnings = append(p.Warnings, "secrets required (supply at deploy via --secret NAME=VALUE / --secrets-file): "+strings.Join(p.SecretsNeeded, ", "))
	}

	p.Rewrites = rewriteEnv(env, opts.DependencyHosts)

	// --- AgentCore CreateAgentRuntime request ---
	runtimeName := sanitizeName(s.Name)
	if strings.TrimSpace(opts.RuntimeName) != "" {
		runtimeName = sanitizeName(opts.RuntimeName)
	}
	p.AgentCore = CreateAgentRuntime{
		AgentRuntimeName: runtimeName,
		Protocol:         "HTTP",
		Container:        ContainerConfig{ImageURI: opts.ImageURI, Port: 8080},
		NetworkMode:      "VPC",
		NetworkConfig:    NetworkConfig{Subnets: opts.Subnets, SecurityGroups: opts.SecurityGroups},
		Env:              env,
		RoleArn:          opts.ExecutionRole,
		InboundAuth:      "SIGV4",
		SessionConfig: SessionConfig{
			IdleTimeoutSeconds: orDefault(opts.IdleTimeoutSeconds, 900),
			MaxLifetimeSeconds: orDefault(opts.MaxLifetimeSeconds, 28800),
		},
	}
	// Every Astro agent gets a persistent /data disk; map it to an S3 Files
	// access-point mount so durable state survives per-session runtimes.
	p.AgentCore.FilesystemConfigs = []FilesystemConfig{
		{Type: "s3FilesAccessPoint", MountPath: spec.DefaultAgentVolumeMount},
	}

	// --- EKS-side patch ---
	// The signed AWS backend is selected by ASTRO_DEPLOY_TARGET=aws +
	// AGENT_RUNTIME_ARN. The ARN doesn't exist at plan time — it materializes
	// when the deploy step calls CreateAgentRuntime — so it's a placeholder here
	// and filled in by the deploy step's emitted messaging env block.
	p.EKS = EKSPatch{
		MessagingEnv: map[string]string{
			"AGENT_TRANSPORT":     "agentcore",
			"ASTRO_DEPLOY_TARGET": "aws",
			"AGENT_RUNTIME_ARN":   "@AGENT_RUNTIME_ARN", // filled by deploy
			"AWS_REGION":          orStr(opts.Region, "<region>"),
		},
		SecurityGroupIn: sgRules(opts),
		VPCEndpoints:    []string{fmt.Sprintf("com.amazonaws.%s.bedrock-agentcore", orStr(opts.Region, "<region>"))},
		PrivateDNS:      dnsRecords(opts),
	}

	// Advisory warnings (not rejections).
	if len(s.Ingestion) > 0 {
		p.Warnings = append(p.Warnings, "ingestion pipelines stay on EKS (K8s Jobs/CronJobs); EventBridge/Lambda migration is a follow-up")
	}
	if s.Agent.Interfaces != nil && s.Agent.Interfaces.Messaging {
		p.Warnings = append(p.Warnings, "agent uses messaging: ensure the standalone messaging service is VPC-reachable and SG-authorized")
	}

	return p, nil
}

// component is a normalized view of a models/knowledge/integrations entry:
// its map name, its provider (empty in container mode), and whether it declares
// a container block. It lets one addComponentEnv handle all three typed maps.
type component struct {
	name      string
	provider  string
	container bool
}

func modelComponents(m map[string]spec.Model) []component {
	out := make([]component, 0, len(m))
	for _, name := range sortedKeys(m) {
		c := m[name]
		out = append(out, component{name: name, provider: c.Provider, container: c.Container != nil})
	}
	return out
}

func knowledgeComponents(m map[string]spec.Knowledge) []component {
	out := make([]component, 0, len(m))
	for _, name := range sortedKeys(m) {
		c := m[name]
		out = append(out, component{name: name, provider: c.Provider, container: c.Container != nil})
	}
	return out
}

func integrationComponents(m map[string]spec.Integration) []component {
	out := make([]component, 0, len(m))
	for _, name := range sortedKeys(m) {
		c := m[name]
		out = append(out, component{name: name, provider: c.Provider, container: c.Container != nil})
	}
	return out
}

// selfHostedProviders are provider bindings that run as an in-cluster container
// reachable over the VPC (not a cloud API). They inject a connection *_HOST the
// wrapper must rewrite to a VPC-resolvable name. Cloud providers instead inject
// a credential secret and need no rewrite.
var selfHostedProviders = map[string]bool{
	"ollama": true, "qdrant": true, "redis": true, "postgres": true,
	"neo4j": true, "chroma": true, "weaviate": true, "pgvector": true,
	"milvus": true, "elasticsearch": true,
}

// cloudProviderSecretEnv maps a managed cloud provider to the env var carrying
// its credential, falling back to {PROVIDER}_API_KEY for unknown providers.
func cloudProviderSecretEnv(provider string) string {
	switch strings.ToLower(provider) {
	case "github":
		return "GITHUB_TOKEN"
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	default:
		return strings.ToUpper(provider) + "_API_KEY"
	}
}

// addComponentEnv injects the env a set of components contributes. `prefix` is
// the container-mode host prefix ("MODEL"/"KNOWLEDGE"; "" for integrations,
// which never run in-cluster). Cloud-provider credential names are recorded in
// `secrets` so they surface as @SECRET: placeholders resolved at deploy.
func addComponentEnv(env map[string]string, secrets map[string]bool, prefix string, comps []component) {
	for _, c := range comps {
		switch {
		case c.container:
			// container-mode → {PREFIX}_{NAME}_HOST (e.g. KNOWLEDGE_CACHE_HOST)
			if prefix != "" {
				key := fmt.Sprintf("%s_%s_HOST", prefix, strings.ToUpper(c.name))
				env[key] = fmt.Sprintf("%s.default.svc.cluster.local", c.name)
			}
		case selfHostedProviders[strings.ToLower(c.provider)]:
			// self-hosted provider → {PROVIDER}_HOST (e.g. NEO4J_HOST, QDRANT_HOST)
			key := strings.ToUpper(c.provider) + "_HOST"
			env[key] = fmt.Sprintf("%s.default.svc.cluster.local", strings.ToLower(c.provider))
		case strings.TrimSpace(c.provider) != "":
			// cloud provider → credential secret placeholder
			sname := cloudProviderSecretEnv(c.provider)
			env[sname] = "@SECRET:" + sname
			secrets[sname] = true
		}
	}
}

// ResolveSecrets replaces every `@SECRET:NAME` placeholder in the runtime env
// with a value from `provided`, and merges any additional provided KEY=VALUE
// pairs (e.g. OPENAI_BASE_URL) as direct env. It returns the sorted names of
// placeholders that had no value, so the caller can fail closed rather than
// deploy a literal `@SECRET:` string. Values are never logged.
func ResolveSecrets(p *Plan, provided map[string]string) (unresolved []string) {
	miss := map[string]bool{}
	for k, v := range p.AgentCore.Env {
		if strings.HasPrefix(v, "@SECRET:") {
			name := strings.TrimPrefix(v, "@SECRET:")
			if val, ok := provided[name]; ok {
				p.AgentCore.Env[k] = val
			} else {
				miss[name] = true
			}
		}
	}
	// Merge extra provided env not present as a placeholder (direct injection).
	for k, v := range provided {
		if _, exists := p.AgentCore.Env[k]; !exists {
			p.AgentCore.Env[k] = v
		}
	}
	return sortedBoolKeys(miss)
}

// rewriteEnv replaces any in-cluster substring in env values per the mapping,
// recording each change (for auditability and the "no .svc.cluster.local leaks"
// invariant).
func rewriteEnv(env map[string]string, hosts map[string]string) []EnvRewrite {
	var rewrites []EnvRewrite
	for _, k := range sortedKeys(env) {
		orig := env[k]
		val := orig
		for _, from := range sortedKeys(hosts) {
			if strings.Contains(val, from) {
				val = strings.ReplaceAll(val, from, hosts[from])
			}
		}
		if val != orig {
			env[k] = val
			rewrites = append(rewrites, EnvRewrite{Key: k, From: orig, To: val})
		}
	}
	return rewrites
}

func sgRules(opts Options) []SGRule {
	if len(opts.SecurityGroups) == 0 {
		return nil
	}
	from := opts.SecurityGroups[0]
	return []SGRule{
		{FromSG: from, Port: 9090, Note: "messaging gRPC — allow inbound from AgentCore ENI SG"},
	}
}

func dnsRecords(opts Options) []DNSRecord {
	var recs []DNSRecord
	for _, from := range sortedKeys(opts.DependencyHosts) {
		recs = append(recs, DNSRecord{
			Name:   opts.DependencyHosts[from],
			Target: from,
			Note:   "front the in-cluster service with an internal LB / private Route 53 name",
		})
	}
	return recs
}

// --- helpers ---

func sortedInputs(m map[string]spec.Input) []spec.Input {
	out := make([]spec.Input, 0, len(m))
	for _, k := range sortedKeys(m) {
		out = append(out, m[k])
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedBoolKeys returns the true-valued keys of a set, sorted.
func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// sanitizeName coerces a name into AgentCore's runtime-name contract:
// [a-zA-Z][a-zA-Z0-9_]{0,47} — must start with a letter, only letters/digits/
// underscore, max 48 chars. Hyphens and other punctuation become underscores.
func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if i >= 47 {
			break
		}
	}
	out := b.String()
	if out == "" || !isLetter(rune(out[0])) {
		out = "a" + out
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func orStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
