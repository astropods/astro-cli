// API client for communicating with the astro-server backend

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

export interface AgentSummary {
  name: string;
  registry: string;
  build_count: number;
}

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
  avatar_version?: number;
  created_at: string;
  updated_at: string;
}

export interface Account {
  id: string;
  name: string;
  type: string;
  display_name?: string;
  role?: string;
  avatar_version?: number;
  agents?: AgentSummary[];
}

export interface AccountSearchResult {
  id: string;
  name: string;
  type: string;
}

export interface AccountSearchResponse {
  results: AccountSearchResult[];
}

export interface ProfileResponse {
  user: User;
  accounts: Account[];
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

export interface Invitation {
  id: string;
  email: string;
  role: string;
  status: string;
  invited_by: string;
  created_at: string;
  expires_at: string;
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
  invitation?: Invitation;
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

class ApiClient {
  private baseUrl: string;
  private authUrl: string;
  private defaultHeaders: Record<string, string>;

  constructor(baseUrl: string = '', authUrl: string = '', defaultHeaders: Record<string, string> = {}) {
    this.baseUrl = baseUrl;
    this.authUrl = authUrl;
    this.defaultHeaders = defaultHeaders;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;

    const response = await fetch(url, {
      ...options,
      credentials: 'include', // Include cookies for session auth
      headers: {
        'Content-Type': 'application/json',
        ...this.defaultHeaders,
        ...options.headers,
      },
    });

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      error.status = response.status;
      throw error;
    }

    // Handle empty responses (204 No Content, etc.)
    const text = await response.text();
    if (!text) {
      return {} as T;
    }

    return JSON.parse(text);
  }

  // Auth endpoints - use authUrl for direct backend communication
  async getCurrentUser(): Promise<AuthResponse> {
    const url = `${this.authUrl}/auth/me`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...this.defaultHeaders },
    });
    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      error.status = response.status;
      throw error;
    }
    return response.json();
  }

  async refreshSession(): Promise<AuthResponse> {
    const url = `${this.authUrl}/auth/refresh`;
    const response = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...this.defaultHeaders },
    });
    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      error.status = response.status;
      throw error;
    }
    return response.json();
  }

  getLoginUrl(): string {
    return `${this.authUrl}/auth/login`;
  }

  getLogoutUrl(): string {
    // Pass current origin as redirect parameter for reliable post-logout redirect
    const origin = typeof window !== 'undefined' ? window.location.origin : '';
    const redirectUrl = encodeURIComponent(origin);
    return `${this.authUrl}/auth/logout?redirect=${redirectUrl}`;
  }

  // Profile endpoints
  async getProfile(): Promise<ProfileResponse> {
    return this.request<ProfileResponse>('/api/v1/me');
  }

  async updateProfile(data: { display_name: string }): Promise<{ user: User }> {
    return this.request<{ user: User }>('/api/v1/me', {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  // Account endpoints
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

  async getAccount(name: string): Promise<AccountPublic> {
    return this.request<AccountPublic>(
      `/api/v1/accounts/${encodeURIComponent(name)}`
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

  // Agent endpoints
  async listAgents(): Promise<AgentsListResponse> {
    return this.request<AgentsListResponse>('/api/v1/agents');
  }

  async listAccountAgents(account: string): Promise<AgentsListResponse> {
    return this.request<AgentsListResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}`
    );
  }

  async getAgent(account: string, name: string): Promise<Agent> {
    return this.request<Agent>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}`
    );
  }

  async deployAgent(deploySpec: DeploymentSpec): Promise<DeployResponse> {
    return this.request<DeployResponse>('/api/v1/deploy', {
      method: 'POST',
      body: JSON.stringify(deploySpec),
    });
  }

  async validateDeployment(deploySpec: DeploymentSpec): Promise<ValidateDeploymentResponse> {
    return this.request<ValidateDeploymentResponse>('/api/v1/deploy/validate', {
      method: 'POST',
      body: JSON.stringify(deploySpec),
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

  async pauseDeployment(data: { deploymentId: string }): Promise<{ status: string; deployment_id: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/pause`, {
      method: "POST",
    });
  }

  async wakeupDeployment(data: { deploymentId: string }): Promise<{ status: string; deployment_id: string }> {
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.deploymentId)}/wakeup`, {
      method: "POST",
    });
  }

  // Get deployment template for an agent (resolves latest build server-side)
  async getDeploymentTemplate(account: string, name: string): Promise<DeploymentTemplate> {
    return this.request<DeploymentTemplate>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment-template?format=json`
    );
  }

  // Get deployment template pre-filled with values from an existing deployment
  async getPrefilledDeploymentTemplate(account: string, name: string, deploymentId: string): Promise<DeploymentTemplate> {
    return this.request<DeploymentTemplate>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment-template/${encodeURIComponent(deploymentId)}?format=json`
    );
  }

  // List current deployments for an account
  async listDeployments(account: string): Promise<DeploymentsListResponse> {
    return this.request<DeploymentsListResponse>(
      `/api/v1/deployments?account=${encodeURIComponent(account)}`
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
  ): Promise<string> {
    const params = new URLSearchParams({ workload: workloadName, container });
    if (since) params.set('since', since);
    const url = `${this.baseUrl}/api/v1/deployments/${encodeURIComponent(deploymentId)}/logs?${params}`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: { ...this.defaultHeaders },
    });
    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        details: `Request failed with status ${response.status}`,
      }));
      error.status = response.status;
      throw error;
    }
    return response.text();
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

  // Observability endpoints (deployment-scoped, backed by Langfuse)
  async getObservabilityMetrics(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityMetricsResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilityMetricsResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/metrics${qs}`
    );
  }

  async getObservabilitySummary(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilitySummaryResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilitySummaryResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/summary${qs}`
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

  async getObservabilityTraces(
    deploymentId: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityTracesResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilityTracesResponse>(
      `/api/v1/deployments/${encodeURIComponent(deploymentId)}/observability/traces${qs}`
    );
  }

  async archiveAgent(account: string, name: string): Promise<void> {
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

  // Avatar endpoints
  private async uploadFormData<T>(
    endpoint: string,
    formData: FormData,
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const response = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: { ...this.defaultHeaders },
      body: formData,
    });

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      error.status = response.status;
      throw error;
    }

    const text = await response.text();
    if (!text) {
      return {} as T;
    }

    return JSON.parse(text);
  }

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
}

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

// Response types
export interface AgentSpec {
  meta?: { visibility?: string };
  integrations?: Record<string, { provider: string; type?: string }>;
  [key: string]: unknown;
}

export interface AgentCardAuthor {
  name: string;
  account?: string;
}

export interface ResolvedIntegration {
  id: string;
  name: string;
  known: boolean;
}

export interface AgentCardData {
  description?: string;
  tags?: string[];
  authors?: AgentCardAuthor[];
  capabilities?: string[];
  integrations?: ResolvedIntegration[];
  body?: string;
}

export interface AgentVersion {
  build_id: string;
  version?: string;
  spec: AgentSpec;
  readme?: string;
  agent_card?: AgentCardData;
  published_at: string;
  validation_warnings?: ValidationError[];
}

export interface AgentMetrics {
  lifetime_messages: number;
}

export interface Agent {
  name: string;
  account: string;
  registry: string;
  visibility?: string;
  versions: AgentVersion[];
  heart_count?: number;
  hearted?: boolean;
  metrics?: AgentMetrics;
}

export interface AgentsListResponse {
  agents: Agent[];
  count: number;
}

export interface DeploymentVariable {
  value?: string;
  targets: string[];
  secret?: boolean;
  optional?: boolean;
  // template-only (present in deployment-template/v1, stripped in deployment/v1)
  default?: string;
  description?: string;
  datatype?: string;
  'display-as'?: string;
  options?: string[];
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
  editable?: string[];
}

export type DeploymentSpec = Omit<DeploymentTemplate, 'spec' | 'editable'> & {
  spec: 'deployment/v1';
};

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

export interface ServiceEndpointInfo {
  name: string;
  url: string;
  type?: string;
}

export interface EnvVar {
  name: string;
  value?: string;
  from?: string;
}

export interface ContainerStatus {
  name: string;
  state: string;
  ready: boolean;
  restart_count: number;
  reason?: string;
  message?: string;
  env?: EnvVar[];
}

export interface WorkloadDetail {
  name: string;
  kind: string;
  component: string;
  age: string;
  containers: ContainerStatus[];
  urls?: ServiceEndpointInfo[];
}

export interface JobDetail {
  name: string;
  status: string;
  component: string;
  age: string;
  start_time?: string;
  completions: string;
}

export interface AgentDeployment {
  id: string;
  name: string;
  display_name?: string;
  build_id: string;
  namespace: string;
  status: string;
  replicas: number;
  ready: number;
  created_at: string;
  components: string[];
  manual_ingestions?: string[];
  external_urls?: ServiceEndpointInfo[];
  workloads?: WorkloadDetail[];
  jobs?: JobDetail[];
}

export interface DeploymentsListResponse {
  deployments: AgentDeployment[];
  count: number;
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
  build_id: string;
  namespace: string;
  status: string;
  deployed_at: string;
  undeployed_at?: string;
  spec: Record<string, unknown>;
}

export interface DeploymentHistoryResponse {
  deployments: DeploymentHistoryRecord[];
  count: number;
}

// Observability types
export interface MetricsBucket {
  timestamp: string;
  trace_count: number;
  avg_latency_ms: number;
  p95_latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  error_count: number;
}

export interface ObservabilityMetricsResponse {
  buckets: MetricsBucket[];
  time_range: { start: string; end: string };
  interval_minutes: number;
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

export interface TraceEntry {
  trace_id: string;
  name: string;
  status: string;
  latency_ms: number;
  total_tokens?: number;
  input: string;
  output: string;
  timestamp: string;
}

export interface ObservabilityTracesResponse {
  traces: TraceEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface TriggerIngestionResponse {
  status: string;
  job_name: string;
  namespace: string;
}

export interface UsageMeter {
  usage: number;
  quota?: number;
}

export interface AvatarResponse {
  avatar_url: string;
  avatar_version: number;
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

export interface AccountUsageResponse {
  account_id: string;
  period_start: string;
  period_end: string;
  compute_unit_hours: UsageMeter;
  agent_builds: UsageMeter;
  active_deployments: UsageMeter;
  active_agents: UsageMeter;
}

// Export singleton instance
// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = typeof import.meta !== 'undefined' && import.meta.env
  ? (import.meta.env.VITE_API_URL || '')
  : '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
