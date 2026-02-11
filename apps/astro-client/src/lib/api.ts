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

export interface AuthResponse {
  user: User;
  session_id: string;
  organization_id?: string;
  role?: string;
  expires_at: string;
}

export interface ValidationError {
  field: string;
  message: string;
}

export interface ApiError {
  error: string;
  error_description?: string;
  code?: string;
  details?: string;
  validation_errors?: ValidationError[];
  missing_credentials?: string[];
}

class ApiClient {
  private baseUrl: string;
  private authUrl: string;

  constructor(baseUrl: string = '', authUrl: string = '') {
    this.baseUrl = baseUrl;
    this.authUrl = authUrl;
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
        ...options.headers,
      },
    });

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
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
      headers: { 'Content-Type': 'application/json' },
    });
    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw error;
    }
    return response.json();
  }

  async refreshSession(): Promise<AuthResponse> {
    const url = `${this.authUrl}/auth/refresh`;
    const response = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'request_failed',
        error_description: `Request failed with status ${response.status}`,
      }));
      throw error;
    }
    return response.json();
  }

  getLoginUrl(): string {
    return `${this.authUrl}/auth/login`;
  }

  getLogoutUrl(): string {
    // Pass current origin as redirect parameter for reliable post-logout redirect
    const redirectUrl = encodeURIComponent(window.location.origin);
    return `${this.authUrl}/auth/logout?redirect=${redirectUrl}`;
  }

  // Agent endpoints
  async listAgents(): Promise<AgentsListResponse> {
    return this.request<AgentsListResponse>('/api/v1/agents');
  }

  async getAgent(name: string): Promise<Agent> {
    return this.request<Agent>(`/api/v1/agents/${encodeURIComponent(name)}`);
  }

  async getAgentVersion(name: string, version: string): Promise<AgentVersion> {
    return this.request<AgentVersion>(
      `/api/v1/agents/${encodeURIComponent(name)}/${encodeURIComponent(version)}`
    );
  }

  async deployAgent(data: {
    name: string;
    version: string;
    user_credentials?: Record<string, string>;
  }): Promise<DeployResponse> {
    return this.request<DeployResponse>('/api/v1/deploy', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async undeployAgent(data: {
    name: string;
    version: string;
  }): Promise<UndeployResponse> {
    return this.request<UndeployResponse>('/api/v1/undeploy', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // Get required credentials for deploying an agent
  async getAgentConfig(name: string, version: string): Promise<AgentConfigResponse> {
    return this.request<AgentConfigResponse>(
      `/api/v1/agents/${encodeURIComponent(name)}/${encodeURIComponent(version)}/config`
    );
  }

  // List current deployments for the authenticated user
  async listDeployments(): Promise<DeploymentsListResponse> {
    return this.request<DeploymentsListResponse>('/api/v1/deployments');
  }

  // Fetch logs for a specific pod in a deployment
  async getDeploymentLogs(
    name: string,
    version: string,
    pod: string,
    container: string,
    tailLines: number = 200
  ): Promise<string> {
    const params = new URLSearchParams({
      pod,
      container,
      tailLines: String(tailLines),
    });
    const url = `${this.baseUrl}/api/v1/deployments/${encodeURIComponent(name)}/${encodeURIComponent(version)}/logs?${params}`;
    const response = await fetch(url, {
      credentials: 'include',
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({
        error: 'request_failed',
        details: `Request failed with status ${response.status}`,
      }));
      throw error;
    }
    return response.text();
  }
}

// Response types
export interface AgentSpec {
  meta?: { description?: string; tags?: string[] };
  integrations?: { tools?: { provider: string }[] };
  [key: string]: unknown;
}

export interface AgentVersion {
  version: string;
  spec: AgentSpec;
  published_at: string;
}

export interface Agent {
  name: string;
  registry: string;
  versions: AgentVersion[];
}

export interface AgentsListResponse {
  agents: Agent[];
  count: number;
}

export interface CredentialInfo {
  key: string;
  provider: string;
  category: string;
  description: string;
  optional: boolean;
}

export interface AgentConfigResponse {
  agent: string;
  version: string;
  credentials: CredentialInfo[];
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
  name: string;
  version: string;
  k8s_namespace: string;
  deployed_at: string;
  resources: ResourceStatus[];
  service_endpoints?: ServiceEndpoint[];
  errors?: DeploymentError[];
}

export interface UndeployResponse {
  status: string;
  name: string;
  version: string;
  k8s_namespace: string;
  undeployed_at: string;
  resources: ResourceStatus[];
  errors?: DeploymentError[];
}

export interface ServiceEndpointInfo {
  name: string;
  url: string;
}

export interface ContainerStatus {
  name: string;
  state: string;
  ready: boolean;
  restart_count: number;
  reason?: string;
  message?: string;
}

export interface PodDetail {
  name: string;
  phase: string;
  pod_ip?: string;
  age: string;
  containers: ContainerStatus[];
}

export interface AgentDeployment {
  name: string;
  version: string;
  status: string;
  replicas: number;
  ready: number;
  created_at: string;
  components: string[];
  service_endpoint?: ServiceEndpointInfo;
  external_url?: string;
  pods?: PodDetail[];
}

export interface DeploymentsListResponse {
  deployments: AgentDeployment[];
  count: number;
  namespace: string;
}

// Export singleton instance
// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = import.meta.env.VITE_API_URL || '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
