// API client for communicating with the astro-server backend

import type { LogEntry } from "./log-utils";
import {
  buildBlueprintListQuery,
  BLUEPRINT_LIST_MAX_LIMIT,
  type BlueprintListParams,
} from "./blueprint-list-params";

function buildQS(params?: Record<string, string>): string {
  return params ? `?${new URLSearchParams(params)}` : '';
}

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
    data: { bio?: string; location?: string; email?: string; local_timezone?: string; pronouns?: string; website?: string; social_links?: string[]; blueprint_order?: string[] },
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

  // Observability endpoints (deployment-scoped, backed by Langfuse)
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

  async getAccountBlueprintsSummary(
    account: string,
    params?: Record<string, string>,
  ): Promise<AccountBlueprintsSummaryResponse> {
    return this.request<AccountBlueprintsSummaryResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/observability/blueprints-summary${buildQS(params)}`
    );
  }

  async getAccountUsersSummary(
    account: string,
    params?: Record<string, string>,
  ): Promise<AccountUsersSummaryResponse> {
    return this.request<AccountUsersSummaryResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/observability/users-summary${buildQS(params)}`
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

  async gitHubConnectAccount(account: string, redirectTo: string, force?: boolean): Promise<GitHubConnectResponse> {
    return this.request<GitHubConnectResponse>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/connect`,
      { method: 'POST', body: JSON.stringify({ redirect_to: redirectTo, force: force ?? false }) }
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

  async gitHubListAccountOrgs(account: string): Promise<{ orgs: Array<{ login: string; avatar_url: string }> }> {
    return this.request<{ orgs: Array<{ login: string; avatar_url: string }> }>(
      `/api/v1/accounts/${encodeURIComponent(account)}/github/orgs`
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
}

export type DeploymentSpec = Omit<DeploymentTemplate, 'spec'> & {
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
  ready?: boolean;
  message?: string;
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
  // "Deployment" | "StatefulSet" | "Job" | "CronJob"
  kind: string;
  component: string;
  age: string;
  phase?: string;
  pod_name?: string;
  containers?: ContainerStatus[];
  urls?: ServiceEndpointInfo[];
  // Job/CronJob-only fields. Empty for Deployment/StatefulSet — their health
  // is read from containers[].ready instead.
  status?: string;
  schedule?: string;
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

// Observability types
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
    users: Array<{ user_id: string; cost_usd: number }>;
  }>;
}

export interface AccountUsersSummaryResponse {
  users: Array<{
    user_id: string;
    requests: number;
    cost_usd: number;
    /** Combined input + output tokens. Traces view only exposes the sum. */
    tokens: number;
    /** RFC3339 timestamp of the user's most recent hour-bucket with activity.
     *  Omitted when the user had no activity in the period. */
    last_seen?: string;
    /** Each entry carries the publishing account so the client can build
     *  the correct avatar URL — public-blueprint deploys resolve under the
     *  original publisher's account, not the deploying org. */
    agents_used: Array<{ name: string; account: string }>;
  }>;
  period: { start: string; end: string; days: number };
}

export interface AccountBlueprintsSummaryResponse {
  blueprints: Array<{
    agent_name: string;
    requests: number;
    cost_usd: number;
    cost_per_request: number;
    /** @deprecated prefer total_tokens. */
    input_tokens: number;
    /** @deprecated prefer total_tokens. */
    output_tokens: number;
    /** Combined token count for the blueprint. New source of truth. */
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
  }>;
  period: { start: string; end: string; days: number };
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

// --- Network (Beyla eBPF) ---

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
  meters: Record<string, UsageMeter>;
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
