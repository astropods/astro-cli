// API client for communicating with the astro-server backend.
//
// File layout: types are declared top-down in the same domain order as the
// methods on `ApiClient` below. Each section is fronted by a banner so the
// types and methods for a domain are easy to navigate together.

import type { LogEntry } from "./log-utils";
import {
  buildBlueprintListQuery,
  BLUEPRINT_LIST_MAX_LIMIT,
  type BlueprintListParams,
} from "./blueprint-list-params";

function buildQS(params?: Record<string, string | undefined>): string {
  if (!params) return '';
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) qs.set(key, value);
  }
  const encoded = qs.toString();
  return encoded ? `?${encoded}` : '';
}

// ============================================================================
// Core: errors
// ============================================================================

export interface ValidationError {
  field: string;
  message: string;
}

export interface ApiError {
  error?: string;
  error_description?: string;
  code?: string;
  details?: string;
  status?: number;
  validation_errors?: ValidationError[];
  missing_variables?: string[];
}

export class ApiRequestError extends Error {
  status: number;
  code?: string;
  details?: string;
  validation_errors?: ValidationError[];
  missing_variables?: string[];

  constructor(apiError: ApiError, status: number) {
    super(apiError.error_description || apiError.details || apiError.error || `Request failed with status ${status}`);
    this.name = 'ApiRequestError';
    this.status = status;
    this.code = apiError.error ?? apiError.code;
    this.details = apiError.details;
    this.validation_errors = apiError.validation_errors;
    this.missing_variables = apiError.missing_variables;
  }
}

// ============================================================================
// Avatars (cross-domain primitives — shared by accounts, blueprints, deployments)
// ============================================================================

export interface AvatarColors {
  version?: number;
  base: string;
  vibrant: string;
  vibrant_light: string;
  accent: string;
  accent_light: string;
  background: string;
  foreground: string;
  glow: string;
}

export interface AvatarResponse {
  avatar_url: string;
  avatar_colors?: AvatarColors;
}

// ============================================================================
// Auth & profile
// ============================================================================

export interface User {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  email_verified: boolean;
  profile_picture_url?: string;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  user: User;
  session_id: string;
  organization_id?: string;
  role?: string;
  permissions: string[];
  expires_at: string;
  accounts: Account[];
}

export interface ProfileResponse {
  user: User;
  accounts: Account[];
}

// ============================================================================
// Accounts
// ============================================================================

export interface AccountOwner {
  first_name?: string;
  last_name?: string;
  profile_picture_url?: string;
}

export interface AccountPublic {
  id: string;
  name: string;
  type: string;
  display_name?: string;
  owner?: AccountOwner;
  created_at: string;
  updated_at: string;
  account_number?: number;
  bio?: string;
  location?: string;
  email?: string;
  local_timezone?: string;
  pronouns?: string;
  website?: string;
  social_links?: string[];
  blueprint_order?: string[];
  avatar_colors?: AvatarColors;
}

export interface Account {
  id: string;
  name: string;
  type: string;
  display_name?: string;
  role?: string;
  organization_id?: string; // WorkOS org ID, present on organization accounts
  /** Placement cluster (e.g. "eu"); empty = primary US cluster */
  cluster_id?: string;
  agents?: BlueprintSummary[];
  account_number?: number;
  bio?: string;
  location?: string;
  email?: string;
  local_timezone?: string;
  pronouns?: string;
  website?: string;
  social_links?: string[];
}

export interface AccountOrg {
  name: string;
  display_name?: string;
  avatar_colors?: AvatarColors;
}

export interface AccountOrgsResponse {
  orgs: AccountOrg[];
}

export interface AccountSearchResult {
  id: string;
  name: string;
  display_name: string;
  type: string;
}

export interface AccountSearchResponse {
  results: AccountSearchResult[];
}

export interface AccountMember {
  account_id: string;
  user_id: string;
  role: string;
  status: string;
  username: string;
  display_name: string;
  created_at: string;
  /** Slack workspaces this member has linked. Empty = not connected, in
   *  which case a Slack-typed grant for this user won't resolve and the
   *  grants UI surfaces a warning. */
  slack_workspaces: AccountMemberSlackWorkspace[];
}

export interface AccountMemberSlackWorkspace {
  team_id: string;
  team_name: string;
  team_domain: string;
  icon_url: string;
}

export interface AccountMembersResponse {
  members: AccountMember[];
}

export interface InviteEntry {
  value: string;
  kind: 'email' | 'account';
  role?: string;
}

export interface InviteResultResponse {
  value: string;
  kind: string;
  email?: string;
  success: boolean;
  error?: string;
  invitation?: { id: string; email: string };
}

export interface CreateAccountData {
  name: string;
  type: string;
  display_name?: string;
  invitations?: InviteEntry[];
}

export interface CreateAccountResponse {
  id: string;
  name: string;
  type: string;
  invitations?: InviteResultResponse[];
  created_at: string;
  updated_at: string;
}

// ============================================================================
// Blueprints (incl. hearts / archive)
// ============================================================================

export interface BlueprintSummary {
  name: string;
  registry: string;
  build_count: number;
}

export interface BlueprintSpec {
  meta?: { visibility?: string };
  integrations?: Record<string, { provider: string; type?: string }>;
  [key: string]: unknown;
}

export interface BlueprintAuthor {
  name: string;
  account?: string;
}

export interface ResolvedIntegration {
  id: string;
  name: string;
  known: boolean;
}

export interface BlueprintCardRepo {
  type?: string;
  url: string;
  directory?: string;
}

export interface BlueprintCardData {
  description?: string;
  tags?: string[];
  authors?: BlueprintAuthor[];
  repository?: BlueprintCardRepo;
  capabilities?: string[];
  integrations?: ResolvedIntegration[];
  body?: string;
}

export interface BlueprintVersion {
  build_id: string;
  version?: string;
  spec: BlueprintSpec;
  readme?: string;
  agent_card?: BlueprintCardData;
  published_at: string;
  validation_warnings?: ValidationError[];
  /** Git commit that produced this build; present only for GitHub-sourced builds. */
  commit_message?: string;
  commit_sha?: string;
  repo_full_name?: string;
}

export interface BlueprintMetrics {
  lifetime_messages: number;
  deploy_count?: number;
}

export interface Blueprint {
  name: string;
  account: string;
  registry: string;
  visibility?: string;
  avatar_url?: string;
  avatar_colors?: AvatarColors;
  archived_at?: string;
  name_reserved?: boolean;
  versions: BlueprintVersion[];
  draft_card?: BlueprintCardData;
  heart_count?: number;
  hearted?: boolean;
  metrics?: BlueprintMetrics;
  publishers?: BlueprintAuthor[];
}

export interface BlueprintsListResponse {
  agents: Blueprint[];
  count: number;
  limit?: number;
  offset?: number;
  has_more?: boolean;
}

export interface HeartedAgent {
  account: string;
  name: string;
  visibility: string;
  avatar_colors?: AvatarColors;
  heart_count: number;
  deploy_count: number;
  hearted_at: string;
  description?: string;
}

export interface HeartedListResponse {
  items: HeartedAgent[];
  next_cursor?: string;
}

// ============================================================================
// Deployments
// ============================================================================

// --- Deployment / template spec ---

export interface DeploymentVariable {
  value?: string;
  ref?: string; // reference to an account variable by name, resolved server-side
  targets: string[];
  secret?: boolean;
  optional?: boolean;
  // template-only (present in deployment-template/v1, stripped in deployment/v1)
  default?: string;
  description?: string;
  label?: string;
  placeholder?: string;
  help_url?: string;
  icon?: string;
  datatype?: string;
  'display-as'?: string;
  options?: string[];
  fields?: Record<string, VariableField>;
  // Non-empty when the variable is deprecated; carries the migration message
  // the UI surfaces in a tooltip next to a "Deprecated" badge.
  deprecated?: string;
  /** Inline secret stored on deployment; value intentionally omitted from API. */
  configured?: boolean;
}

export interface VariableField {
  label?: string;
  description?: string;
  placeholder?: string;
  datatype?: string;
  optional?: boolean;
  // Non-empty when the sub-field is deprecated; same semantics as
  // DeploymentVariable.deprecated.
  deprecated?: string;
}

export interface DeploymentEndpoint {
  port: number;
  protocol?: 'http' | 'grpc' | 'tcp';
  expose?: { enabled: boolean; domain?: string };
}

export interface DeploymentTrigger {
  type: string;
  schedule?: string;
}

export interface DeploymentIngestion {
  image: string;
  trigger: DeploymentTrigger;
  resources?: Record<string, unknown>;
  environment?: Record<string, string>;
  endpoints?: Record<string, DeploymentEndpoint>;
  healthcheck?: Record<string, unknown>;
}

export interface DeploymentTemplate {
  spec: 'deployment-template/v1';
  source: { account: string; name: string; build: string; registry: string };
  target: { runtime: string; account?: string; display_name?: string; deployment_id?: string };
  agent: Record<string, unknown>;
  models?: Record<string, unknown>;
  knowledge?: Record<string, unknown>;
  tools?: Record<string, unknown>;
  ingestion?: Record<string, DeploymentIngestion>;
  interfaces?: Record<string, unknown>;
  variables?: Record<string, DeploymentVariable>;
  observability?: Record<string, unknown>;
}

export type DeploymentSpec = Omit<DeploymentTemplate, 'spec'> & {
  spec: 'deployment/v1';
};

// --- Interactive POST template ---

export interface TemplateRequest {
  build?: string;
  deployment_id?: string;
  revision?: number;
  interfaces?: TemplateInterfaces;
  variables?: Record<string, { value?: string; ref?: string }>;
  schedules?: Record<string, string>;
  bindings?: {
    knowledge?: Record<string, string>; // entry name → store ARN
  };
  provisioning?: TemplateProvisioning;
  finalize?: boolean;
}

export interface KnowledgeBindingInfo {
  arn: string;
  name: string;
  provider: string;
  status: string;
}

export interface TemplateResponse {
  spec: 'deployment-template/v1';
  template: DeploymentSpec;
  variables: Record<string, DeploymentVariable>;
  interfaces: TemplateInterfaces;
  schedules: Record<string, string>;
  bindings?: {
    knowledge?: Record<string, KnowledgeBindingInfo>;
  };
  provisioning?: TemplateProvisioning;
  validation: TemplateValidation;
  signature?: string;
}

/** Top-level provisioning block — per-component compute/volume overrides.
 *  v1 scopes this to the agent container only. */
export interface TemplateProvisioning {
  agent?: ComponentProvisioning;
}

export interface ComponentProvisioning {
  compute?: ComponentCompute;
  volume?: ComponentVolume;
}

/** Simple compute knobs; the server expands these into K8s requests==limits
 *  (Guaranteed QoS). */
export interface ComponentCompute {
  cpu?: string;
  memory?: string;
}

export interface ComponentVolume {
  mount?: string;
  storage?: { size?: string; class?: string; access_mode?: string };
}

/** Authorization grant — exactly one of org, user_id, or anyone must be set.
 *  Slack rejects user_id (slack identity is opaque). */
export interface AuthGrant {
  /** Organization (account) ID — any current member of this org is allowed. */
  org?: string;
  user_id?: string;
  anyone?: boolean;
}

export interface WebAuthConfig {
  type?: string;
  grants?: AuthGrant[];
}

export interface SlackAuthConfig {
  grants?: AuthGrant[];
}

export interface TemplateInterfaces {
  adapters: string[];
  auth?: {
    web?: WebAuthConfig;
    slack?: SlackAuthConfig;
  };
}

export interface TemplateValidation {
  valid: boolean;
  errors: ValidationError[];
}

// --- Deploy / undeploy responses ---

export interface ResourceStatus {
  kind: string;
  name: string;
  namespace?: string;
  status: string;
  message?: string;
}

export interface ServiceEndpoint {
  name: string;
  type: string;
  url: string;
  port?: number;
}

export interface DeploymentError {
  resource: string;
  kind: string;
  error: string;
}

export interface DeployResponse {
  status: string;
  deployment_id?: string;
  name: string;
  build_id: string;
  k8s_namespace: string;
  deployed_at: string;
  resources: ResourceStatus[];
  service_endpoints?: ServiceEndpoint[];
  errors?: DeploymentError[];
}

export interface ValidateDeploymentResponse {
  valid: boolean;
  build_id?: string;
  name?: string;
  validation_errors?: ValidationError[];
}

export interface UndeployResponse {
  status: string;
  name: string;
  build_id: string;
  k8s_namespace: string;
  undeployed_at: string;
  resources: ResourceStatus[];
  errors?: DeploymentError[];
}

// --- Deployment state / inspection ---

export type DeploymentSummaryStatus = "Running" | "pending" | "Stopped" | "undeploying" | "error";

export interface ServiceEndpointInfo {
  name: string;
  url: string;
  type?: string;
  ready?: boolean;
  message?: string;
}

export interface EnvVar {
  name: string;
  // Plaintext for non-secrets; "••••••••" placeholder when is_secret is true.
  value?: string;
  // Categorical provenance from deployment_build_env: 'user_var' |
  // 'platform_meta' | 'service_url' | 'knowledge_cred' | 'auth_token' |
  // 'adapter_config' | 'derived'.
  source?: string;
  // Authoritative secret flag from deployment_build_env. When true the
  // value is redacted; clients should treat the entry as sensitive.
  is_secret?: boolean;
}

// ContainerStatus is the live K8s state for a single container — no env.
// Env is apply-time intent and lives on WorkloadSpec.env, keyed by role.
export interface ContainerStatus {
  name: string;
  state: string;
  ready: boolean;
  restart_count: number;
  reason?: string;
  message?: string;
}

// WorkloadDetail is the JOINED view of a workload — what the page builds by
// zipping a WorkloadSpec from the record onto its WorkloadRuntime entry from
// the runtime response, keyed by name. Leaf components (PodTile,
// PodDetailPanel) consume this shape; the join lives at the page boundary.
//
// Fields are typed as optional where they only come from one side of the
// join, so a runtime-less render (cluster unreachable, loading) still
// produces a valid object containing just the spec.
export interface WorkloadDetail {
  // Intent (WorkloadSpec):
  name: string;
  kind: string;
  component: string;
  image?: string;
  replicas?: number;
  schedule?: string;
  urls?: ServiceEndpointInfo[];
  // env is keyed by role; clients map a container's
  // (component, container_name) → role via roleFor() in env-utils to look up
  // its env list. The agent workload may carry both "agent" and "messaging"
  // when a messaging sidecar is configured.
  env?: Record<string, EnvVar[]>;
  // Live (WorkloadRuntime):
  age?: string;
  phase?: string;
  pod_name?: string;
  containers?: ContainerStatus[];
  status?: string;
  start_time?: string;
  completions?: string;
  runs?: JobDetail[];
}

// WorkloadSpec is the DB-sourced intent for a single workload — what the
// deployment is supposed to run. Lives on AgentDeployment (the record).
// Live state (age, pod state, restart counts, runs) is on WorkloadRuntime
// in the runtime response, keyed by Name.
export interface WorkloadSpec {
  name: string;
  // "Deployment" | "StatefulSet" | "Job" | "CronJob"
  kind: string;
  component: string;
  // image / replicas are populated on the wire; declared optional so fixtures
  // and Partial<> consumers don't have to set them.
  image?: string;
  replicas?: number;
  schedule?: string;
  urls?: ServiceEndpointInfo[];
  // Env vars keyed by role (e.g. "agent", "messaging", "collector",
  // "knowledge:<name>", "ingestion:<name>"). Most workloads have one role;
  // the agent workload carries both "agent" and "messaging" when a messaging
  // sidecar is configured. Use roleFor(component, containerName) to pick the
  // right entry for a container.
  env?: Record<string, EnvVar[]>;
}

// WorkloadRuntime is the K8s-sourced live state for a single workload,
// keyed by Name to stitch onto the corresponding WorkloadSpec on the record.
export interface WorkloadRuntime {
  name: string;
  age?: string;
  phase?: string;
  pod_name?: string;
  containers?: ContainerStatus[];
  // Job/CronJob-only fields. Empty for Deployment/StatefulSet — their health
  // is read from containers[].ready instead.
  status?: string;
  start_time?: string;
  completions?: string;
  runs?: JobDetail[];
}

export interface JobDetail {
  name: string;
  status: string;
  component: string;
  age: string;
  start_time?: string;
  completions: string;
}

// AgentDeployment is the DB-sourced view of a deployment (GET /deployments/:id).
// Mirrors handlers.DeploymentRecord on the server: spec, URLs, status, intent-
// level workload list — everything we wrote at apply / normalization time.
// Live operational state (ready count, per-pod containers/restarts/age, runs)
// is fetched separately via useDeploymentRuntime and stitched in by Name.
export interface AgentDeployment {
  id: string;
  name: string;
  display_name?: string;
  avatar_url?: string;
  avatar_colors?: AvatarColors;
  build_id: string;
  // Publishing-account name resolved from deployments.source_account_id.
  // Empty/undefined when the deployment is same-account (or pre-migration
  // legacy with no spec.source.account); callers should fall back to the
  // URL/owning account in that case. Use this to look up blueprint
  // upgrade signals against the correct lineage on cross-account deploys.
  source_account?: string;
  // Most-recent published build_id for the agent's lineage. Populated by the
  // server in the deployments list response. Empty when there are no
  // published builds, when the lineage agent is private to another account,
  // or when the lookup fails. UI compares against build_id to decide whether
  // to render the upgrade affordance — saves N blueprint queries on the
  // dashboard.
  latest_build_id?: string;
  namespace: string;
  status: string;
  // Server-populated reason a deployment is in error/failed. Set by the deploy
  // preflight 422 path, the reconcile pod-failure escalation, or any other
  // status writer that recorded a message on deployments.error_message.
  // Surfaced as a tooltip on the status badge.
  error_message?: string;
  // Desired replica count, summed across primary workload specs. Live (observed)
  // count is on DeploymentRuntime.
  replicas: number;
  created_at: string;
  updated_at?: string;
  updated_by?: string;
  components: string[];
  external_urls?: ServiceEndpointInfo[];
  // A messaging sidecar is part of the deployment spec. Distinct from
  // DeploymentRuntime.messaging_reachable, which is the live in-cluster
  // Service existence probe.
  messaging_configured?: boolean;
  workloads?: WorkloadSpec[];
}

// DeploymentStatusValue is the coarse, server-derived status the UI renders.
// Single source of truth: GET /deployments/:id/status.
export type DeploymentStatusValue =
  | "active"
  | "deploying"
  | "inactive"
  | "undeploying"
  | "error";

// DeploymentStatusReason is a stable machine-readable code for *why* the
// server chose `value`. Use it to branch UI (e.g. distinguish "warming up
// pods" from "DB pending") without re-deriving anything client-side. Keep
// in sync with the Status* constants on the server.
export type DeploymentStatusReason =
  | "paused"
  | "undeploying"
  | "failed"
  | "provisioning"
  | "ready"
  | "ready_lag"
  | "cluster_unreachable";

// DeploymentStatus is the body of GET /deployments/:id/status (no envelope).
// Mirrors handlers.DeploymentStatus on the server. Live replica/ready counts
// and per-workload state live on DeploymentRuntime; this stays narrow.
//
// `details` is a human-readable sentence ("3 of 4 replicas ready", "Cluster
// unreachable; reporting active from spec") suitable for tooltips. `reason`
// is the machine-readable companion.
export interface DeploymentStatus {
  value: DeploymentStatusValue;
  reason: DeploymentStatusReason;
  details: string;
  error_message?: string;
}

// DeploymentRuntime is the K8s-sourced live view (GET /deployments/:id/runtime).
// Mirrors handlers.DeploymentRuntime on the server. Frontend stitches
// `workloads` onto AgentDeployment.workloads by `name`.
export interface DeploymentRuntime {
  ready: number;
  replicas: number;
  messaging_reachable: boolean;
  manual_ingestions?: string[];
  workloads?: WorkloadRuntime[];
}

export interface AgentDeploymentSummary {
  id: string;
  name: string;
  display_name?: string;
  avatar_colors?: AvatarColors;
  build_id: string;
  latest_build_id?: string;
  status?: DeploymentSummaryStatus;
  namespace?: string;
  external_urls?: ServiceEndpointInfo[];
  /** True when the deployment has a messaging sidecar with a web (http) service. */
  messaging_web_configured?: boolean;
  created_at: string;
  updated_at?: string;
}

// ============================================================================
// Deployment messaging proxy (via astro-server → sidecar)
// ============================================================================

export interface MessagingCreateConversationResponse {
  conversation_id: string;
  created_at?: string;
}

export interface MessagingSendMessageResponse {
  message_id: string;
  timestamp?: string;
}

export interface MessagingHistoryMessage {
  message_id: string;
  content: string;
  timestamp?: string;
  user?: { id: string; username?: string };
}

export interface DeploymentChatConversationSummary {
  conversation_id: string;
  title: string;
  updated_at: string;
}

export interface DeploymentChatMessageRecord {
  id: string;
  role: "user" | "assistant";
  content: string;
}

export interface ListDeploymentChatConversationsResponse {
  conversations: DeploymentChatConversationSummary[];
}

export interface GetDeploymentChatConversationResponse {
  conversation_id: string;
  title: string;
  updated_at: string;
  messages: DeploymentChatMessageRecord[];
  /** Server-authoritative: the messaging proxy is persisting an assistant reply. */
  assistant_streaming?: boolean;
  has_more?: boolean;
  oldest_seq?: number;
}

export type DeploymentChatConversationQuery = {
  /** Return the last N messages (live refresh). Omit for full thread. */
  limit?: number;
  before_seq?: number;
};

export interface MessagingHistoryResponse {
  conversation_id: string;
  messages: MessagingHistoryMessage[];
  is_complete?: boolean;
  fetched_at?: string;
}

export interface DeploymentsListResponse {
  deployments: AgentDeploymentSummary[];
  count: number;
}

export interface DeploymentSummaryItem {
  id: string;
  name: string;
  display_name?: string;
  status: string;
  avatar_url?: string;
  avatar_colors?: AvatarColors;
}

export interface AccountDeploymentsSummary {
  id: string;
  name: string;
  type: string;
  display_name: string;
  deployments: DeploymentSummaryItem[];
}

export interface DeploymentsSummaryResponse {
  accounts: AccountDeploymentsSummary[];
}

export interface ActiveDeploymentSpecResponse {
  id: string;
  agent_name: string;
  build_id: string;
  namespace: string;
  status: string;
  deployed_at: string;
  spec: Record<string, unknown>;
}

export interface DeploymentHistoryRecord {
  id: string;
  agent_name: string;
  revision: number;
  build_id: string;
  namespace: string;
  display_name: string;
  is_current: boolean;
  status: string;
  deployed_at: string;
  spec: Record<string, unknown>;
  source: "github" | "direct";
  commit_sha?: string;
  branch?: string;
  commit_message?: string;
  repo_full_name?: string;
}

export interface DeploymentHistoryResponse {
  deployments: DeploymentHistoryRecord[];
  count: number;
}

export interface K8sEvent {
  type: "Normal" | "Warning";
  reason: string;
  message: string;
  object_kind: string;
  object_name: string;
  count: number;
  first_timestamp: string;
  last_timestamp: string;
}

export interface DeploymentEventsResponse {
  events: K8sEvent[];
}

// --- Dataset ---

export interface EvalDatasetResponse {
  dataset_name: string;
  item_count: number;
}

// --- Pod metrics (CPU / memory time series) ---

export type PodMetricsRange = "1h" | "6h" | "24h" | "7d";

export interface PodMetricPoint {
  /** ISO 8601 timestamp. */
  timestamp: string;
  /** CPU: vCPU cores. Memory: bytes (working set). */
  value: number;
}

export interface PodMetricsResponse {
  pod: string;
  range: PodMetricsRange;
  /** Bucket size as a Go duration string (e.g. "30s", "10m"). */
  step: string;
  /** Server always returns [] when there's no data, but defend against null
   *  on the off chance the cached payload was produced before the server fix. */
  cpu: PodMetricPoint[] | null;
  memory: PodMetricPoint[] | null;
  /** Sum of `kubelet_volume_stats_used_bytes` across this pod's PVCs.
   *  Empty when the pod mounts no PVC. */
  storage_used: PodMetricPoint[] | null;
  /** Sum of `kubelet_volume_stats_capacity_bytes` across this pod's PVCs.
   *  Empty when the pod mounts no PVC. */
  storage_capacity: PodMetricPoint[] | null;
  /** Pod-level network receive (ingress) throughput, bytes/sec. */
  network_rx: PodMetricPoint[] | null;
  /** Pod-level network transmit (egress) throughput, bytes/sec. */
  network_tx: PodMetricPoint[] | null;
  /** Filesystem read throughput across containers, bytes/sec. */
  fs_read: PodMetricPoint[] | null;
  /** Filesystem write throughput across containers, bytes/sec. */
  fs_write: PodMetricPoint[] | null;
  /** Timestamps (ISO 8601) of container restart events within the window.
   *  Rendered as vertical markers across the CPU/Memory/Storage charts. */
  restarts: string[] | null;
  /** Timestamps (ISO 8601) of OOM-kill events within the window.
   *  Rendered as vertical markers on the Memory chart only. */
  ooms: string[] | null;
  /** Pod-level CPU limit in vCPU cores, summed across regular containers and
   *  Always-restart init sidecars. 0 when unknown or no limit is set. */
  cpu_limit: number;
  /** Pod-level memory limit in bytes, summed the same way as cpu_limit.
   *  0 when unknown or no limit is set. */
  memory_limit: number;
}

// --- ConfigMap / Secret inspection ---

export interface ConfigMapResponse {
  name: string;
  namespace: string;
  data: Record<string, string>;
}

export interface SecretKeysResponse {
  name: string;
  namespace: string;
  keys: string[];
}

// ============================================================================
// Observability (deployment- and account-scoped, backed by Langfuse)
// ============================================================================

export interface MetricsBucket {
  timestamp: string;
  trace_count: number;
  avg_latency_ms: number;
  p95_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  error_count: number;
}

export interface ObservabilityMetricsResponse {
  buckets: MetricsBucket[];
  time_range: { start: string; end: string };
  interval_minutes: number;
}

export interface DeploymentSummaryEntry {
  total_traces: number;
  last_trace_at: string;
  /** 30-day daily request counts, oldest → newest. Server pads days with no
   *  activity to zero so consumers can rely on a fixed-length array. */
  request_series?: number[];
  /** 30-day daily token totals (input + output), oldest → newest. */
  token_series?: number[];
}

export interface DeploymentSummariesResponse {
  summaries: Record<string, DeploymentSummaryEntry>;
}

export interface ObservabilitySummaryResponse {
  total_traces: number;
  time_range: { start: string; end: string };
  metrics: {
    avg_latency_ms: number;
    p95_latency_ms: number;
    total_tokens: number;
    error_rate: number;
    traces_per_hour: number;
  };
}

export interface AccountObservabilitySummaryResponse {
  period: { start: string; end: string; days: number };
  totals: {
    cost_usd: number;
    requests: number;
    /** @deprecated prefer total_tokens; kept for back-compat. May be 0 in
     *  views derived from the traces view (combined-only). */
    input_tokens: number;
    /** @deprecated prefer total_tokens; kept for back-compat. May be 0 in
     *  views derived from the traces view (combined-only). */
    output_tokens: number;
    /** Combined token count. New source of truth for token totals. */
    total_tokens: number;
    active_agents: number;
  };
  daily_avg: { cost_usd: number; requests: number; tokens: number };
  change?: {
    cost_pct: number | null;
    requests_pct: number | null;
    tokens_pct: number | null;
  } | null;
  cost_over_time: Array<{
    date: string;
    models: Array<{ model: string; cost_usd: number }>;
  }>;
  cost_by_model: Array<{ model: string; cost_usd: number; cost_pct: number }>;
  sparklines?: { cost: number[]; requests: number[]; tokens: number[] };
  /** Present only when the endpoint was called with ?group_by=user. */
  cost_over_time_by_user?: Array<{
    date: string;
    users: Array<UserIdentity & { cost_usd: number; requests: number; tokens: number }>;
  }>;
  /** True when the upstream Langfuse query failed (e.g. ClickHouse outage).
   *  Payload is zero-valued; render a "metrics temporarily unavailable" banner. */
  metrics_unavailable?: boolean;
}

export interface AccountUsersSummaryResponse {
  users: Array<UserIdentity & {
    requests: number;
    cost_usd: number;
    /** Combined input + output tokens. Traces view only exposes the sum. */
    tokens: number;
    /** RFC3339 timestamp of the user's most recent hour-bucket with activity.
     *  Omitted when the user had no activity in the period. */
    last_seen?: string;
    /** One entry per deployment the user has touched — two deployments of
     *  the same blueprint produce two refs (identical name/account, distinct
     *  deployment_id). Each entry carries the publishing account so the
     *  client can build the correct avatar URL — public-blueprint deploys
     *  resolve under the original publisher's account, not the deploying org.
     *  deployment_id drives per-deployment click-through and tooltip
     *  enrichment (display_name + namespace via the deployments-summary
     *  response). */
    agents_used: Array<{ deployment_id: string; name: string; account: string }>;
  }>;
  period: { start: string; end: string; days: number };
  /** See note on AccountObservabilitySummaryResponse.metrics_unavailable. */
  metrics_unavailable?: boolean;
}

/** UserDetailsKind is the discriminator for {@link UserDetails}. Every row
 *  that represents a user carries exactly one. The server determines this
 *  from the user_id shape — bare Slack id → "slack", WorkOS-prefixed id →
 *  "astro", anything else → "unknown". */
export type UserDetailsKind = "astro" | "slack" | "unknown";

/** UserDetails is the discriminated identity payload for a user row.
 *  `kind` is always present; the Slack-specific fields (team_id, profile)
 *  populate only when `kind === "slack"`, and even then only when the
 *  Slack directory has metadata for the user. */
export interface UserDetails {
  kind: UserDetailsKind;
  /** Slack team (workspace) id — required for the slack:// deep link. */
  team_id?: string;
  /** Slack profile metadata. */
  display_name?: string;
  username?: string;
  avatar_url?: string;
  is_bot?: boolean;
  deleted?: boolean;
}

/** UserIdentity is the user_id + user_details pair the server surfaces
 *  for every per-user row (users-summary, cost_over_time_by_user,
 *  users_used_details). */
export interface UserIdentity {
  user_id: string;
  user_details: UserDetails;
}

export interface AccountDeploymentsSummaryResponse {
  deployments: Array<{
    deployment_id: string;
    agent_name: string;
    display_name?: string;
    namespace?: string;
    requests: number;
    cost_usd: number;
    cost_per_request: number;
    /** @deprecated prefer total_tokens. */
    input_tokens: number;
    /** @deprecated prefer total_tokens. */
    output_tokens: number;
    /** Combined token count for the deployment. New source of truth. */
    total_tokens: number;
    tok_per_request: number;
    p95_latency_ms: number;
    top_model: string;
    cost_over_time?: Array<{ date: string; cost_usd: number }>;
    requests_over_time?: Array<{ date: string; requests: number }>;
    tokens_over_time?: Array<{
      date: string;
      input_tokens: number;
      output_tokens: number;
      /** Combined per-day token count. Prefer over input+output. */
      total_tokens: number;
    }>;
    /** WorkOS user IDs that drove >=1 trace against this deployment in the period. */
    users_used: string[];
    /** Display-ready identities for the Used by column. Prefer over users_used when present. */
    users_used_details?: UserIdentity[];
    /** RFC3339 timestamp set when the deployment has been soft-deleted (status
     *  flipped to 'undeployed'). Optional date for the tombstone caption -
     *  can be missing even on archived rows (e.g. status='undeploying'
     *  mid-tear-down). Don't use this as the tombstone signal; use
     *  is_archived instead. */
    undeployed_at?: string | null;
    /** True when this entry corresponds to a deployment not in the visible
     *  list (i.e. archived). Source of truth for the tombstone styling -
     *  set independently of undeployed_at. */
    is_archived?: boolean;
  }>;
  period: { start: string; end: string; days: number };
  /** See note on AccountObservabilitySummaryResponse.metrics_unavailable. */
  metrics_unavailable?: boolean;
}

export interface InsightsIdentityRef {
  kind: 'agent' | 'member' | 'slack' | 'unidentified' | 'system';
  identity_key?: string;
  id?: string;
  label: string;
  href?: string;
  avatar_account?: string;
  avatar_name?: string;
  avatar_handle?: string;
  user_id?: string;
  user_details?: UserDetails;
  tooltip?: string;
}

export interface InsightsAgentChip {
  key: string;
  label: string;
  href: string;
  avatar_account: string;
  avatar_name: string;
  is_deleted?: boolean;
}

export interface InsightsStatCards {
  totals: AccountObservabilitySummaryResponse['totals'];
  change?: AccountObservabilitySummaryResponse['change'];
}

export interface InsightsAgentRow {
  key: string;
  search_text: string;
  identity: InsightsIdentityRef;
  used_by: InsightsIdentityRef[];
  metrics: {
    requests: number;
    cost_usd: number;
    cost_pct: number;
    cost_per_request: number;
    tok_per_request: number;
    p95_latency_ms: number;
  };
  not_instrumented?: boolean;
}

export interface InsightsPersonRow {
  key: string;
  search_text: string;
  identity: InsightsIdentityRef;
  agents_used: InsightsAgentChip[];
  metrics: {
    requests: number;
    cost_usd: number;
    cost_pct: number;
    tokens: number;
    last_seen?: string;
  };
}

export interface InsightsQueryParams {
  [key: string]: string | undefined;
  q?: string;
  agents_limit?: string;
  agents_offset?: string;
  agents_sort?: string;
  agents_direction?: string;
  people_limit?: string;
  people_offset?: string;
  people_sort?: string;
  people_direction?: string;
  skip_ranges?: string;
}

export interface InsightsTablePagination {
  limit: number;
  offset: number;
  total_count: number;
  filtered_count: number;
  has_more: boolean;
}

export interface InsightsResponse {
  metrics_unavailable?: boolean;
  ranges: Record<string, {
    days: number;
    period: { start: string; end: string; days: number };
    stat_cards: InsightsStatCards;
    agent_spend_chart: AccountObservabilitySummaryResponse['cost_over_time'];
    people_spend_chart: Array<{ date: string; users: number; cost: number }>;
    series_labels: Record<string, string>;
  }>;
  tables: {
    agents: {
      rows: InsightsAgentRow[];
      total_cost: number;
      count: number;
      pagination: InsightsTablePagination;
    };
    people: {
      rows: InsightsPersonRow[];
      total_cost: number;
      count: number;
      missing_slack_details_count?: number;
      pagination: InsightsTablePagination;
    };
  };
}

export interface TraceEntry {
  trace_id: string;
  name: string;
  status: string;
  latency_ms: number;
  total_tokens?: number;
  total_cost?: number;
  input: string;
  output: string;
  timestamp: string;
  user_id?: string;
  user_details?: UserDetails;
}

export interface ObservabilityTracesResponse {
  traces: TraceEntry[];
  total: number;
  limit: number;
  offset: number;
}

export type ObservationType = 'span' | 'generation' | 'event';

export type ObservationLevel = 'debug' | 'default' | 'warning' | 'error';

export interface ObservationUsage {
  input: number;
  output: number;
  total: number;
  unit?: string;
}

export interface TraceObservation {
  id: string;
  parent_id?: string;
  type: ObservationType;
  name: string;
  start_time: string;
  end_time?: string;
  latency_ms: number;
  level?: ObservationLevel;
  status_message?: string;
  input?: unknown;
  output?: unknown;
  metadata?: Record<string, unknown>;
  cost?: number;
  model?: string;
  model_parameters?: Record<string, unknown>;
  usage?: ObservationUsage;
}

export interface TraceScore {
  id: string;
  name: string;
  value: number;
  string_value?: string;
  data_type?: 'numeric' | 'categorical' | 'boolean' | string;
  comment?: string;
  observation_id?: string;
  source?: string;
  created_at?: string;
}

export interface TraceDetail {
  trace_id: string;
  name: string;
  timestamp: string;
  latency_ms: number;
  total_cost: number;
  input: unknown;
  output: unknown;
  session_id?: string;
  user_id?: string;
  user_details?: UserDetails;
  tags?: string[];
  metadata?: Record<string, unknown>;
  environment?: string;
  release?: string;
  version?: string;
}

export interface TraceDetailResponse {
  trace: TraceDetail;
  observations: TraceObservation[];
  scores: TraceScore[];
}

export interface TriggerIngestionResponse {
  status: string;
  job_name: string;
  namespace: string;
}

// ============================================================================
// Network (Beyla eBPF)
// ============================================================================

export type NetworkDirection = 'inbound' | 'outbound' | 'database';
export type NetworkMetric = 'rate' | 'errors' | 'latency_p95' | 'bytes';
export type NetworkGroupBy = 'peer' | 'status_class';
export type NetworkFlowsSort = 'requests' | 'latency_p95' | 'errors';

export interface NetworkDirectionSummary {
  request_count: number;
  error_count: number;
  error_rate: number;
  latency_p50_ms: number | null;
  latency_p95_ms: number | null;
  latency_p99_ms: number | null;
  unique_peer_count: number;
  bytes_total: number;
}

export interface NetworkSummaryResponse {
  inbound: NetworkDirectionSummary;
  outbound: NetworkDirectionSummary;
  database: NetworkDirectionSummary;
  window_from: string;
  window_to: string;
}

export interface NetworkFlow {
  peer: string;
  peer_kind: 'route' | 'address' | 'db_system';
  request_count: number;
  error_count: number;
  error_rate: number;
  latency_p50_ms: number | null;
  latency_p95_ms: number | null;
  bytes_total: number;
  status_codes?: Record<string, number>;
}

export interface NetworkFlowsResponse {
  direction: NetworkDirection;
  flows: NetworkFlow[];
}

export interface NetworkPoint {
  timestamp: string;
  value: number;
}

export interface NetworkSeries {
  label: string;
  points: NetworkPoint[];
}

export interface NetworkTimeseriesResponse {
  direction: NetworkDirection;
  metric: NetworkMetric;
  step: string;
  series: NetworkSeries[];
}

// ============================================================================
// Knowledge
// ============================================================================

export type KnowledgeProvider = 'postgres' | 'qdrant' | 'redis' | 'neo4j' | 'pinecone' | 'mysql';
export type KnowledgeMode = 'managed' | 'external';
export type KnowledgeStatus = 'provisioning' | 'connecting' | 'pending-acceptance' | 'ready' | 'error';

export interface KnowledgeEndpoint {
  cloud_provider: string;
  endpoint_service: string;
  region: string;
  endpoint_id?: string;
  endpoint_dns?: string;
  status: string;
  error?: string | null;
}

export interface KnowledgeEvent {
  type: 'Normal' | 'Warning';
  reason: string;
  message: string;
  count: number;
  timestamp?: string;
}

export interface BoundAgent {
  deployment_id: string;
  agent_name: string;
  display_name?: string;
  knowledge_name: string;
}

export interface KnowledgeStore {
  id: string;
  arn: string;
  name: string;
  provider: KnowledgeProvider;
  mode: KnowledgeMode;
  status: KnowledgeStatus;
  storage?: string;
  public?: boolean;
  public_host?: string;
  endpoint?: KnowledgeEndpoint;
  error?: string | null;
  created_at: string;
  updated_at: string;
  events?: KnowledgeEvent[];
  bound_agents?: BoundAgent[];
}

export type KnowledgeStoreListResponse = KnowledgeStore[];

export interface KnowledgeMetrics {
  cpu_cores: number | null;
  memory_bytes: number | null;
  storage_used: number | null;
  storage_total: number | null;
  uptime_seconds: number;
}

export interface CreateKnowledgeStoreInput {
  name: string;
  provider: KnowledgeProvider;
  storage?: string;
  public?: boolean;
}

export interface ConnectKnowledgeStoreInput {
  name: string;
  provider: KnowledgeProvider;
  host: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
  api_key?: string;
  private_link?: boolean;
  skip_health_check?: boolean;
}

export type KnowledgeCredentials = Record<string, string>;

// ============================================================================
// GitHub
// ============================================================================

export interface GitHubRepo {
  full_name: string;
  default_branch: string;
  private: boolean;
}

export interface GitHubBuildComponent {
  id: number;
  component_name: string;
  status: 'pending' | 'building' | 'succeeded' | 'failed';
  logs?: string;
  started_at?: string;
  completed_at?: string;
}

export interface GitHubBuild {
  id: string;
  build_id: string;
  commit_sha: string;
  branch: string;
  status: 'pending' | 'building' | 'registered' | 'failed' | 'skipped';
  step?: string;
  commit_message?: string;
  commit_author?: string;
  error?: string;
  enqueued_at: string;
  completed_at?: string;
  components?: GitHubBuildComponent[];
}

export interface GitHubStatusResponse {
  connected: boolean;
  repo_full_name?: string;
  branch?: string;
  builds: GitHubBuild[];
  draft_card?: BlueprintCardData;
}

export interface GitHubConnectResponse {
  connected?: boolean;
  redirect_url?: string;
  github_login?: string;
}

export interface GitHubLinkInput {
  repo_full_name: string;
  branch: string;
}

// ============================================================================
// Slack
//
// Account-level Slack identity link via WorkOS Pipes. The mapping that backs
// per-user grants on slack lives in slack_identity_mappings and is populated
// by the callback handler.
// ============================================================================

/** One Slack workspace linked to the current user. The pair (team_id,
 *  slack_user_id) is the unique key; the rest are display fields captured
 *  from oauth.v2.access + team.info at link time. `icon` is the
 *  workspace's avatar URL (empty when the workspace uses slack's default
 *  icon — the UI falls back to a generic Slack svg in that case). */
export interface SlackWorkspace {
  team_id: string;
  slack_user_id: string;
  team?: string;
  team_domain?: string;
  icon?: string;
  slack_username?: string;
}

/** GET /api/v1/accounts/:account/slack — returns every active Slack
 *  workspace mapping for the current user. Empty list = not connected. */
export interface SlackStatusResponse {
  workspaces: SlackWorkspace[];
}

/** POST /api/v1/accounts/:account/slack/connect — returns the OAuth URL
 *  the frontend navigates to. Always a fresh redirect; no short-circuit. */
export interface SlackConnectResponse {
  redirect_url: string;
}

// ============================================================================
// Variables (account-level vault — separate from per-deployment DeploymentVariable)
// ============================================================================

export interface AccountVariable {
  name: string;
  secret: boolean;
  description: string;
  created_at: string;
  updated_at: string;
  value?: string; // only present for non-secret variables
}

export interface AccountVariablesListResponse {
  variables: AccountVariable[];
}

export interface CreateAccountVariableInput {
  name: string;
  value: string;
  secret: boolean;
  description?: string;
}

export interface CreateVariableResult {
  name: string;
  status: 'created' | 'error';
  error?: string;
}

export interface CreateAccountVariablesResponse {
  results: CreateVariableResult[];
}

export interface UpdateAccountVariableInput {
  value?: string;
  secret?: boolean;
  description?: string;
}

// ============================================================================
// Audit log
// ============================================================================

export interface AuditLogActor {
  id: string;
  type: string;
}

export interface AuditLogResource {
  type: string;
  id: string;
  name?: string;
}

export interface AuditLogEntry {
  id: number;
  actor: AuditLogActor;
  action: string;
  resource: AuditLogResource;
  description?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface AuditLogListResponse {
  entries: AuditLogEntry[];
  has_more: boolean;
  next_before?: string;
}

export interface AuditLogQueryParams {
  limit?: number;
  before?: string;
  actor_id?: string;
  resource_type?: string;
  action?: string;
}

export interface AuditLogFilterOptions {
  resource_types: string[];
  actions: string[];
}

// ============================================================================
// Usage / quota / feedback
// ============================================================================

export interface UsageMeter {
  usage: number;
  quota?: number;
}

export interface AccountUsageResponse {
  account_id: string;
  period_start: string;
  period_end: string;
  meters: Record<string, UsageMeter>;
}

export interface FeedbackInput {
  message: string;
  page_url?: string;
}

export interface QuotaIncreaseInput {
  feature_key: string;
  current_usage: number;
  current_quota?: number;
  requested_amount?: number;
  reason: string;
}

export interface QuotaIncreaseListItem {
  id: string;
  feature_key: string;
  current_usage: number;
  current_quota?: number;
  requested_amount?: number;
  reason: string;
  status: string;
  created_at: string;
}

export interface QuotaIncreaseListResponse {
  requests: QuotaIncreaseListItem[];
}

// ============================================================================
// ApiClient
// ============================================================================

class ApiClient {
  private baseUrl: string;
  private authUrl: string;
  private defaultHeaders: Record<string, string>;

  constructor(baseUrl: string = '', authUrl: string = '', defaultHeaders: Record<string, string> = {}) {
    this.baseUrl = baseUrl;
    this.authUrl = authUrl;
    this.defaultHeaders = defaultHeaders;
  }

  // --------------------------------------------------------------------------
  // Fetch infrastructure
  // --------------------------------------------------------------------------

  /** Single entry point for all HTTP traffic. Handles credential cookies,
   *  header merging, error mapping to ApiRequestError, and empty-body
   *  responses. Callers shouldn't reach for `fetch` directly. */
  private async _fetch<T>(
    url: string,
    init: RequestInit = {},
    opts: { isFormData?: boolean } = {},
  ): Promise<T> {
    const headers: Record<string, string> = {
      ...this.defaultHeaders,
      ...(init.headers as Record<string, string> | undefined),
    };
    // FormData uploads must let the browser set the multipart Content-Type
    // (including the boundary). For JSON traffic, default Content-Type unless
    // the caller already provided one.
    if (!opts.isFormData && headers['Content-Type'] === undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const response = await fetch(url, {
      ...init,
      credentials: 'include',
      headers,
    });

    if (!response.ok) {
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
    }

    const text = await response.text();
    if (!text) return {} as T;
    return JSON.parse(text);
  }

  private request<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
    return this._fetch<T>(`${this.baseUrl}${endpoint}`, init);
  }

  // Auth endpoints go directly to the backend (bypassing the Vite proxy) so
  // session cookies are written on the correct origin.
  private authRequest<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
    return this._fetch<T>(`${this.authUrl}${endpoint}`, init);
  }

  private uploadFormData<T>(endpoint: string, formData: FormData): Promise<T> {
    return this._fetch<T>(
      `${this.baseUrl}${endpoint}`,
      { method: 'POST', body: formData },
      { isFormData: true },
    );
  }

  // --------------------------------------------------------------------------
  // Auth
  // --------------------------------------------------------------------------

  async getCurrentUser(): Promise<AuthResponse> {
    return this.authRequest<AuthResponse>('/auth/me');
  }

  async refreshSession(): Promise<AuthResponse> {
    return this.authRequest<AuthResponse>('/auth/refresh', { method: 'POST' });
  }

  async switchOrg(organizationId: string): Promise<AuthResponse> {
    return this.authRequest<AuthResponse>('/auth/switch-org', {
      method: 'POST',
      body: JSON.stringify({ organization_id: organizationId }),
    });
  }

  getLoginUrl(redirect?: string, screenHint?: string): string {
    const params = new URLSearchParams();
    if (redirect) params.set("redirect", redirect);
    if (screenHint) params.set("screen_hint", screenHint);
    const qs = params.toString();
    return `${this.authUrl}/auth/login${qs ? `?${qs}` : ""}`;
  }

  getLogoutUrl(): string {
    // Pass current origin as redirect parameter for reliable post-logout redirect
    const origin = typeof window !== 'undefined' ? window.location.origin : '';
    const redirectUrl = encodeURIComponent(origin);
    return `${this.authUrl}/auth/logout?redirect=${redirectUrl}`;
  }

  // --------------------------------------------------------------------------
  // Profile
  // --------------------------------------------------------------------------

  async getProfile(): Promise<ProfileResponse> {
    return this.request<ProfileResponse>('/api/v1/me');
  }

  async updateProfile(data: { display_name: string }): Promise<{ user: User }> {
    return this.request<{ user: User }>('/api/v1/me', {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  // --------------------------------------------------------------------------
  // Accounts
  // --------------------------------------------------------------------------

  async createAccount(data: CreateAccountData): Promise<CreateAccountResponse> {
    return this.request<CreateAccountResponse>('/api/v1/accounts', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteAccount(account: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}`,
      { method: 'DELETE' }
    );
  }

  async renameAccount(account: string, newName: string): Promise<{ message: string; name: string }> {
    return this.request<{ message: string; name: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}`,
      {
        method: 'PUT',
        body: JSON.stringify({ name: newName }),
      }
    );
  }

  async updateAccountDisplayName(account: string, displayName: string): Promise<{ message: string; display_name: string }> {
    return this.request<{ message: string; display_name: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ display_name: displayName }),
      }
    );
  }

  async updateAccountProfile(
    account: string,
    data: { bio?: string; location?: string; email?: string; local_timezone?: string; pronouns?: string; website?: string; social_links?: string[]; blueprint_order?: string[] },
  ): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}`,
      { method: 'PATCH', body: JSON.stringify(data) },
    );
  }

  async getAccount(name: string): Promise<AccountPublic> {
    return this.request<AccountPublic>(
      `/api/v1/accounts/${encodeURIComponent(name)}`
    );
  }

  async getAccountOrgs(account: string): Promise<AccountOrgsResponse> {
    return this.request<AccountOrgsResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/orgs`,
    );
  }

  async getAccountMembers(account: string, opts?: { includePending?: boolean }): Promise<AccountMembersResponse> {
    const params = opts?.includePending ? '?include_pending=true' : '';
    return this.request<AccountMembersResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/members${params}`
    );
  }

  async updateMemberRole(account: string, userId: string, role: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/members/${encodeURIComponent(userId)}`,
      {
        method: 'PUT',
        body: JSON.stringify({ role }),
      }
    );
  }

  async removeAccountMember(account: string, userId: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/members/${encodeURIComponent(userId)}`,
      { method: 'DELETE' }
    );
  }

  async createInvitations(account: string, invitations: InviteEntry[]): Promise<{ results: InviteResultResponse[] }> {
    return this.request<{ results: InviteResultResponse[] }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/invitations`,
      {
        method: 'POST',
        body: JSON.stringify({ invitations }),
      }
    );
  }

  async checkAccountName(name: string): Promise<{ available: boolean; reason?: string }> {
    return this.request<{ available: boolean; reason?: string }>(
      `/api/v1/accounts/check/${encodeURIComponent(name)}`
    );
  }

  async searchAccounts(
    q: string,
    opts?: { type?: 'personal' | 'organization'; limit?: number }
  ): Promise<AccountSearchResponse> {
    const params = new URLSearchParams({ q });
    if (opts?.type) params.set('type', opts.type);
    if (opts?.limit) params.set('limit', String(opts.limit));
    return this.request<AccountSearchResponse>(
      `/api/v1/accounts/search?${params}`
    );
  }

  // --------------------------------------------------------------------------
  // Blueprints (incl. hearts / archive)
  // --------------------------------------------------------------------------

  async listBlueprints(params?: BlueprintListParams): Promise<BlueprintsListResponse> {
    const qs = buildBlueprintListQuery({
      limit: BLUEPRINT_LIST_MAX_LIMIT,
      offset: 0,
      ...params,
    });
    return this.request<BlueprintsListResponse>(`/api/v1/agents?${qs}`);
  }

  /** Defaults to limit=100 for callers that need a full list in one request (no pagination). */
  async listAccountBlueprints(
    account: string,
    params?: BlueprintListParams,
  ): Promise<BlueprintsListResponse> {
    const qs = buildBlueprintListQuery({
      limit: BLUEPRINT_LIST_MAX_LIMIT,
      offset: 0,
      ...params,
    });
    const base = `/api/v1/agents/${encodeURIComponent(account)}`;
    return this.request<BlueprintsListResponse>(`${base}?${qs}`);
  }

  async getBlueprint(account: string, name: string): Promise<Blueprint> {
    return this.request<Blueprint>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}`
    );
  }

  async createBlueprint(account: string, body: { name: string; visibility?: string }): Promise<{ account: string; name: string }> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}`,
      { method: 'POST', body: JSON.stringify(body) }
    );
  }

  async archiveBlueprint(account: string, name: string): Promise<void> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/archive`,
      { method: 'POST' }
    );
  }

  async toggleHeart(account: string, name: string): Promise<{ hearted: boolean; heart_count: number }> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/heart`,
      { method: 'POST' }
    );
  }

  async listHearted(account: string, cursor?: string): Promise<HeartedListResponse> {
    const params = new URLSearchParams({ limit: '20' });
    if (cursor) params.set('cursor', cursor);
    return this.request(`/api/v1/accounts/${encodeURIComponent(account)}/hearts?${params}`);
  }

  // --------------------------------------------------------------------------
  // Deployments: lifecycle
  // --------------------------------------------------------------------------

  async deployAgent(deploySpec: DeploymentSpec, opts?: { signature?: string }): Promise<DeployResponse> {
    const headers: Record<string, string> = {};
    if (opts?.signature) headers['X-Template-Signature'] = opts.signature;
    return this.request<DeployResponse>('/api/v1/deploy', {
      method: 'POST',
      body: JSON.stringify(deploySpec),
      headers,
    });
  }

  async validateDeployment(deploySpec: DeploymentSpec, opts?: { signature?: string }): Promise<ValidateDeploymentResponse> {
    const headers: Record<string, string> = {};
    if (opts?.signature) headers['X-Template-Signature'] = opts.signature;
    return this.request<ValidateDeploymentResponse>('/api/v1/deploy/validate', {
      method: 'POST',
      body: JSON.stringify(deploySpec),
      headers,
    });
  }

  async undeployAgent(data: {
    deployment_id: string;
  }): Promise<UndeployResponse> {
    return this.request<UndeployResponse>('/api/v1/undeploy', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async stopDeployment(data: { deploymentId: string }): Promise<{ status: string; deployment_id: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/stop`, {
      method: "POST",
    });
  }

  async restartDeployment(data: { deploymentId: string }): Promise<{ status: string; pods: string[] }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/restart`, {
      method: "POST",
    });
  }

  async restartPod(data: { deploymentId: string; podName: string }): Promise<{ status: string; pod: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/pods/${encodeURIComponent(data.podName)}/restart`, {
      method: "POST",
    });
  }

  async wakeupDeployment(data: { deploymentId: string }): Promise<{ status: string; deployment_id: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/wakeup`, {
      method: "POST",
    });
  }

  // Interactive POST deployment template: accepts deploy-time inputs, shapes template, returns validation.
  async postDeploymentTemplate(account: string, name: string, body: TemplateRequest = {}): Promise<TemplateResponse> {
    return this.request<TemplateResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment-template`,
      { method: "POST", body: JSON.stringify(body) },
    );
  }

  // --------------------------------------------------------------------------
  // Deployments: inspection (status, history, logs, events, pods, configmaps)
  // --------------------------------------------------------------------------

  async countDeployments(account: string): Promise<{ count: number }> {
    return this.request<{ count: number }>(
      `/api/v1/deployments/count?account=${encodeURIComponent(account)}`
    );
  }

  async getDeploymentsSummary(): Promise<DeploymentsSummaryResponse> {
    return this.request<DeploymentsSummaryResponse>('/api/v1/deployments/summary');
  }

  async listDeployments(account: string): Promise<DeploymentsListResponse> {
    return this.request<DeploymentsListResponse>(
      `/api/v1/deployments?account=${encodeURIComponent(account)}`
    );
  }

  async getDeployment(id: string): Promise<{ deployment: AgentDeployment }> {
    return this.request<{ deployment: AgentDeployment }>(
      `/api/v1/deployments/${encodeURIComponent(id)}`
    );
  }

  async getDeploymentRuntime(id: string): Promise<{ runtime: DeploymentRuntime }> {
    return this.request<{ runtime: DeploymentRuntime }>(
      `/api/v1/deployments/${encodeURIComponent(id)}/runtime`
    );
  }

  async getDeploymentStatus(id: string): Promise<DeploymentStatus> {
    return this.request<DeploymentStatus>(
      `/api/v1/deployments/${encodeURIComponent(id)}/status`
    );
  }

  messagingProxyPath(deploymentId: string, subpath: string): string {
    const base = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/messaging`;
    return subpath ? `${base}/${subpath.replace(/^\//, "")}` : base;
  }

  async createMessagingConversation(
    deploymentId: string,
  ): Promise<MessagingCreateConversationResponse> {
    return this.request<MessagingCreateConversationResponse>(
      this.messagingProxyPath(deploymentId, "conversations"),
      { method: "POST", body: JSON.stringify({}) },
    );
  }

  async sendMessagingMessage(
    deploymentId: string,
    conversationId: string,
    content: string,
  ): Promise<MessagingSendMessageResponse> {
    return this.request<MessagingSendMessageResponse>(
      this.messagingProxyPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}/messages`,
      ),
      { method: "POST", body: JSON.stringify({ content }) },
    );
  }

  messagingStreamPath(deploymentId: string, conversationId: string): string {
    return this.messagingProxyPath(
      deploymentId,
      `conversations/${encodeURIComponent(conversationId)}/stream`,
    );
  }

  async getMessagingHistory(
    deploymentId: string,
    conversationId: string,
  ): Promise<MessagingHistoryResponse> {
    return this.request<MessagingHistoryResponse>(
      this.messagingProxyPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}/history`,
      ),
    );
  }

  /** Platform deployment chat API — shared by all Astro clients; see docs/04-guides/deployment-chat.md */
  private deploymentChatPath(deploymentId: string, subpath?: string) {
    const base = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/chat`;
    return subpath ? `${base}/${subpath.replace(/^\//, "")}` : base;
  }

  async listDeploymentChatConversations(
    deploymentId: string,
  ): Promise<ListDeploymentChatConversationsResponse> {
    return this.request<ListDeploymentChatConversationsResponse>(
      this.deploymentChatPath(deploymentId, "conversations"),
    );
  }

  async getDeploymentChatConversation(
    deploymentId: string,
    conversationId: string,
    query?: DeploymentChatConversationQuery,
  ): Promise<GetDeploymentChatConversationResponse> {
    const params = new URLSearchParams();
    if (query?.limit != null) params.set("limit", String(query.limit));
    if (query?.before_seq != null) {
      params.set("before_seq", String(query.before_seq));
    }
    const qs = params.toString();
    return this.request<GetDeploymentChatConversationResponse>(
      this.deploymentChatPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}${qs ? `?${qs}` : ""}`,
      ),
    );
  }

  async upsertDeploymentChatConversation(
    deploymentId: string,
    conversationId: string,
    body: { title: string },
  ): Promise<{ conversation_id: string; title: string }> {
    return this.request(
      this.deploymentChatPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}`,
      ),
      { method: "PUT", body: JSON.stringify(body) },
    );
  }

  async deleteDeploymentChatConversation(
    deploymentId: string,
    conversationId: string,
  ): Promise<void> {
    await this.request(
      this.deploymentChatPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}`,
      ),
      { method: "DELETE" },
    );
  }

  async updateDeploymentDisplayName(id: string, displayName: string): Promise<{ display_name: string }> {
    return this.request<{ display_name: string }>(
      `/api/v1/deployments/${encodeURIComponent(id)}`,
      { method: "PATCH", body: JSON.stringify({ display_name: displayName }) },
    );
  }

  // Get active deployment spec for an agent
  async getActiveDeploymentSpec(account: string, name: string): Promise<ActiveDeploymentSpecResponse> {
    return this.request<ActiveDeploymentSpecResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment`
    );
  }

  // Get deployment history for an agent
  async getDeploymentHistory(account: string, name: string, deploymentId?: string): Promise<DeploymentHistoryResponse> {
    const params = new URLSearchParams();
    if (deploymentId) params.set("deployment_id", deploymentId);
    const qs = params.toString();
    return this.request<DeploymentHistoryResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment/history${qs ? `?${qs}` : ""}`
    );
  }

  // Fetch logs for a workload's containers
  async getDeploymentLogs(
    deploymentId: string,
    workloadName: string,
    container: string,
    since?: string,
    timezone?: string,
    options?: { level?: string; direction?: string; tailLines?: number; until?: string },
  ): Promise<LogEntry[]> {
    const params = new URLSearchParams({ workload: workloadName, container });
    if (since) params.set('since', since);
    if (options?.until) params.set('until', options.until);
    if (timezone && timezone !== 'UTC') params.set('timezone', timezone);
    if (options?.level) params.set('level', options.level);
    if (options?.direction) params.set('direction', options.direction);
    if (options?.tailLines !== undefined) params.set('tailLines', String(options.tailLines));
    return this.request<LogEntry[]>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/logs?${params}`
    );
  }

  getDeploymentLogsStreamUrl(deploymentId: string, workloadName: string, container: string, timezone?: string): string {
    const params = new URLSearchParams({ workload: workloadName, container });
    if (timezone && timezone !== 'UTC') params.set('timezone', timezone);
    return `${this.baseUrl}/api/v1/deployments/${encodeURIComponent(deploymentId)}/logs/stream?${params}`;
  }

  async getDeploymentEvents(deploymentId: string): Promise<DeploymentEventsResponse> {
    return this.request<DeploymentEventsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/events`
    );
  }

  async getEvalDataset(deploymentId: string): Promise<EvalDatasetResponse> {
    return this.request<EvalDatasetResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset`
    );
  }

  async getPodMetrics(
    deploymentId: string,
    pod: string,
    range: PodMetricsRange,
  ): Promise<PodMetricsResponse> {
    return this.request<PodMetricsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/pods/${encodeURIComponent(pod)}/metrics?range=${range}`
    );
  }

  // Fetch ConfigMap data for a deployment
  async getConfigMapData(
    deploymentId: string,
    cmname: string,
  ): Promise<ConfigMapResponse> {
    return this.request<ConfigMapResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/configmap/${encodeURIComponent(cmname)}`
    );
  }

  // Fetch Secret key names (no values) for a deployment
  async getSecretKeys(
    deploymentId: string,
    secretName: string,
  ): Promise<SecretKeysResponse> {
    return this.request<SecretKeysResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/secret/${encodeURIComponent(secretName)}/keys`
    );
  }

  // --------------------------------------------------------------------------
  // Observability (deployment- and account-scoped, backed by Langfuse)
  // --------------------------------------------------------------------------

  async getObservabilityMetrics(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityMetricsResponse> {
    return this.request<ObservabilityMetricsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/metrics${buildQS(params)}`
    );
  }

  async getDeploymentObservabilitySummaries(
    account: string,
  ): Promise<DeploymentSummariesResponse> {
    return this.request<DeploymentSummariesResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/observability/deployment-summaries`
    );
  }

  async getObservabilitySummary(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilitySummaryResponse> {
    return this.request<ObservabilitySummaryResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/summary${buildQS(params)}`
    );
  }

  async getAccountObservabilitySummary(
    account: string,
    params?: Record<string, string>,
  ): Promise<AccountObservabilitySummaryResponse> {
    return this.request<AccountObservabilitySummaryResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/observability/summary${buildQS(params)}`
    );
  }

  async getAccountInsights(account: string, params?: InsightsQueryParams): Promise<InsightsResponse> {
    return this.request<InsightsResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/insights${buildQS(params)}`
    );
  }

  async getObservabilityTraces(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityTracesResponse> {
    return this.request<ObservabilityTracesResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/traces${buildQS(params)}`
    );
  }

  async getObservabilityTraceDetail(
    deploymentId: string,
    traceId: string,
  ): Promise<TraceDetailResponse> {
    return this.request<TraceDetailResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/traces/${encodeURIComponent(traceId)}`
    );
  }

  async getObservabilityObservationDetail(
    deploymentId: string,
    observationId: string,
  ): Promise<TraceObservation> {
    return this.request<TraceObservation>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/observations/${encodeURIComponent(observationId)}`
    );
  }

  async triggerIngestion(data: {
    deploymentId: string;
    ingestion: string;
  }): Promise<TriggerIngestionResponse> {
    return this.request<TriggerIngestionResponse>(
      `/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/ingestion/${encodeURIComponent(data.ingestion)}/trigger`,
      { method: 'POST' }
    );
  }

  // --------------------------------------------------------------------------
  // Network (Beyla eBPF)
  // --------------------------------------------------------------------------

  async getNetworkSummary(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<NetworkSummaryResponse> {
    return this.request<NetworkSummaryResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/network/summary${buildQS(params)}`
    );
  }

  async getNetworkFlows(
    deploymentId: string,
    params: Record<string, string>,
  ): Promise<NetworkFlowsResponse> {
    return this.request<NetworkFlowsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/network/flows${buildQS(params)}`
    );
  }

  async getNetworkTimeseries(
    deploymentId: string,
    params: Record<string, string>,
  ): Promise<NetworkTimeseriesResponse> {
    return this.request<NetworkTimeseriesResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/network/timeseries${buildQS(params)}`
    );
  }

  // --------------------------------------------------------------------------
  // Avatars (accounts, blueprints, deployments)
  // --------------------------------------------------------------------------

  async uploadAvatar(account: string, file: Blob): Promise<AvatarResponse> {
    const formData = new FormData();
    formData.append('avatar', file, 'avatar.jpg');
    return this.uploadFormData<AvatarResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/avatar`,
      formData,
    );
  }

  async setAvatarPreset(account: string, index: number): Promise<AvatarResponse> {
    return this.request<AvatarResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/avatar/preset/${index}`,
      { method: 'PUT' },
    );
  }

  async resetAvatar(account: string): Promise<AvatarResponse> {
    return this.request<AvatarResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/avatar`,
      { method: 'DELETE' },
    );
  }

  async uploadBlueprintAvatar(account: string, name: string, file: Blob): Promise<AvatarResponse> {
    const formData = new FormData();
    formData.append('avatar', file, 'avatar.jpg');
    return this.uploadFormData<AvatarResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/avatar`,
      formData,
    );
  }

  async deleteBlueprintAvatar(account: string, name: string): Promise<AvatarResponse> {
    return this.request<AvatarResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/avatar`,
      { method: 'DELETE' },
    );
  }

  async uploadDeploymentAvatar(id: string, file: Blob): Promise<AvatarResponse> {
    const formData = new FormData();
    formData.append('avatar', file, 'avatar.jpg');
    return this.uploadFormData<AvatarResponse>(
      `/api/v1/deployments/${encodeURIComponent(id)}/avatar`,
      formData,
    );
  }

  async deleteDeploymentAvatar(id: string): Promise<AvatarResponse> {
    return this.request<AvatarResponse>(
      `/api/v1/deployments/${encodeURIComponent(id)}/avatar`,
      { method: 'DELETE' },
    );
  }

  // --------------------------------------------------------------------------
  // Knowledge
  // --------------------------------------------------------------------------

  async listKnowledgeStores(account: string): Promise<KnowledgeStoreListResponse> {
    return this.request<KnowledgeStoreListResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge`
    );
  }

  async getKnowledgeStore(account: string, name: string): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}`
    );
  }

  async createKnowledgeStore(account: string, data: CreateKnowledgeStoreInput): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge`,
      { method: 'POST', body: JSON.stringify(data) }
    );
  }

  async connectKnowledgeStore(account: string, data: ConnectKnowledgeStoreInput): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/connect`,
      { method: 'POST', body: JSON.stringify(data) }
    );
  }

  async deleteKnowledgeStore(account: string, name: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}`,
      { method: 'DELETE' }
    );
  }

  async getKnowledgeCredentials(account: string, name: string): Promise<KnowledgeCredentials> {
    return this.request<KnowledgeCredentials>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/credentials`
    );
  }

  async getKnowledgeLogs(
    account: string,
    name: string,
    since?: string,
  ): Promise<LogEntry[]> {
    const params = new URLSearchParams();
    if (since) params.set('since', since);
    const qs = params.toString();
    return this.request<LogEntry[]>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/logs${qs ? `?${qs}` : ''}`
    );
  }

  getKnowledgeLogsStreamUrl(account: string, name: string): string {
    return `${this.baseUrl}/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/logs/stream`;
  }

  async getKnowledgeMetrics(account: string, name: string): Promise<KnowledgeMetrics> {
    return this.request<KnowledgeMetrics>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/metrics`
    );
  }

  getKnowledgeEventsStreamUrl(account: string, name: string): string {
    return `${this.baseUrl}/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/events`;
  }

  // --------------------------------------------------------------------------
  // GitHub (per-blueprint connection + per-account connection)
  // --------------------------------------------------------------------------

  async gitHubRebuild(account: string, name: string): Promise<{ build_id: string; commit_sha: string }> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/github/rebuild`,
      { method: 'POST' }
    );
  }

  async getGitHubBuildLogs(account: string, name: string, buildId: string): Promise<{ components?: Array<{ name: string; status: string; logs: string }>; job: string; pod: string; logs: string }> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/github/builds/${encodeURIComponent(buildId)}/logs`
    );
  }

  async getGitHubStatus(account: string, name: string): Promise<GitHubStatusResponse> {
    return this.request<GitHubStatusResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/github`
    );
  }

  async gitHubLink(account: string, name: string, body: GitHubLinkInput): Promise<void> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/github/link`,
      { method: 'POST', body: JSON.stringify(body) }
    );
  }

  async gitHubDisconnect(account: string, name: string): Promise<void> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/github`,
      { method: 'DELETE' }
    );
  }

  // Account-level GitHub connection (blueprint-agnostic)
  async gitHubAccountStatus(account: string): Promise<{ connected: boolean; github_login?: string }> {
    return this.request<{ connected: boolean; github_login?: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github`
    );
  }

  async gitHubAccountDisconnect(account: string): Promise<void> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/github`,
      { method: 'DELETE' }
    );
  }

  async gitHubConnectAccount(account: string, redirectTo: string, force?: boolean): Promise<GitHubConnectResponse> {
    return this.request<GitHubConnectResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/connect`,
      { method: 'POST', body: JSON.stringify({ redirect_to: redirectTo, force: force ?? false }) }
    );
  }

  async gitHubListAccountRepos(account: string, q: string, login?: string): Promise<{ repos: GitHubRepo[]; has_more: boolean }> {
    const params = new URLSearchParams({ q });
    if (login) params.set("login", login);
    return this.request<{ repos: GitHubRepo[]; has_more: boolean }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/repos?${params}`
    );
  }

  async gitHubAccountScan(account: string, repo: string, branch: string, agentName?: string): Promise<{ found: boolean }> {
    const params = new URLSearchParams({ repo, branch });
    if (agentName) params.set("agent_name", agentName);
    return this.request<{ found: boolean }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/scan?${params}`
    );
  }

  async gitHubListAccountConnections(account: string): Promise<{ connections: Array<{ agent_name: string; repo_full_name: string; created_at: string }> }> {
    return this.request<{ connections: Array<{ agent_name: string; repo_full_name: string; created_at: string }> }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/connections`
    );
  }

  async gitHubListAccountOrgs(account: string): Promise<{ orgs: Array<{ login: string; avatar_url: string }> }> {
    return this.request<{ orgs: Array<{ login: string; avatar_url: string }> }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/orgs`
    );
  }

  // --------------------------------------------------------------------------
  // Slack
  // --------------------------------------------------------------------------

  async slackAccountStatus(account: string): Promise<SlackStatusResponse> {
    return this.request<SlackStatusResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/slack`
    );
  }

  /** Disconnect: omit teamID to revoke every workspace mapping; pass a
   *  team_id to revoke just that one (multi-workspace per-row disconnect). */
  async slackAccountDisconnect(account: string, teamID?: string): Promise<void> {
    const url = `/api/v1/accounts/${encodeURIComponent(account)}/slack`;
    const qs = teamID ? `?team_id=${encodeURIComponent(teamID)}` : '';
    return this.request(url + qs, { method: 'DELETE' });
  }

  async slackConnectAccount(account: string, redirectTo: string): Promise<SlackConnectResponse> {
    return this.request<SlackConnectResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/slack/connect`,
      { method: 'POST', body: JSON.stringify({ redirect_to: redirectTo }) }
    );
  }

  // --------------------------------------------------------------------------
  // Variables (account-level vault)
  // --------------------------------------------------------------------------

  async listAccountVariables(account: string): Promise<AccountVariablesListResponse> {
    return this.request<AccountVariablesListResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/variables`
    );
  }

  async createAccountVariables(
    account: string,
    variables: CreateAccountVariableInput[],
  ): Promise<CreateAccountVariablesResponse> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/variables`,
      { method: 'POST', body: JSON.stringify({ variables }) }
    );
  }

  async updateAccountVariable(
    account: string,
    varName: string,
    data: UpdateAccountVariableInput,
  ): Promise<{ name: string; message: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/variables/${encodeURIComponent(varName)}`,
      { method: 'PUT', body: JSON.stringify(data) }
    );
  }

  async deleteAccountVariable(
    account: string,
    varName: string,
  ): Promise<{ message: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/variables/${encodeURIComponent(varName)}`,
      { method: 'DELETE' }
    );
  }

  // --------------------------------------------------------------------------
  // Audit log
  // --------------------------------------------------------------------------

  async listAuditLog(
    account: string,
    params?: AuditLogQueryParams,
  ): Promise<AuditLogListResponse> {
    const qs = params
      ? `?${new URLSearchParams(
          Object.entries(params)
            .filter(([, v]) => v != null && v !== '')
            .map(([k, v]) => [k, String(v)])
        )}`
      : '';
    return this.request<AuditLogListResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/audit-log${qs}`
    );
  }

  async listAuditLogFilters(
    account: string,
  ): Promise<AuditLogFilterOptions> {
    return this.request<AuditLogFilterOptions>(
      `/api/v1/accounts/${encodeURIComponent(account)}/audit-log/filters`
    );
  }

  // --------------------------------------------------------------------------
  // Usage / quota / feedback
  // --------------------------------------------------------------------------

  async getAccountUsage(
    account: string,
    params?: { from?: string; to?: string },
  ): Promise<AccountUsageResponse> {
    const qs = params ? `?${new URLSearchParams(Object.entries(params).filter(([, v]) => v) as [string, string][])}` : '';
    return this.request<AccountUsageResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/usage${qs}`
    );
  }

  async listQuotaIncreaseRequests(
    account: string,
  ): Promise<QuotaIncreaseListResponse> {
    return this.request<QuotaIncreaseListResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/quota-increase`
    );
  }

  async requestQuotaIncrease(
    account: string,
    body: QuotaIncreaseInput,
  ): Promise<{ id: string; status: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/quota-increase`,
      { method: 'POST', body: JSON.stringify(body) }
    );
  }

  async submitFeedback(body: FeedbackInput): Promise<{ id: string }> {
    return this.request('/api/v1/feedback', {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }
}

// ============================================================================
// Singleton export
// ============================================================================

// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = typeof import.meta !== 'undefined' && import.meta.env
  ? (import.meta.env.VITE_API_URL || '')
  : '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
