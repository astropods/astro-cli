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
  drift_summary?: DriftSummary;
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

export interface DriftResourceItem {
  name: string;
  type: string;     // deployment, statefulset, service, ingress
  status: string;   // match, missing, extra, drift
  expected: Record<string, string>;
  actual: Record<string, string>;
}

export interface DriftSummary {
  total: number;
  match: number;
  missing: number;
  extra: number;
  drift: number;
}

export interface DriftReport {
  detected_at: string;
  workloads: DriftResourceItem[];
  services: DriftResourceItem[];
  ingresses: DriftResourceItem[];
  env_vars?: DriftResourceItem[];
  secrets?: DriftResourceItem[];
  summary: DriftSummary;
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
  drift_report?: DriftReport;
  drift_checked_at?: string;
  variables?: AdminVariable[];
  adapters?: string[];
}

export interface RefreshDriftReportResponse {
  drift_report?: DriftReport;
  drift_checked_at?: string;
}

export interface GetDeploymentEventsResponse {
  events: DeploymentEvent[];
}

export interface DeploymentJob {
  kind: string;
  state: string;
  attempt: number;
  max_attempt: number;
  created_at: string;
  attempted_at?: string;
  finalized_at?: string;
  errors?: string;
}

export interface GetDeploymentJobsResponse {
  jobs: DeploymentJob[];
  last_reconcile_at?: string;
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
  created_at: string;
  updated_at: string;
}

export interface ListAccountsResponse {
  accounts: AdminAccount[];
  count: number;
}

export interface AdminAgent {
  account_name: string;
  name: string;
  build_count: number;
  latest_build_id: string;
  created_at: string;
  updated_at: string;
}

export interface ListAgentsResponse {
  agents: AdminAgent[];
  count: number;
}

export interface AgentBuild {
  build_id: string;
  published_at: string;
  updated_at: string;
}

export interface GetAgentBuildsResponse {
  builds: AgentBuild[];
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
