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
import {
  parseFilesApiError,
  type FilesApiOperation,
} from "./files-api-errors";
import type {
  Interaction,
  InteractionResponseAck,
  InteractionResponseBody,
} from "./chat/interaction";
import type { UserResourceScopeSelection } from "./user-resource-scope";
import type { UserResourceListParams } from "./user-resource-list-params";

type UserResourceListOptions = UserResourceListParams & { cursor?: string };

function buildQS(params?: Record<string, string | undefined>): string {
  if (!params) return '';
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) qs.set(key, value);
  }
  const encoded = qs.toString();
  return encoded ? `?${encoded}` : '';
}

function buildUserResourceQuery(
  scope: UserResourceScopeSelection,
  options?: UserResourceListOptions,
): URLSearchParams {
  const params = new URLSearchParams();
  if (scope.all) {
    params.set("scope", "all");
  } else {
    scope.accounts.forEach((account) => params.append("account", account));
  }
  if (options?.q?.trim()) params.set("q", options.q.trim());
  if (options?.limit != null) params.set("limit", String(options.limit));
  if (options?.cursor) params.set("cursor", options.cursor);
  return params;
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

export function getApiErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object') {
    const apiError = error as ApiError & { message?: unknown };
    const message =
      apiError.error_description ||
      apiError.details ||
      apiError.error ||
      (typeof apiError.message === 'string' ? apiError.message : '');

    if (message.trim()) return message;
  }

  return fallback;
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
  local_timezone?: string;
  pronouns?: string;
  website?: string;
  social_links?: string[];
  blueprint_order?: string[];
  avatar_url?: string;
  avatar_colors?: AvatarColors;
}

export interface Account {
  id: string;
  name: string;
  type: string;
  display_name?: string;
  avatar_url?: string;
  role?: string;
  organization_id?: string; // WorkOS org ID, present on organization accounts
  /** Placement cluster (e.g. "eu"); empty = primary US cluster */
  cluster_id?: string;
  agents?: BlueprintSummary[];
  account_number?: number;
  bio?: string;
  location?: string;
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
  avatar_url?: string;
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

export interface UserResourcePage {
  limit: number;
  next_cursor?: string;
}

export interface UserResourceScope {
  accounts: string[];
  all: boolean;
}

export interface UserResourceResponse {
  page: UserResourcePage;
  scope: UserResourceScope;
  rejected_accounts?: string[];
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

export interface UserBlueprintsResponse extends UserResourceResponse {
  blueprints: Blueprint[];
}

export type UserBlueprintListParams = Omit<BlueprintListParams, "offset"> & {
  cursor?: string;
};

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

/** Deploy-time gateway model choice, surfaced at the template response root.
 *  The chosen identifier is injected server-side into the agent env var. */
export interface ModelSelection {
  name: string;
  env_var: string;
  options: string[];
  default: string;
  selected: string;
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
  models?: Record<string, string>; // model selection name → chosen identifier
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
  models?: ModelSelection[];
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
  /** Front-door ingress upstream response timeout. Go duration ("15s", "2m");
   *  empty falls back to the server default (15s). */
  response_timeout?: string;
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
  /**
   * Public routes the web chat ingress to the open (no-OIDC) cohort, so it's
   * reachable without sign-in. Requires an "anyone" grant (the server rejects
   * org/user grants here, since there's no OIDC identity to authorize against).
   */
  public?: boolean;
}

export interface SlackAuthConfig {
  grants?: AuthGrant[];
}

export interface CustomAuthConfig {
  /** Route the agent's custom interface to the open (no-OIDC) cohort — reachable without sign-in. */
  public?: boolean;
  /** Who may use the custom interface. Recorded, but not enforced by the platform today. */
  grants?: AuthGrant[];
}

export interface TemplateInterfaces {
  adapters: string[];
  auth?: {
    web?: WebAuthConfig;
    slack?: SlackAuthConfig;
    custom?: CustomAuthConfig;
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

// ContainerStatus is the observed runtime state for a single container — no
// env. Env is apply-time intent and lives on WorkloadSpec.env, keyed by role.
export interface ContainerStatus {
  name: string;
  state: string;
  ready: boolean;
  restart_count: number;
  message?: string; // plain-language explanation when unhealthy
}

// WorkloadDetail is the JOINED view of a workload — what the page builds by
// zipping a WorkloadSpec from the record onto its WorkloadRuntime entry from
// the runtime response, keyed by name. Leaf components (PodTile,
// PodDetailPanel) consume this shape; the join lives at the page boundary.
//
// Fields are typed as optional where they only come from one side of the
// join, so a runtime-less render (loading, or before the controller's first
// observation) still produces a valid object containing just the spec.
export interface WorkloadDetail {
  // Intent (WorkloadSpec):
  name: string;
  kind: string;
  component: string;
  /** Platform provider (e.g. postgres, qdrant); empty for custom/agent workloads. */
  provider?: string;
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
  /** Persistent-volume usage in bytes; absent when there's no volume or metrics are unavailable. */
  storage_used_bytes?: number;
  storage_capacity_bytes?: number;
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
  /** Platform provider (e.g. postgres, qdrant); empty for custom/agent workloads. */
  provider?: string;
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

// WorkloadRuntime is the observed runtime state for a single workload
// (projected from the controller-maintained snapshot), keyed by Name to stitch
// onto the corresponding WorkloadSpec on the record.
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
  /** Persistent-volume usage in bytes; absent when there's no volume or metrics are unavailable. */
  storage_used_bytes?: number;
  storage_capacity_bytes?: number;
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
  deployed_by?: string;
  components: string[];
  external_urls?: ServiceEndpointInfo[];
  // A messaging sidecar is part of the deployment spec. Distinct from
  // DeploymentRuntime.messaging_reachable, which is the observed reachability
  // (messaging Service present AND sidecar container ready) from the runtime
  // snapshot.
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
  | "suspended"
  | "undeploying"
  | "failed"
  | "provisioning"
  | "ready";

// A workload keeping a deployment out of "active": still settling (waiting_on —
// phase "missing" or an observed phase that isn't ready) or terminally broken
// (failed_on — phase "failed"). message is a plain-language explanation; raw
// K8s reason codes are never sent.
export interface WorkloadIssue {
  workload: string;
  component?: string;
  phase: string;
  message?: string;
  title?: string;
  guidance?: string;
}

// DeploymentStatus is the body of GET /deployments/:id/status (no envelope).
// Mirrors handlers.DeploymentStatus on the server. Live replica/ready counts
// and per-workload state live on DeploymentRuntime; this stays narrow.
export interface DeploymentStatus {
  value: DeploymentStatusValue;
  reason: DeploymentStatusReason;
  details: string;
  error_message?: string;
  waiting_on?: WorkloadIssue[]; // set only while value === "deploying"
  failed_on?: WorkloadIssue[]; // set only while value === "error"
}

// DeploymentRuntime is the observed-runtime view (GET /deployments/:id/runtime).
// Mirrors handlers.DeploymentRuntime on the server, which serves it from the
// controller-maintained snapshot (deployment_runtime_status) rather than a
// per-request K8s read — so it's cluster-independent and never 503s; a
// not-yet-observed deployment reads empty. Frontend stitches `workloads` onto
// AgentDeployment.workloads by `name`.
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
  avatar_url?: string;
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
  account_id?: string;
  account_name?: string;
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

// Agent self-reported config from the messaging web sidecar (GET
// /api/agent/config), proxied via astro-server. Same shape the playground
// reads; powers the chat inspector's Settings/config tab.
export interface DeploymentAgentConfigTool {
  name: string;
  title?: string;
  description?: string;
  type?: string;
}

export interface DeploymentAgentConfigCapabilities {
  // Uploads are usable: the sidecar has file storage AND the agent declared it
  // consumes attachments. The composer hides its upload affordance when false;
  // absent (older sidecar) is treated as false so the button stays hidden.
  files?: boolean;
}

export interface DeploymentAgentConfig {
  systemPrompt: string;
  tools: DeploymentAgentConfigTool[];
  capabilities?: DeploymentAgentConfigCapabilities;
}

export interface DeploymentChatConversationSummary {
  conversation_id: string;
  title: string;
  updated_at: string;
  assistant_streaming?: boolean;
}

/** A file attached to a chat message. `key` is the files-API key; the rest is
 *  display metadata. Same shape for user-uploaded and agent-produced files. */
export interface ChatAttachment {
  key: string;
  name: string;
  content_type: string;
  size: number;
}

export interface DeploymentChatMessageRecord {
  id: string;
  // "note" is a server-injected ghost recording a resolved interaction.
  role: "user" | "assistant" | "note";
  content: string;
  attachments?: ChatAttachment[];
}

export interface ListDeploymentChatConversationsResponse {
  conversations: DeploymentChatConversationSummary[];
}

export interface GetDeploymentChatConversationResponse {
  conversation_id: string;
  title: string;
  updated_at: string;
  messages: DeploymentChatMessageRecord[];
  /** True while the latest persisted message is the user's (assistant reply still in flight). */
  assistant_streaming?: boolean;
  has_more?: boolean;
  oldest_seq?: number;
  /** Still-open blocking interactions (the FIFO queue), for reload re-render. */
  pending_interactions?: Interaction[];
}

export type DeploymentChatConversationQuery = {
  /** Return the last N messages (live refresh). Omit for full thread. */
  limit?: number;
  before_seq?: number;
};

/** Agent file metadata. `key` is the opaque server id used across the API. */
export interface DeploymentFileMeta {
  key: string;
  name: string;
  size: number;
  content_type: string;
  updated_at: string;
  uploaded_by?: string;
}

export interface ListDeploymentFilesResponse {
  files: DeploymentFileMeta[];
}

/** Capacity of the volume backing the deployment's file store. `available` is
 *  false when the store can't report usage (S3-backed, or no statfs) — the UI
 *  hides the capacity warning rather than showing a misleading 0%. */
export interface DeploymentStorageUsage {
  available: boolean;
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  percent_used: number;
}

/** Where the client sends the bytes after reserving a file. `url` may be a
 *  relative content path (server-received upload) or an absolute presigned URL
 *  (direct-to-store); the client resolves and uses it as-is either way. */
export interface DeploymentFileUploadDescriptor {
  url: string;
  method: string;
  headers?: Record<string, string>;
}

export interface CreateDeploymentFileResponse {
  key: string;
  file: DeploymentFileMeta;
  upload: DeploymentFileUploadDescriptor;
}

export interface DeploymentsListResponse {
  deployments: AgentDeploymentSummary[];
  count: number;
}

export interface UserDeploymentsResponse extends UserResourceResponse {
  deployments: AgentDeploymentSummary[];
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
  deployed_by?: string;
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
  // Server-populated, action-oriented copy for events that mean "the
  // deployment is stuck and the user must act" (e.g. FailedScheduling).
  // Absent for events the server has no copy for; the UI then renders the
  // raw reason/message.
  title?: string;
  guidance?: string;
  // Server-assigned category for humanized events: "info" (normal progress),
  // "transient" (self-recovering), or "stuck" (needs user action). The stuck
  // banner triggers on "stuck". Absent for events with no copy.
  severity?: "info" | "transient" | "stuck";
}

export interface DeploymentEventsResponse {
  events: K8sEvent[];
}

// One configured observation alert and its current state for a deployment.
// state: "ok" (not breaching), "pending" (breaching but within the sustained
// window, no alert sent), "firing" (alert emitted). activeSince is set while
// pending/firing.
export interface DeploymentAlert {
  name: string;
  title: string;
  description: string;
  severity: "warning" | "critical";
  state: "ok" | "pending" | "firing";
  activeSince: string | null;
}

export interface DeploymentAlertsResponse {
  alerts: DeploymentAlert[];
}

// --- Dataset ---

export interface EvalDatasetCriteriaCount {
  dimension_key: string;
  good_count: number;
  bad_count: number;
}

export interface EvalDatasetResponse {
  dataset_name: string;
  item_count: number;
  good_count: number;
  bad_count: number;
  /** Server-computed letter grade: A, B, C, D, F, or "—" when empty. */
  grade: string;
  /** Letter of the next grade level above the current one. Empty when already at A. */
  next_grade: string;
  /** Progress within the current grade band toward `next_grade`, 0..1. */
  next_grade_progress: number;
  /** Additional judged cases needed for `next_grade`; null when there is no next grade. */
  cases_to_next_grade: number | null;
  criteria_counts: EvalDatasetCriteriaCount[];
}

/** Per-item metadata persisted on Langfuse dataset items. `verdict` is a
 *  numeric score: 1 = good, -1 = bad. Skip/unknown verdicts never produce an
 *  item, so good/bad are the only values that surface here. */
export interface EvalDatasetItemMetadata {
  verdict?: number;
  confidence?: number;
  judged_by_user_id?: string;
  judged_at?: string;
  judgment_criteria?: JudgmentCriterion[];
}

export interface EvalDatasetItem {
  id: string;
  input: unknown;
  expected_output: unknown;
  metadata: EvalDatasetItemMetadata | null;
  source_trace_id: string;
  created_at: string;
}

export interface EvalDatasetItemsResponse {
  items: EvalDatasetItem[];
  page: number;
  limit: number;
  total_items: number;
  total_pages: number;
  next_cursor?: string;
}

export type EvalDatasetItemsVerdict = "good" | "bad";

export interface EvalDatasetItemsParams {
  page?: number;
  cursor?: string;
  limit: number;
  verdict?: EvalDatasetItemsVerdict;
}

export type ReviewQueuePredictionStatus =
  | "not_requested"
  | "queued"
  | "in_progress"
  | "completed"
  | "failed";
export type ReviewQueuePredictionFilter =
  | "good"
  | "bad"
  | "unknown"
  | "none";

export interface ReviewQueuePredictionCriterion {
  dimension_key: string;
  dimension_value: number;
}

export interface ReviewQueuePrediction {
  verdict_score: number;
  confidence: number;
  explanation: string;
  judge_version: string;
  criteria: ReviewQueuePredictionCriterion[];
}

export interface ReviewQueueItem {
  trace_id: string;
  timestamp: string;
  user_id?: string;
  user_details?: UserDetails;
  input: unknown;
  output: unknown;
  prediction_status: ReviewQueuePredictionStatus;
  prediction_error: string | null;
  prediction: ReviewQueuePrediction | null;
}

export interface PredictionStatusCounts {
  queued: number;
  in_progress: number;
  completed: number;
  failed: number;
}

export interface ReviewQueueResponse {
  items: ReviewQueueItem[];
  /** Opaque continuation cursor; omitted when the trace snapshot is exhausted. */
  next_cursor?: string;
}

export interface ReviewQueueParams {
  /** Page size requested from the review queue endpoint. */
  limit?: number;
  /** Opaque cursor returned by the previous page. */
  cursor?: string;
  prediction?: ReviewQueuePredictionFilter;
}

export interface DatasetPredictionsResponse {
  enqueued_trace_ids: string[];
  failed_trace_ids: string[];
}

export type DatasetJudgmentVerdict = "good" | "bad" | "unknown";

export interface DatasetJudgmentRequest {
  trace_id: string;
  verdict: DatasetJudgmentVerdict;
}

export interface DatasetJudgmentResponse {
  eval_dataset_id: string;
  trace_id: string;
  verdict: DatasetJudgmentVerdict;
}

/** A selected judgment criterion. `value` is the dimension score captured at
 *  judgment time: human review sends 1 (good) or -1 (bad). */
export interface JudgmentCriterion {
  dimension_key: string;
  value: number;
}

export interface DatasetJudgmentCriteriaResponse {
  eval_dataset_id: string;
  trace_id: string;
  verdict: DatasetJudgmentVerdict;
  criteria: JudgmentCriterion[];
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
  /** Total spend in USD over the trailing 30-day window (sum of cost_series). */
  cost_usd?: number;
  /** 30-day daily request counts, oldest → newest. Server pads days with no
   *  activity to zero so consumers can rely on a fixed-length array. */
  request_series?: number[];
  /** 30-day daily token totals (input + output), oldest → newest. */
  token_series?: number[];
  /** 30-day daily spend totals, oldest → newest. */
  cost_series?: number[];
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
  cost_by_model: Array<{
    model: string;
    cost_usd: number;
    cost_pct: number;
    /** Token volume for this model and its share of total tokens. */
    total_tokens: number;
    token_pct: number;
    /** Request count, latency percentiles, and latest activity for this model over the period. */
    requests: number;
    p50_latency_ms: number;
    p95_latency_ms: number;
    last_seen?: string;
  }>;
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
  icon?: string; // integration-icon key (e.g. "anthropic") resolved to a themed brand logo
  // Set when the underlying resource no longer exists — a deleted agent whose
  // past spend is still reported. The server also clears `href`, so the row is
  // not a link.
  is_deleted?: boolean;
}

export interface InsightsAgentChip {
  key: string;
  label: string;
  href: string;
  avatar_account: string;
  avatar_name: string;
  is_deleted?: boolean;
  icon?: string; // integration-icon key (e.g. "anthropic") → themed logo, for dev-tool chips
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
    last_seen?: string;
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
  hide_sources?: string; // comma-separated source keys (or "agents") to exclude from the fold-in
  // Scopes the tables to the selected range. Only the v2 path honours it — v1's
  // people rows come from a cached aggregate with no daily breakdown, so its
  // tables are account-wide by necessity.
  days?: string;
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
  /** Last day ("YYYY-MM-DD", UTC) the data is complete through, and the day
   *  every window in `ranges` ends on. Absent when the server has no watermark
   *  to report, in which case the windows end today. */
  as_of?: string;
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
  // Dev-tool sources present in the account, for the Sources filter. Their usage
  // is already folded into ranges/tables unless excluded via hide_sources.
  devtool_sources?: InsightsDevtoolSource[];
}

export interface InsightsDevtoolSource {
  key: string;
  label: string;
  icon?: string;
}

export interface TraceEntry {
  trace_id: string;
  name: string;
  status: string;
  latency_ms: number;
  total_tokens?: number;
  total_cost?: number;
  timestamp: string;
  user_id?: string;
  user_details?: UserDetails;
}

export interface ObservabilityTracesResponse {
  traces: TraceEntry[];
  total: number;
  limit: number;
  offset: number;
  truncated?: boolean;
  scanned_count?: number;
}

export interface TraceUserFacet {
  user_id?: string;
  user_details?: UserDetails;
  count: number;
}

export interface TraceUserFacetsResponse {
  users: TraceUserFacet[];
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
  input?: unknown;
  output?: unknown;
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
  /** eTLD+1, server-computed. Only set for outbound address peers. */
  registrable_domain?: string;
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

export type KnowledgeProvider = 'postgres' | 'qdrant' | 'redis' | 'neo4j' | 'pinecone' | 'mysql' | 'supabase';
// Every store created now is 'external'. Rows predating the removal of
// platform-provisioned stores still report 'managed'.
export type KnowledgeMode = 'managed' | 'external';
export type KnowledgeStatus = 'connecting' | 'pending-acceptance' | 'ready' | 'error';

export interface KnowledgeEndpoint {
  cloud_provider: string;
  endpoint_service: string;
  region: string;
  endpoint_id?: string;
  endpoint_dns?: string;
  status: string;
  error?: string | null;
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
  endpoint?: KnowledgeEndpoint;
  error?: string | null;
  annotations?: Record<string, string>;
  created_at: string;
  updated_at: string;
  bound_agents?: BoundAgent[];
  account_id?: string;
  account?: string;
}

export type KnowledgeStoreListResponse = KnowledgeStore[];

export interface UserKnowledgeStoresResponse extends UserResourceResponse {
  stores: KnowledgeStore[];
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
  // When set, marks the store as a Supabase import; the server composes the
  // store's origin annotations from it.
  supabase_project?: { id: string; name: string; region: string; organization_id: string };
}

export type KnowledgeCredentials = Record<string, string>;

// Fields are optional: only the provided ones are updated. For Supabase-imported
// stores, host/port/username are server-managed and rejected if supplied.
export interface UpdateKnowledgeCredentialsInput {
  host?: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
  api_key?: string;
  skip_health_check?: boolean;
}

// ============================================================================
// Supabase OAuth (project auto-import for PostgreSQL knowledge stores)
// ============================================================================

export interface SupabaseProject {
  id: string;
  name: string;
  region: string;
  organization_id: string;
}

// Proxied verbatim from Supabase's services-health endpoint; fields are optional
// because we render whatever the provider reports without imposing a schema.
export interface SupabaseServiceHealth {
  name?: string;
  status?: string;
  healthy?: boolean;
}

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
 *  workspace's avatar URL (empty when the workspace uses Slack's default
 *  icon — the UI falls back to a generic Lucide building icon in that case). */
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

export interface OtelIngestKey {
  id: string;
  name: string;
  token_prefix: string;
  created_at: string;
  last_used_at?: string;
  excluded_emails: string[];
  // Which external tool this source is (e.g. "claude-code"). Absent today —
  // every source is Claude Code — but drives the per-kind icon in the table.
  source_type?: string;
}

export interface OtelIngestKeysListResponse {
  tokens: OtelIngestKey[];
  endpoint?: string;
}

export interface CreateOtelIngestKeyResponse extends OtelIngestKey {
  token: string; // plaintext, returned once
  endpoint?: string;
}

export interface NotificationPreference {
  type: string;
  name: string;
  description?: string;
  category: string;
  critical: boolean; // locked on; the user cannot disable it
  email: boolean;
  in_app: boolean;
}

export interface NotificationPreferencesResponse {
  delivery_enabled: boolean; // whether Novu is wired
  preferences: NotificationPreference[];
}

export interface UpdateNotificationPreferenceInput {
  type: string;
  email: boolean;
  in_app: boolean;
}

export interface NotificationInboxConfig {
  enabled: boolean; // false when the Inbox isn't configured
  application_identifier?: string;
  subscriber_id?: string;
  subscriber_hash?: string;
  backend_url?: string;
  socket_url?: string;
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

// Billing data is returned by the provider (Metronome) and rendered as-is, so
// the row shapes are intentionally loose. `available` is false when the hosted
// backend or the account's customer isn't configured.
export interface BillingDataResponse<T> {
  available: boolean;
  data?: T;
}

export interface BillingCreditType {
  id?: string;
  name?: string;
}

export interface BillingInvoiceLineItem {
  name?: string;
  total?: number;
  type?: string;
  credit_type?: BillingCreditType;
}

export interface BillingInvoice {
  id?: string;
  status?: string;
  type?: string;
  total?: number;
  subtotal?: number;
  credit_type?: BillingCreditType;
  start_timestamp?: string;
  end_timestamp?: string;
  issued_at?: string;
  line_items?: BillingInvoiceLineItem[];
}

export interface BillingUsageRow {
  billable_metric_id?: string;
  billable_metric_name?: string;
  start_timestamp?: string;
  end_timestamp?: string;
  value?: number;
  groups?: Record<string, number | null>;
}

export type BillingRecord = Record<string, unknown>;

export interface BillingBalances {
  credits: BillingRecord[];
  commits: BillingRecord[];
}

export interface SavedCard {
  id: string;
  brand: string;
  last4: string;
  exp_month: number;
  exp_year: number;
}

export interface PaymentMethodResponse {
  available: boolean;
  card?: SavedCard;
}

/** Gating state for the account. `credits_exhausted` + `has_payment_method`
 *  are reported alongside the status so the banner can distinguish "free
 *  credits spent" from a declined card without parsing `reason`. */
export interface BillingStatusResponse {
  status: 'active' | 'past_due' | 'suspended';
  reason?: string;
  credits_exhausted: boolean;
  has_payment_method: boolean;
  /** Whether the server acts on this status (observe vs enforce). */
  enforced: boolean;
  /** Whether billing has already stopped this account's deployments. Outlives
   *  `enforced`, so a real suspension stays visible after enforcement is
   *  turned off. */
  workloads_suspended: boolean;
  /** Whether this status is worth surfacing. The server owns the rule so the
   *  web client, the CLI, and the 402 body cannot disagree about it. */
  gated: boolean;
  /** The one action that lifts the gate, matching the 402 body's `action`. */
  action?: string;
}

export interface SetupIntentResponse {
  client_secret: string;
  publishable_key: string;
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
  private defaultSignal?: AbortSignal;

  constructor(
    baseUrl: string = '',
    authUrl: string = '',
    defaultHeaders: Record<string, string> = {},
    defaultSignal?: AbortSignal,
  ) {
    this.baseUrl = baseUrl;
    this.authUrl = authUrl;
    this.defaultHeaders = defaultHeaders;
    this.defaultSignal = defaultSignal;
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
    opts: {
      isFormData?: boolean;
      parseError?: (response: Response) => Promise<ApiError>;
    } = {},
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
      signal: init.signal ?? this.defaultSignal,
    });

    if (!response.ok) {
      const body: ApiError = opts.parseError
        ? await opts.parseError(response)
        : await response.json().catch(() => ({
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
    data: { bio?: string; location?: string; local_timezone?: string; pronouns?: string; website?: string; social_links?: string[]; blueprint_order?: string[] },
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

  async listUserBlueprints(
    scope: UserResourceScopeSelection,
    params?: UserBlueprintListParams,
  ): Promise<UserBlueprintsResponse> {
    const query = buildUserResourceQuery(scope, {
      q: params?.q,
      limit: params?.limit,
      cursor: params?.cursor,
    });
    if (params?.tag?.trim()) query.set("tag", params.tag.trim());
    if (params?.visibility) query.set("visibility", params.visibility);
    if (params?.sort) query.set("sort", params.sort);
    return this.request<UserBlueprintsResponse>(`/api/v1/me/blueprints?${query}`);
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

  async cancelDeployment(data: { deploymentId: string }): Promise<{ status: string; deployment_id: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/cancel`, {
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

  async listUserDeployments(
    scope: UserResourceScopeSelection,
    options?: UserResourceListOptions,
  ): Promise<UserDeploymentsResponse> {
    const query = buildUserResourceQuery(scope, options);
    return this.request<UserDeploymentsResponse>(`/api/v1/me/deployments?${query}`);
  }

  async getUserDeploymentSummaries(
    deploymentIDs: string[],
  ): Promise<DeploymentSummariesResponse> {
    const query = new URLSearchParams();
    deploymentIDs.forEach((id) => query.append("deployment", id));
    return this.request<DeploymentSummariesResponse>(`/api/v1/me/deployment-summaries?${query}`);
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
    attachments?: ChatAttachment[],
  ): Promise<MessagingSendMessageResponse> {
    // Only the key is sent per attachment; the sidecar re-reads authoritative
    // metadata from the file store, so this body stays small (no inlined bytes).
    const body: { content: string; attachments?: { key: string }[] } = {
      content,
    };
    if (attachments && attachments.length > 0) {
      body.attachments = attachments.map((a) => ({ key: a.key }));
    }
    return this.request<MessagingSendMessageResponse>(
      this.messagingProxyPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}/messages`,
      ),
      { method: "POST", body: JSON.stringify(body) },
    );
  }

  // Stop generating: asks the messaging sidecar to end the in-flight turn
  // (drop the agent's remaining output, persist the partial, finish the stream).
  async cancelMessagingStream(
    deploymentId: string,
    conversationId: string,
  ): Promise<void> {
    await this.request(
      this.messagingProxyPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}/cancel`,
      ),
      { method: "POST" },
    );
  }

  async respondToInteraction(
    deploymentId: string,
    conversationId: string,
    interactionId: string,
    body: InteractionResponseBody,
  ): Promise<InteractionResponseAck> {
    return this.request<InteractionResponseAck>(
      this.messagingProxyPath(
        deploymentId,
        `chat/conversations/${encodeURIComponent(conversationId)}/interactions/${encodeURIComponent(interactionId)}`,
      ),
      { method: "POST", body: JSON.stringify(body) },
    );
  }

  messagingStreamPath(deploymentId: string, conversationId: string): string {
    return this.messagingProxyPath(
      deploymentId,
      `conversations/${encodeURIComponent(conversationId)}/stream`,
    );
  }

  /** Agent self-reported config (system prompt + tools) via the messaging proxy.
   *  Accepts an AbortSignal so callers can bound the request — the sidecar can
   *  hang (the proxy upstream client has no timeout), so the inspector caps it. */
  async getDeploymentAgentConfig(
    deploymentId: string,
    signal?: AbortSignal,
  ): Promise<DeploymentAgentConfig> {
    return this.request<DeploymentAgentConfig>(
      this.messagingProxyPath(deploymentId, "agent/config"),
      { signal },
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

  // Sets the title of an existing conversation. Idempotent and rename-only (it
  // cannot create a conversation), so it's a PUT to the /title sub-resource.
  async setDeploymentChatConversationTitle(
    deploymentId: string,
    conversationId: string,
    body: { title: string },
  ): Promise<{ conversation_id: string; title: string }> {
    return this.request(
      this.deploymentChatPath(
        deploymentId,
        `conversations/${encodeURIComponent(conversationId)}/title`,
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

  /** Agent files API — per-deployment upload/download backed by the agent's
   *  persistent disk (or a presigned object store later). See the files handlers
   *  in astro-server for the proxy contract. */
  private deploymentFilesPath(deploymentId: string, subpath?: string) {
    const base = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/files`;
    return subpath ? `${base}/${subpath.replace(/^\//, "")}` : base;
  }

  private fileRequest<T>(
    endpoint: string,
    operation: FilesApiOperation,
    init: RequestInit = {},
  ): Promise<T> {
    return this._fetch<T>(`${this.baseUrl}${endpoint}`, init, {
      parseError: (response) => parseFilesApiError(response, operation),
    });
  }

  async listDeploymentFiles(
    deploymentId: string,
  ): Promise<ListDeploymentFilesResponse> {
    return this.fileRequest<ListDeploymentFilesResponse>(
      this.deploymentFilesPath(deploymentId),
      "list",
    );
  }

  async getDeploymentStorageUsage(
    deploymentId: string,
  ): Promise<DeploymentStorageUsage> {
    return this.fileRequest<DeploymentStorageUsage>(
      this.deploymentFilesPath(deploymentId, "usage"),
      "usage",
    );
  }

  async createDeploymentFile(
    deploymentId: string,
    input: { name: string; size: number; content_type: string },
  ): Promise<CreateDeploymentFileResponse> {
    return this.fileRequest<CreateDeploymentFileResponse>(
      this.deploymentFilesPath(deploymentId),
      "upload",
      { method: "POST", body: JSON.stringify(input) },
    );
  }

  // Two-step upload: reserve a key + upload target, then send the bytes to
  // wherever the server points us. The target is a relative content path today
  // (server-received PUT) and can become an absolute presigned URL with no
  // change here — we resolve and use whatever we're handed.
  async uploadDeploymentFile(
    deploymentId: string,
    file: File,
  ): Promise<DeploymentFileMeta> {
    const created = await this.createDeploymentFile(deploymentId, {
      name: file.name,
      size: file.size,
      content_type: file.type || "application/octet-stream",
    });

    const uploadUrl = this.resolveDeploymentFileUrl(
      deploymentId,
      created.upload.url,
    );
    // Only attach our session cookie when the target is our own origin; a
    // presigned object URL is cross-origin and must not receive it.
    const sameOrigin = this.isSameOrigin(uploadUrl);
    const response = await fetch(uploadUrl, {
      method: created.upload.method || "PUT",
      headers: {
        ...(created.upload.headers ?? {}),
        "Content-Type": file.type || "application/octet-stream",
      },
      body: file,
      credentials: sameOrigin ? "include" : "omit",
    });
    if (!response.ok) {
      const body = await parseFilesApiError(response, "upload");
      throw new ApiRequestError(body, response.status);
    }
    // The server-received PUT returns the reconciled metadata; a presigned PUT
    // returns no useful body, so fall back to the declared metadata.
    const text = await response.text();
    if (text) {
      try {
        return JSON.parse(text) as DeploymentFileMeta;
      } catch {
        // ignore — presigned stores don't return our JSON
      }
    }
    return created.file;
  }

  async downloadDeploymentFile(
    deploymentId: string,
    key: string,
  ): Promise<Blob> {
    // Default redirect-follow: a 200 stream (server-received) and a 3xx to a
    // presigned URL both resolve to the bytes with the same call.
    const url = `${this.baseUrl}${this.deploymentFilesPath(
      deploymentId,
      `${encodeURIComponent(key)}/content`,
    )}`;
    const response = await fetch(url, { credentials: "include" });
    if (!response.ok) {
      const body = await parseFilesApiError(response, "download");
      throw new ApiRequestError(body, response.status);
    }
    return response.blob();
  }

  async deleteDeploymentFile(
    deploymentId: string,
    key: string,
  ): Promise<void> {
    await this.fileRequest(
      this.deploymentFilesPath(deploymentId, encodeURIComponent(key)),
      "delete",
      { method: "DELETE" },
    );
  }

  // Resolve an upload target against the files collection base. Absolute URLs
  // (presigned) pass through unchanged; a relative content path resolves to the
  // proxied endpoint.
  private resolveDeploymentFileUrl(
    deploymentId: string,
    target: string,
  ): string {
    if (/^https?:\/\//i.test(target)) return target;
    const origin =
      typeof window !== "undefined"
        ? window.location.origin
        : "http://localhost";
    const base = new URL(
      `${this.baseUrl}${this.deploymentFilesPath(deploymentId)}/`,
      origin,
    );
    return new URL(target, base).toString();
  }

  private isSameOrigin(url: string): boolean {
    if (typeof window === "undefined") return true;
    try {
      return new URL(url, window.location.origin).origin ===
        window.location.origin;
    } catch {
      return true;
    }
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

  async getDeploymentAlerts(deploymentId: string, workload: string): Promise<DeploymentAlertsResponse> {
    return this.request<DeploymentAlertsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/alerts?workload=${encodeURIComponent(workload)}`
    );
  }

  async getEvalDataset(deploymentId: string): Promise<EvalDatasetResponse> {
    return this.request<EvalDatasetResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset`
    );
  }

  async getEvalDatasetItems(
    deploymentId: string,
    { page, cursor, limit, verdict }: EvalDatasetItemsParams,
  ): Promise<EvalDatasetItemsResponse> {
    return this.request<EvalDatasetItemsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/items${buildQS({
        limit: String(limit),
        page: page != null ? String(page) : undefined,
        cursor,
        verdict,
      })}`
    );
  }

  async getDatasetReviewQueue(
    deploymentId: string,
    { limit, cursor, prediction }: ReviewQueueParams = {},
  ): Promise<ReviewQueueResponse> {
    return this.request<ReviewQueueResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/review-queue${buildQS({
        limit: limit != null ? String(limit) : undefined,
        cursor,
        prediction,
      })}`
    );
  }

  async getDatasetPredictionStatus(
    deploymentId: string,
  ): Promise<PredictionStatusCounts> {
    return this.request<PredictionStatusCounts>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/predictions/status`,
    );
  }

  async postDatasetPredictions(
    deploymentId: string,
  ): Promise<DatasetPredictionsResponse> {
    return this.request<DatasetPredictionsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/predictions`,
      {
        method: "POST",
      },
    );
  }

  async postDatasetJudgment(
    deploymentId: string,
    body: DatasetJudgmentRequest,
  ): Promise<DatasetJudgmentResponse> {
    return this.request<DatasetJudgmentResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/judgments`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    );
  }

  async patchDatasetJudgment(
    deploymentId: string,
    traceId: string,
    body: Pick<DatasetJudgmentRequest, "verdict">,
  ): Promise<DatasetJudgmentResponse> {
    return this.request<DatasetJudgmentResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/judgments/${encodeURIComponent(traceId)}`,
      {
        method: "PATCH",
        body: JSON.stringify(body),
      },
    );
  }

  async deleteDatasetJudgment(
    deploymentId: string,
    traceId: string,
  ): Promise<DatasetJudgmentResponse> {
    return this.request<DatasetJudgmentResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/judgments/${encodeURIComponent(traceId)}`,
      { method: "DELETE" },
    );
  }

  async putDatasetJudgmentCriteria(
    deploymentId: string,
    traceId: string,
    body: { criteria: JudgmentCriterion[] },
  ): Promise<DatasetJudgmentCriteriaResponse> {
    return this.request<DatasetJudgmentCriteriaResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/judgments/${encodeURIComponent(traceId)}/criteria`,
      {
        method: "PUT",
        body: JSON.stringify(body),
      },
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

  /**
   * `version` selects the read path: "v1" is the Langfuse-plus-Redis endpoint,
   * "v2" is served from the Postgres rollup store. The response shape is
   * identical by design, so callers need no branching — but the version must be
   * part of the caller's cache key, or a toggle would serve the other path's
   * cached response.
   */
  async getAccountInsights(
    account: string,
    params?: InsightsQueryParams,
    version: "v1" | "v2" = "v1"
  ): Promise<InsightsResponse> {
    return this.request<InsightsResponse>(
      `/api/${version}/accounts/${encodeURIComponent(account)}/insights${buildQS(params)}`
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

  async getObservabilityTraceUsers(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<TraceUserFacetsResponse> {
    return this.request<TraceUserFacetsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/trace-users${buildQS(params)}`
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

  async listUserKnowledgeStores(
    scope: UserResourceScopeSelection,
    options?: UserResourceListOptions,
  ): Promise<UserKnowledgeStoresResponse> {
    const query = buildUserResourceQuery(scope, options);
    return this.request<UserKnowledgeStoresResponse>(`/api/v1/me/knowledge?${query}`);
  }

  async getKnowledgeStore(account: string, name: string): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}`
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

  // ── Supabase OAuth ─────────────────────────────────────────────────────────

  async supabaseConnect(account: string, redirectTo: string): Promise<{ redirect_url?: string; connected?: boolean }> {
    return this.request<{ redirect_url?: string; connected?: boolean }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/supabase/connect`,
      { method: 'POST', body: JSON.stringify({ redirect_to: redirectTo }) }
    );
  }

  async supabaseStatus(account: string): Promise<{ connected: boolean }> {
    return this.request<{ connected: boolean }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/supabase/status`
    );
  }

  async supabaseListProjects(account: string): Promise<{ projects: SupabaseProject[] }> {
    return this.request<{ projects: SupabaseProject[] }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/supabase/projects`
    );
  }

  async supabaseProjectHealth(account: string, ref: string): Promise<{ services: SupabaseServiceHealth[] }> {
    return this.request<{ services: SupabaseServiceHealth[] }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/supabase/projects/${encodeURIComponent(ref)}/health`
    );
  }

  async supabaseDisconnect(account: string): Promise<{ disconnected: boolean }> {
    return this.request<{ disconnected: boolean }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/supabase`,
      { method: 'DELETE' }
    );
  }

  async recheckKnowledgeStore(account: string, name: string): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/recheck`,
      { method: 'POST' }
    );
  }

  async getKnowledgeCredentials(account: string, name: string): Promise<KnowledgeCredentials> {
    return this.request<KnowledgeCredentials>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/credentials`
    );
  }

  async updateKnowledgeCredentials(account: string, name: string, data: UpdateKnowledgeCredentialsInput): Promise<KnowledgeStore> {
    return this.request<KnowledgeStore>(
      `/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/credentials`,
      { method: 'PUT', body: JSON.stringify(data) }
    );
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

  async gitHubListAccountBranches(account: string, repo: string): Promise<{ branches: string[] }> {
    const params = new URLSearchParams({ repo });
    return this.request<{ branches: string[] }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/branches?${params}`
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
  // OTel ingest keys (developer-tools telemetry)
  // --------------------------------------------------------------------------

  async listOtelIngestKeys(account: string): Promise<OtelIngestKeysListResponse> {
    return this.request<OtelIngestKeysListResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/otel-keys`
    );
  }

  async createOtelIngestKey(
    account: string,
    name: string,
  ): Promise<CreateOtelIngestKeyResponse> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/otel-keys`,
      { method: 'POST', body: JSON.stringify({ name }) }
    );
  }

  async revokeOtelIngestKey(
    account: string,
    keyId: string,
  ): Promise<{ message: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/otel-keys/${encodeURIComponent(keyId)}`,
      { method: 'DELETE' }
    );
  }

  // Rename a source in place — no key rotation, key never revealed.
  async renameOtelIngestKey(
    account: string,
    keyId: string,
    name: string,
  ): Promise<{ name: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/otel-keys/${encodeURIComponent(keyId)}/name`,
      { method: 'PATCH', body: JSON.stringify({ name }) }
    );
  }

  // Edit a key's exclusion list in place — no key rotation, key never revealed.
  async updateOtelIngestKeyExclusions(
    account: string,
    keyId: string,
    excludedEmails: string[],
  ): Promise<{ excluded_emails: string[] }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/otel-keys/${encodeURIComponent(keyId)}/exclusions`,
      { method: 'PATCH', body: JSON.stringify({ excluded_emails: excludedEmails }) }
    );
  }

  // --------------------------------------------------------------------------
  // Notification preferences
  // --------------------------------------------------------------------------

  async getNotificationPreferences(account: string): Promise<NotificationPreferencesResponse> {
    return this.request<NotificationPreferencesResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/notification-preferences`
    );
  }

  async updateNotificationPreference(
    account: string,
    data: UpdateNotificationPreferenceInput,
  ): Promise<{ message: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/notification-preferences`,
      { method: 'PATCH', body: JSON.stringify(data) }
    );
  }

  async sendTestNotification(account: string): Promise<{ message: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/notification-preferences/test`,
      { method: 'POST' }
    );
  }

  async getNotificationInboxConfig(): Promise<NotificationInboxConfig> {
    return this.request<NotificationInboxConfig>('/api/v1/notifications/inbox-config');
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

  async getBillingUsage(
    account: string,
    params?: { from?: string; to?: string },
  ): Promise<BillingDataResponse<BillingUsageRow[]>> {
    const qs = params
      ? `?${new URLSearchParams(Object.entries(params).filter(([, v]) => v) as [string, string][])}`
      : "";
    return this.request<BillingDataResponse<BillingUsageRow[]>>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/usage${qs}`
    );
  }

  async getBillingInvoices(
    account: string,
  ): Promise<BillingDataResponse<BillingInvoice[]>> {
    return this.request<BillingDataResponse<BillingInvoice[]>>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/invoices`
    );
  }

  async getInvoicePdf(account: string, invoiceId: string): Promise<Blob> {
    const res = await fetch(
      `${this.baseUrl}/api/v1/accounts/${encodeURIComponent(account)}/billing/invoices/${encodeURIComponent(invoiceId)}/pdf`,
      { credentials: "include" },
    );
    if (!res.ok) throw new Error(`Failed to load invoice PDF (${res.status})`);
    return res.blob();
  }

  async getBillingBalances(
    account: string,
  ): Promise<BillingDataResponse<BillingBalances>> {
    return this.request<BillingDataResponse<BillingBalances>>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/balances`
    );
  }

  async createSetupIntent(account: string): Promise<SetupIntentResponse> {
    return this.request<SetupIntentResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/setup-intent`,
      { method: 'POST' }
    );
  }

  async confirmPaymentMethod(
    account: string,
    setupIntentId: string,
  ): Promise<PaymentMethodResponse> {
    return this.request<PaymentMethodResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/payment-method`,
      { method: 'POST', body: JSON.stringify({ setup_intent_id: setupIntentId }) }
    );
  }

  async getBillingStatus(account: string): Promise<BillingStatusResponse> {
    return this.request<BillingStatusResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/status`
    );
  }

  async getPaymentMethod(account: string): Promise<PaymentMethodResponse> {
    return this.request<PaymentMethodResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/payment-method`
    );
  }

  async deletePaymentMethod(account: string): Promise<{ status: string }> {
    return this.request(
      `/api/v1/accounts/${encodeURIComponent(account)}/billing/payment-method`,
      { method: 'DELETE' }
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
