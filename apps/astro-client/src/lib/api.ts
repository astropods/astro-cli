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

export interface ApiError {
  error: string;
  error_description?: string;
  code?: string;
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
// Auth endpoints go directly to the backend (for cookies), API endpoints use proxy
const authUrl = import.meta.env.VITE_API_URL || '';
export const api = new ApiClient('', authUrl);

// Export class for testing or custom instances
export { ApiClient };
