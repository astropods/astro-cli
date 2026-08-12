export interface DeploymentErrorDetail {
  resource?: string;
  kind?: string;
  error: string;
}

export interface AdminDeployment {
  name: string;
  build_id: string;
  namespace: string;
  status: string;
  created_at: string;
  account_name: string;
  components: string[];
  deployment_id: string;
  error_message?: string;
  error_details?: DeploymentErrorDetail[];
  status_changed_at?: string;
  current_revision?: number;
  owner_email?: string;
  cluster_id?: string;
  account_cluster_id?: string;
  placement_mismatch?: boolean;
  account_id?: string;
}

export interface DeploymentEvent {
  status: string;
  message?: string;
  created_at: string;
}

export interface DeploymentRevision {
  revision: number;
  build_id: string;
  created_at: string;
}

export interface ListDeploymentsResponse {
  deployments: AdminDeployment[];
  count: number;
}

export interface AdminWorkload {
  name: string;
  component_kind: string;
  component_key: string;
  workload_type: string;
  image: string;
  replicas: number;
  cpu_request: string;
  memory_request: string;
  persistent: boolean;
}

export interface ExpectedService {
  name: string;
  port: number;
  target_port: number;
  protocol: string;
  workload_name?: string;
}

export interface ExpectedIngress {
  hostname: string;
  path: string;
  service: string;
}

export interface AdminVariable {
  name: string;
  secret: boolean;
  optional: boolean;
  value: string;
  targets: string[];
}

export interface GetDeploymentResponse {
  deployment: AdminDeployment;
  spec_json: string;
  cluster_status: ClusterStatusResponse;
  events?: DeploymentEvent[];
  revisions?: DeploymentRevision[];
  workloads?: AdminWorkload[];
  expected_services?: ExpectedService[];
  expected_ingresses?: ExpectedIngress[];
  variables?: AdminVariable[];
  adapters?: string[];
  placement_hint?: string;
}

export interface GetDeploymentEventsResponse {
  events: DeploymentEvent[];
}

export interface DeploymentJob {
  job_id?: number;
  kind: string;
  state: string;
  attempt: number;
  max_attempt: number;
  created_at: string;
  attempted_at?: string;
  finalized_at?: string;
  errors?: string;
  cluster_id?: string;
}

export interface ReapplyDeploymentResponse {
  status: string;
  cluster_placement_updated?: boolean;
  message?: string;
}

export interface GetDeploymentJobsResponse {
  jobs: DeploymentJob[];
}

export interface ClusterStatusResponse {
  timestamp: string;
  namespace: string;
  deployments: K8sDeploymentInfo[];
  statefulsets: K8sDeploymentInfo[];
  pods: K8sPodInfo[];
  services: K8sServiceInfo[];
  ingresses: K8sIngressInfo[];
  network_policies: K8sNetworkPolicyInfo[];
  events: K8sEventInfo[];
  summary: ClusterSummary;
  resolved_cluster_id?: string;
}

export interface ClusterSummary {
  total_deployments: number;
  total_pods: number;
  running_pods: number;
  pending_pods: number;
  failed_pods: number;
  total_services: number;
  total_ingresses: number;
  total_network_policies: number;
  total_events: number;
  warning_events: number;
}

export interface K8sDeploymentInfo {
  name: string;
  namespace: string;
  replicas: number;
  ready_replicas: number;
  available_replicas: number;
  labels: Record<string, string>;
  created_at: string;
}

export interface K8sPodInfo {
  name: string;
  namespace: string;
  phase: string;
  node_name: string;
  pod_ip: string;
  labels: Record<string, string>;
  created_at: string;
  container_statuses: K8sContainerStatus[];
  containers: K8sContainerResources[];
  conditions: string[];
  pod_security?: K8sPodSecurityContext;
  service_account?: string;
  automount_service_token?: boolean;
  volumes?: K8sVolume[];
}

export interface K8sPodSecurityContext {
  run_as_user?: number;
  run_as_group?: number;
  fs_group?: number;
  seccomp_profile?: string;
}

export interface K8sContainerStatus {
  name: string;
  ready: boolean;
  restart_count: number;
  state: string;
  image: string;
  init?: boolean; // from initContainers (includes native sidecars)
}

export interface K8sContainerResources {
  name: string;
  request_cpu: string;
  request_memory: string;
  limit_cpu: string;
  limit_memory: string;
  security?: K8sContainerSecurityContext;
  volume_mounts?: K8sVolumeMount[];
  env_from?: string[];
  image_pull_policy?: string;
  init?: boolean; // from initContainers
  sidecar?: boolean; // init container with restartPolicy: Always
}

export interface K8sContainerSecurityContext {
  run_as_user?: number;
  run_as_non_root?: boolean;
  read_only_root_filesystem?: boolean;
  allow_privilege_escalation?: boolean;
  privileged?: boolean;
  capabilities?: string[];
  add_capabilities?: string[];
  seccomp_profile?: string;
}

export interface K8sVolumeMount {
  name: string;
  mount_path: string;
  read_only?: boolean;
  sub_path?: string;
}

export interface K8sVolume {
  name: string;
  type: string;
  source?: string;
}

export interface K8sServiceInfo {
  name: string;
  namespace: string;
  type: string;
  cluster_ip: string;
  external_ip: string[];
  ports: K8sServicePort[];
  labels: Record<string, string>;
  created_at: string;
}

export interface K8sServicePort {
  name: string;
  port: number;
  target_port: string;
  protocol: string;
}

export interface K8sIngressInfo {
  name: string;
  namespace: string;
  ingress_class_name: string;
  rules: K8sIngressRule[];
  tls: K8sIngressTLS[];
  labels: Record<string, string>;
  created_at: string;
}

export interface K8sIngressRule {
  host: string;
  paths: K8sIngressPath[];
}

export interface K8sIngressPath {
  path: string;
  path_type: string;
  backend_service: string;
  backend_port: string;
}

export interface K8sIngressTLS {
  hosts: string[];
  secret_name: string;
}

export interface K8sNetworkPolicyInfo {
  name: string;
  namespace: string;
  policy_types: string[];
  ingress_rule_count: number;
  egress_rule_count: number;
  pod_selector_labels: Record<string, string>;
  labels: Record<string, string>;
  created_at: string;
}

export interface K8sEventInfo {
  name: string;
  namespace: string;
  type: string;
  reason: string;
  message: string;
  involved_object: string;
  count: number;
  first_seen: string;
  last_seen: string;
}

export interface ImageInfo {
  repository: string;
  namespace: string;
  name: string;
  tags: string[];
}

export interface ListImagesResponse {
  images: ImageInfo[];
  count: number;
}

export interface AdminAccount {
  id: string;
  name: string;
  type: string;
  owner_user_id: string;
  member_count: number;
  has_langfuse: boolean;
  deleted_at?: string;
  created_at: string;
  updated_at: string;
  cluster_id?: string;
  billing_status?: string; // active | past_due | suspended; "" = never billed
}

export interface ListAccountsResponse {
  accounts: AdminAccount[];
  count: number;
}

export interface AccountBillingInfo {
  status: string; // active | past_due | suspended; "" = never billed
  reason: string;
  dunning_since: string;
  alert_active: boolean;
  updated_at: string;
  metronome_customer_id: string;
  stripe_customer_id: string;
  bifrost_customer_id: string;
}

export interface AccountResourceLimit {
  resource: string;
  used: number;
  limit: number; // -1 unlimited, 0 disabled
}

export interface AccountMemberInfo {
  user_id: string;
  email: string;
  created_at: string;
  is_owner: boolean;
}

export interface GetAccountResponse {
  account: AdminAccount;
  billing?: AccountBillingInfo;
  limits?: AccountResourceLimit[];
  members?: AccountMemberInfo[];
  langfuse_project_id?: string;
}

export interface MetronomeAliasStatus {
  configured: boolean; // provider available and account has a Metronome customer
  ok: boolean;
  expected?: string[];
  actual?: string[];
  missing?: string[];
  error?: string;
}

export interface RegisterAccountMetronomeResponse {
  metronome_customer_id?: string;
}

export interface BillingContract {
  id: string;
  name?: string;
  rate_card_id?: string;
  starting_at?: string;
  ending_before?: string;
}

export interface BillingProvisionJob {
  id?: number;
  state?: string;
  attempt: number;
  created_at?: string;
  finalized_at?: string;
  last_error?: string;
}

// Amounts carry a presence flag because zero and absent are different: a
// customer with no draft invoice and one whose lookup failed both show nothing.
export interface BillingSpend {
  currency?: string;
  credit_remaining: number;
  has_credit: boolean;
  current_spend: number;
  current_period_end?: string;
  has_current_spend: boolean;
  last_invoice_total: number;
  last_invoice_at?: string;
  has_last_invoice: boolean;
}

export interface BillingCard {
  brand?: string;
  last4?: string;
  exp_month: number;
  exp_year: number;
}

export interface GetAccountBillingDetailResponse {
  billing?: AccountBillingInfo;
  provisioned_at?: string;
  enforced: boolean;
  workloads_suspended: boolean;
  coverage?: "covered" | "none" | "unknown";
  contracts?: BillingContract[];
  provision_job?: BillingProvisionJob;
  card?: BillingCard;
  spend?: BillingSpend;
  metronome_url?: string;
  stripe_url?: string;
  warnings?: string[];
}

export interface RetryBillingProvisionResponse {
  status?: string;
}

export interface ForceBillingResumeResponse {
  status?: string;
}

export interface RecoverAccountLangfuseResponse {
  langfuse_project_id?: string;
}

export interface RecoverAccountBifrostResponse {
  bifrost_customer_id?: string;
}

export interface AdminBlueprint {
  account_name: string;
  name: string;
  build_count: number;
  latest_build_id: string;
  created_at: string;
  updated_at: string;
}

export interface ListBlueprintsResponse {
  agents: AdminBlueprint[];
  count: number;
}

export interface BlueprintBuild {
  build_id: string;
  published_at: string;
  updated_at: string;
}

export interface GetBlueprintBuildsResponse {
  builds: BlueprintBuild[];
  count: number;
}

export interface ColumnInfo {
  table_name: string;
  column_name: string;
  data_type: string;
}

export interface GetSchemaResponse {
  columns: ColumnInfo[];
}

export interface QueryDatabaseResponse {
  columns: string[];
  rows: { values: string[] }[];
}

export interface GetPodLogsResponse {
  logs: string;
}

export interface ContainerEnv {
  container: string;
  vars: { name: string; value: string; value_from: string }[];
}

export interface GetPodEnvResponse {
  containers: ContainerEnv[];
}

export interface ConnectedDevice {
  id: string;
  account_id: string;
  user_id: string;
  device_id: string;
  hostname: string;
  os: string;
  arch: string;
  cli_version: string;
  status: string;
  last_heartbeat_at: string;
  connected_at: string;
  disconnected_at: string;
  account_name: string;
}

export interface ListConnectedDevicesResponse {
  devices: ConnectedDevice[];
  count: number;
}

export interface SendCommandResponse {
  command_id: string;
  exit_code: number;
  stdout: string;
  stderr: string;
}

export interface RegisteredCluster {
  id: string;
  region: string;
  eks_cluster_name: string;
  eks_cluster_endpoint: string;
  enabled: boolean;
  is_primary: boolean;
  created_at: string;
  updated_at: string;
  healthy: boolean;
  health_error: string;
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
  /** IPv6 counterpart to pod_subnet_cidrs. Empty for IPv4-only clusters. */
  pod_subnet_ipv6_cidrs: string;
  /** Optional per-cluster observability query endpoints. Empty means this
   * cluster is queried through the global LOKI_URL/PROMETHEUS_URL instead. */
  loki_url: string;
  prometheus_url: string;
  /** Optional private (non-OIDC) address:port for this cluster's tenant-router
   * Envoy, over PrivateLink. Empty means the in-app chat proxy still relays
   * through the K8s apiserver's services/proxy subresource instead. */
  tenant_router_internal_url: string;
  /** Base64-encoded PEM from `aws eks describe-cluster`. Omitted for primary. */
  eks_cluster_ca?: string;
}

export interface ListClustersResponse {
  clusters: RegisteredCluster[];
}

export interface RegisterClusterRequest {
  id: string;
  region: string;
  eks_cluster_name: string;
  eks_cluster_endpoint: string;
  /** Base64-encoded PEM from `aws eks describe-cluster`. */
  eks_cluster_ca: string;
  enabled?: boolean;
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
  /** Optional — empty for IPv4-only clusters (every cluster except the pm-eu IPv6 pilot). */
  pod_subnet_ipv6_cidrs?: string;
  /** Optional — empty means this cluster is queried through the global
   * LOKI_URL/PROMETHEUS_URL instead of its own observability stack. */
  loki_url?: string;
  prometheus_url?: string;
  /** Optional — empty means the in-app chat proxy relays through the K8s
   * apiserver's services/proxy subresource instead of this cluster's own
   * PrivateLink path to the tenant-router Envoy. */
  tenant_router_internal_url?: string;
}

export interface RegisterClusterResponse {
  cluster: RegisteredCluster;
}

export interface EnableClusterResponse {
  cluster: RegisteredCluster;
}

export interface DisableClusterResponse {
  cluster: RegisteredCluster;
}

export interface UpdateClusterRequest {
  region: string;
  eks_cluster_name: string;
  eks_cluster_endpoint: string;
  /** Base64-encoded PEM from `aws eks describe-cluster`. */
  eks_cluster_ca: string;
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
  /** Optional — empty for IPv4-only clusters (every cluster except the pm-eu IPv6 pilot). */
  pod_subnet_ipv6_cidrs?: string;
  /** Optional — empty means this cluster is queried through the global
   * LOKI_URL/PROMETHEUS_URL instead of its own observability stack. */
  loki_url?: string;
  prometheus_url?: string;
  /** Optional — empty means the in-app chat proxy relays through the K8s
   * apiserver's services/proxy subresource instead of this cluster's own
   * PrivateLink path to the tenant-router Envoy. */
  tenant_router_internal_url?: string;
}

export interface UpdateClusterResponse {
  cluster: RegisteredCluster;
}

export interface ClusterBlocker {
  id: string;
  name: string;
  /** Empty for accounts; deployment status otherwise. */
  status: string;
}

export interface GetClusterBlockersResponse {
  account_count: number;
  accounts: ClusterBlocker[];
  deployment_count: number;
  deployments: ClusterBlocker[];
}

export interface UrlReachability {
  label: string;
  url: string;
  reachable: boolean;
  error: string;
}

export interface CheckClusterHealthResponse {
  cluster: RegisteredCluster;
  /** TCP reachability of langfuse/loki/prometheus/tenant-router URLs. Only
   * populated for fields the cluster actually has set. */
  url_checks?: UrlReachability[];
}

export interface SetAccountClusterResponse {
  status?: string;
  cluster_id?: string;
}

export interface MigrateAccountDeploymentsResponse {
  migrations_enqueued?: number;
  deployment_ids?: string[];
}

export interface InvalidateCachesResponse {
  accounts_busted: number;
  deployments_busted: number;
}

export interface RefreshMessagingCacheResponse {
  image?: string;
  message?: string;
}

export interface ClusterMigrationEvent {
  deployment_id: string;
  account_name: string;
  agent_name: string;
  status: string;
  message: string;
  created_at: string;
}

export interface ClusterMigrationJob {
  job_id: number;
  kind: string;
  state: string;
  deployment_id: string;
  args_json: string;
  errors?: string;
  created_at: string;
  finalized_at?: string;
  attempt: number;
  max_attempt: number;
  duration_ms: number;
  account_name?: string;
  agent_name?: string;
  source_cluster_id?: string;
  target_cluster_id?: string;
  deploy_cluster_id?: string;
}

export interface ListClusterMigrationsResponse {
  events: ClusterMigrationEvent[];
  jobs: ClusterMigrationJob[];
  mismatch_count: number;
}

export interface AlertCondition {
  name: string;
  title: string;
  description: string;
  severity: string; // "info" | "warning" | "critical"
}

export interface ActiveAlert {
  deployment_id: string;
  agent_name?: string;
  account_id?: string;
  account_name?: string;
  workload?: string;
  condition: string;
  title?: string;
  severity?: string;
  state: string; // "pending" | "firing" | "ok" (mute with no active breach)
  active_since?: string;
  muted?: boolean;
  muted_until?: string;
  last_notified?: string;
}

export interface ListAlertsResponse {
  catalog: AlertCondition[];
  active: ActiveAlert[];
}
