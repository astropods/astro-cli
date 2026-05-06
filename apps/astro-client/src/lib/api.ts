// API client for communicating with the astro-server backend

import type { LogEntry } from "./log-utils";

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

export interface BlueprintSummary {
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
  avatar_colors?: AvatarColors;
}

export interface Account {
  id: string;
  name: string;
  type: string;
  display_name?: string;
  role?: string;
  organization_id?: string; // WorkOS org ID, present on organization accounts
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
  type: string;
}

export interface AccountMember {
  account_id: string;
  user_id: string;
  role: string;
  status: string;
  username: string;
  display_name: string;
  created_at: string;
}

export interface AccountMembersResponse {
  members: AccountMember[];
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
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
    }

    // Handle empty responses (204 No Content, etc.)
    const text = await response.text();
    if (!text) {
      return {} as T;
    }

    return JSON.parse(text);
  }

  // Auth endpoints - use authUrl for direct backend communication
  private async authRequest<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.authUrl}${endpoint}`;
    const response = await fetch(url, {
      ...options,
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...this.defaultHeaders, ...options.headers },
    });
    if (!response.ok) {
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
    }
    return response.json();
  }

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
    data: { bio?: string; location?: string; email?: string; local_timezone?: string; pronouns?: string; website?: string; social_links?: string[] },
  ): Promise<{ message: string }> {
    return this.request<{ message: string }>(
      `/api/v1/accounts/${encodeURIComponent(account)}`,
      { method: 'PATCH', body: JSON.stringify(data) },
    );
  }

  async getAccountOrgs(account: string): Promise<AccountOrgsResponse> {
    return this.request<AccountOrgsResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/orgs`,
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


  async getAccount(name: string): Promise<AccountPublic> {
    return this.request<AccountPublic>(
      `/api/v1/accounts/${encodeURIComponent(name)}`
    );
  }

  async getAccountMembers(account: string, opts?: { includePending?: boolean }): Promise<AccountMembersResponse> {
    const params = opts?.includePending ? '?include_pending=true' : '';
    return this.request<AccountMembersResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/members${params}`
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

  // Blueprint endpoints
  async listBlueprints(): Promise<BlueprintsListResponse> {
    return this.request<BlueprintsListResponse>('/api/v1/agents');
  }

  async listAccountBlueprints(account: string): Promise<BlueprintsListResponse> {
    return this.request<BlueprintsListResponse>(
      `/api/v1/agents/${encodeURIComponent(account)}`
    );
  }

  async getBlueprint(account: string, name: string): Promise<Blueprint> {
    return this.request<Blueprint>(
      `/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}`
    );
  }

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



  // List current deployments for an account
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
    options?: { level?: string; direction?: string; tailLines?: number },
  ): Promise<LogEntry[]> {
    const params = new URLSearchParams({ workload: workloadName, container });
    if (since) params.set('since', since);
    if (timezone && timezone !== 'UTC') params.set('timezone', timezone);
    if (options?.level) params.set('level', options.level);
    if (options?.direction) params.set('direction', options.direction);
    if (options?.tailLines !== undefined) params.set('tailLines', String(options.tailLines));
    const url = `${this.baseUrl}/api/v1/deployments/${encodeURIComponent(deploymentId)}/logs?${params}`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: { ...this.defaultHeaders },
    });
    if (!response.ok) {
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        details: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
    }
    return response.json();
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

  async getAccountObservabilitySummary(
    account: string,
    params?: Record<string, string>,
  ): Promise<AccountObservabilitySummaryResponse> {
    const qs = params ? `?${new URLSearchParams(params)}` : '';
    return this.request<AccountObservabilitySummaryResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/observability/summary${qs}`
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

  // Audit log
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

  // Account variables / secrets vault
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
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
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

  // Blueprint avatar endpoints
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

  // Deployment avatar endpoints
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

  async createBlueprint(account: string, body: { name: string; visibility?: string }): Promise<{ account: string; name: string }> {
    return this.request(
      `/api/v1/agents/${encodeURIComponent(account)}`,
      { method: 'POST', body: JSON.stringify(body) }
    );
  }

  // Knowledge Store endpoints
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
    const url = `${this.baseUrl}/api/v1/accounts/${encodeURIComponent(account)}/knowledge/${encodeURIComponent(name)}/logs${qs ? `?${qs}` : ''}`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: { ...this.defaultHeaders },
    });
    if (!response.ok) {
      const body: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        details: `Request failed with status ${response.status}`,
      }));
      throw new ApiRequestError(body, response.status);
    }
    return response.json();
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

  // GitHub connection
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

  async gitHubConnectAccount(account: string, redirectTo: string): Promise<GitHubConnectResponse> {
    return this.request<GitHubConnectResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/connect`,
      { method: 'POST', body: JSON.stringify({ redirect_to: redirectTo }) }
    );
  }

  // Account-level Slack identity link via WorkOS Pipes. The mapping that
  // backs per-user grants on slack lives in slack_identity_mappings and
  // is populated by the callback handler.

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
}

export interface BlueprintMetrics {
  lifetime_messages: number;
  deploy_count?: number;
}

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
}

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
}

export interface VariableField {
  label?: string;
  description?: string;
  placeholder?: string;
  datatype?: string;
  optional?: boolean;
}

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

// --- Interactive POST template types ---

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
  editable: string[];
  interfaces: TemplateInterfaces;
  schedules: Record<string, string>;
  bindings?: {
    knowledge?: Record<string, KnowledgeBindingInfo>;
  };
  validation: TemplateValidation;
  signature?: string;
}

export interface TemplateInterfaces {
  adapters: string[];
  auth?: { web?: { type?: string } };
}


export interface TemplateValidation {
  valid: boolean;
  errors: ValidationError[];
}

export interface ValidationError {
  field: string;
  message: string;
}

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

export interface ServiceEndpointInfo {
  name: string;
  url: string;
  type?: string;
}

export interface EnvVar {
  name: string;
  value?: string;
  from?: string;
  // Authoritative provenance from deployment_build_env when present:
  // 'user_var' | 'platform_meta' | 'service_url' | 'knowledge_cred' |
  // 'auth_token' | 'adapter_config' | 'derived'. Empty for legacy
  // deployments without rows yet — clients fall back to inferring from
  // `from`.
  source?: string;
  // Authoritative secret flag from deployment_build_env. When true the
  // value is redacted in the UI; replaces the client-side
  // isSensitiveEnvVar name heuristic.
  is_secret?: boolean;
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
  phase?: string;
  pod_name?: string;
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
  replicas: number;
  ready: number;
  created_at: string;
  updated_at?: string;
  updated_by?: string;
  components: string[];
  manual_ingestions?: string[];
  external_urls?: ServiceEndpointInfo[];
  messaging_available?: boolean;
  workloads?: WorkloadDetail[];
  jobs?: JobDetail[];
}

export interface DeploymentsListResponse {
  deployments: AgentDeployment[];
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
  total_cost?: number;
  input: string;
  output: string;
  timestamp: string;
}

export interface AccountObservabilitySummaryResponse {
  total_traces: number;
  input_tokens: number;
  output_tokens: number;
  time_range: { start: string; end: string };
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
  avatar_colors?: AvatarColors;
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

// Audit Log types
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

// Knowledge Store types
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

export interface BoundAgent {
  deployment_id: string;
  agent_name: string;
  display_name?: string;
  knowledge_name: string;
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

export interface KnowledgeCredentials {
  [key: string]: string;
}

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

export interface GitHubLinkInput {
  repo_full_name: string;
  branch: string;
}

// Export singleton instance
// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = typeof import.meta !== 'undefined' && import.meta.env
  ? (import.meta.env.VITE_API_URL || '')
  : '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
