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
  owner?: AccountOwner;
  created_at: string;
  updated_at: string;
}

export interface Account {
  id: string;
  name: string;
  type: string;
  role?: string;
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

  async updateProfile(data: { first_name: string; last_name: string }): Promise<{ user: User }> {
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
  async getDeploymentHistory(account: string, name: string): Promise<DeploymentHistoryResponse> {
    return this.request<DeploymentHistoryResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/deployment/history`
    );
  }

  // Fetch logs for a specific pod in a deployment
  async getDeploymentLogs(
    namespace: string,
    pod: string,
    container: string,
    account: string,
    tailLines: number = 200
  ): Promise<string> {
    const params = new URLSearchParams({
      pod,
      container,
      account,
      tailLines: String(tailLines),
    });
    const url = `${this.baseUrl}/api/v1/deployments/${encodeURIComponent(namespace)}/logs?${params}`;
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
    namespace: string,
    cmname: string,
    account: string,
  ): Promise<ConfigMapResponse> {
    const params = new URLSearchParams({ account });
    return this.request<ConfigMapResponse>(
      `/api/v1/deployments/${encodeURIComponent(namespace)}/configmap/${encodeURIComponent(cmname)}?${params}`
    );
  }

  // Fetch Secret key names (no values) for a deployment
  async getSecretKeys(
    namespace: string,
    secretName: string,
    account: string,
  ): Promise<SecretKeysResponse> {
    const params = new URLSearchParams({ account });
    return this.request<SecretKeysResponse>(
      `/api/v1/deployments/${encodeURIComponent(namespace)}/secret/${encodeURIComponent(secretName)}/keys?${params}`
    );
  }

  // Restart (delete) a pod so Kubernetes recreates it
  async restartPod(data: {
    namespace: string;
    pod: string;
    account: string;
  }): Promise<{ status: string; pod: string }> {
    const params = new URLSearchParams({ account: data.account });
    return this.request(`/api/v1/deployments/${encodeURIComponent(data.namespace)}/pods/${encodeURIComponent(data.pod)}/restart?${params}`, {
      method: 'POST',
    });
  }

  // Observability endpoints
  async getObservabilityMetrics(
    account: string,
    name: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityMetricsResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilityMetricsResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/observability/metrics${qs}`
    );
  }

  async getObservabilitySummary(
    account: string,
    name: string,
    params?: Record<string, string>,
  ): Promise<ObservabilitySummaryResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilitySummaryResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/observability/summary${qs}`
    );
  }

  async triggerIngestion(data: {
    namespace: string;
    ingestion: string;
    account: string;
  }): Promise<TriggerIngestionResponse> {
    const params = new URLSearchParams({ account: data.account });
    return this.request<TriggerIngestionResponse>(
      `/api/v1/deployments/${encodeURIComponent(data.namespace)}/ingestion/${encodeURIComponent(data.ingestion)}/trigger?${params}`,
      { method: 'POST' }
    );
  }

  async getObservabilityTraces(
    account: string,
    name: string,
    params?: Record<string, string>,
  ): Promise<ObservabilityTracesResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<ObservabilityTracesResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/observability/traces${qs}`
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
  meta?: { description?: string; tags?: string[] };
  integrations?: Record<string, { provider: string; type?: string }>;
  [key: string]: unknown;
}

export interface AgentVersion {
  build_id: string;
  version?: string;
  spec: AgentSpec;
  readme?: string;
  published_at: string;
  validation_warnings?: ValidationError[];
}

export interface Agent {
  name: string;
  account: string;
  registry: string;
  visibility?: string;
  versions: AgentVersion[];
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

export interface DeploymentTemplate {
  spec: 'deployment-template/v1';
  source: { account: string; name: string; build: string; registry: string };
  target: { runtime: string; account?: string; display_name?: string; deployment_id?: string };
  agent: Record<string, unknown>;
  models?: Record<string, unknown>;
  knowledge?: Record<string, unknown>;
  tools?: Record<string, unknown>;
  ingestion?: Record<string, unknown>;
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

export interface PodDetail {
  name: string;
  phase: string;
  pod_ip?: string;
  age: string;
  containers: ContainerStatus[];
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
  pods?: PodDetail[];
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

// Export singleton instance
// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = typeof import.meta !== 'undefined' && import.meta.env
  ? (import.meta.env.VITE_API_URL || '')
  : '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
