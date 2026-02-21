import { type ReactNode } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { createRoutesStub } from 'react-router';
import { AuthContext, type AuthContextType } from '@/lib/auth-context';

// Fresh QueryClient per test — no retries, no gc delay, instant stale
function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: Infinity,
        staleTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

interface WrapperOptions {
  initialEntries?: string[];
}

function createWrapper({ initialEntries = ['/'] }: WrapperOptions = {}) {
  const queryClient = createTestQueryClient();

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  }

  return { Wrapper, queryClient };
}

export function renderWithProviders(
  ui: ReactNode,
  options?: WrapperOptions & Omit<RenderOptions, 'wrapper'>,
) {
  const { initialEntries, ...renderOptions } = options ?? {};
  const { Wrapper, queryClient } = createWrapper({ initialEntries });
  const result = render(ui, { wrapper: Wrapper, ...renderOptions });
  return { ...result, queryClient };
}

// For testing hooks directly (pass to renderHook's wrapper option)
export function createHookWrapper(options?: WrapperOptions) {
  const { Wrapper, queryClient } = createWrapper(options);
  return { wrapper: Wrapper, queryClient };
}

// Default mock auth context for authenticated tests
export const mockAuthContext: AuthContextType = {
  user: {
    id: 'user-1',
    email: 'test@example.com',
    first_name: 'Test',
    last_name: 'User',
    email_verified: true,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
  },
  sessionId: 'session-1',
  organizationId: 'org-1',
  role: 'admin',
  expiresAt: new Date(Date.now() + 86400000),
  isLoading: false,
  isAuthenticated: true,
  error: null,
  accounts: [{ id: 'acct-1', name: 'testuser', type: 'personal' }],
  needsOnboarding: false,
  login: () => {},
  logout: () => {},
  refresh: async () => {},
  checkAuth: async () => {},
};

// Render a route using createRoutesStub (RR v7 test API)
export function renderRoute(
  routes: Parameters<typeof createRoutesStub>[0],
  options?: { initialEntries?: string[]; auth?: AuthContextType | null } & Omit<RenderOptions, 'wrapper'>,
) {
  const { initialEntries = ['/'], auth, ...renderOptions } = options ?? {};
  const queryClient = createTestQueryClient();
  const Stub = createRoutesStub(routes);

  let tree = (
    <QueryClientProvider client={queryClient}>
      <Stub initialEntries={initialEntries} />
    </QueryClientProvider>
  );

  if (auth !== null) {
    tree = (
      <AuthContext.Provider value={auth ?? mockAuthContext}>
        {tree}
      </AuthContext.Provider>
    );
  }

  const result = render(tree, renderOptions);

  return { ...result, queryClient };
}
