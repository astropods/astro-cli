// API client for communicating with the astro-server backend

// Use relative URLs - Vite proxy handles routing to the backend
const API_BASE_URL = '';

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

export interface ApiError {
  error: string;
  error_description?: string;
  code?: string;
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
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

  // Auth endpoints
  async getCurrentUser(): Promise<AuthResponse> {
    return this.request<AuthResponse>('/auth/me');
  }

  async refreshSession(): Promise<AuthResponse> {
    return this.request<AuthResponse>('/auth/refresh', { method: 'POST' });
  }

  getLoginUrl(): string {
    return `${this.baseUrl}/auth/login`;
  }

  getLogoutUrl(): string {
    return `${this.baseUrl}/auth/logout`;
  }

  // Agent endpoints
  async listAgents(): Promise<unknown[]> {
    return this.request<unknown[]>('/api/v1/agents');
  }

  async getAgent(name: string): Promise<unknown> {
    return this.request<unknown>(`/api/v1/agents/${encodeURIComponent(name)}`);
  }

  async getAgentVersion(name: string, version: string): Promise<unknown> {
    return this.request<unknown>(
      `/api/v1/agents/${encodeURIComponent(name)}/${encodeURIComponent(version)}`
    );
  }

  async registerAgent(data: unknown): Promise<unknown> {
    return this.request<unknown>('/api/v1/agents/register', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deployAgent(data: {
    name: string;
    version: string;
    k8s_namespace: string;
    user_credentials?: Record<string, string>;
  }): Promise<unknown> {
    return this.request<unknown>('/api/v1/deploy', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async undeployAgent(data: {
    name: string;
    version: string;
    k8s_namespace: string;
  }): Promise<unknown> {
    return this.request<unknown>('/api/v1/undeploy', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }
}

// Export singleton instance
export const api = new ApiClient();

// Export class for testing or custom instances
export { ApiClient };
