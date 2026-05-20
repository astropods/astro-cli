import { type ReactNode } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Outlet, type InitialEntry } from 'react-router';
import { createRoutesStub } from 'react-router';
import { AuthContext, type AuthContextType } from '@/lib/auth-context';
import { ActiveAccountProvider } from '@/hooks/use-active-account';

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
  initialEntries?: InitialEntry[];
}

function createWrapper({ initialEntries = ['/'] }: WrapperOptions = {}) {
  const queryClient = createTestQueryClient();

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <AuthContext.Provider value={mockAuthContext}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
        </QueryClientProvider>
      </AuthContext.Provider>
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
  permissions: [],
  expiresAt: new Date(Date.now() + 86400000),
  isLoading: false,
  isAuthenticated: true,
  error: null,
  accounts: [{ id: 'acct-1', name: 'testuser', type: 'personal' }],
  needsOnboarding: false,
  refreshVersion: 0,
  login: () => {},
  logout: () => {},
  refresh: async () => {},
  refreshUserData: async () => {},
  checkAuth: async () => {},
  switchOrg: async () => {},
  hydrateAuth: () => {},
};

// Render a route using createRoutesStub (RR v7 test API).
//
// ActiveAccountProvider must live INSIDE the router stub because it calls
// useRevalidator() — that hook requires the data router context and throws
// if invoked outside it. We wrap the test's routes in a synthetic layout
// route that mounts AuthContext + ActiveAccountProvider around an <Outlet />,
// matching how Layout.tsx does it in the real app.
export function renderRoute(
  routes: Parameters<typeof createRoutesStub>[0],
  options?: { initialEntries?: InitialEntry[]; auth?: AuthContextType | null } & Omit<RenderOptions, 'wrapper'>,
) {
  const { initialEntries = ['/'], auth, ...renderOptions } = options ?? {};
  const queryClient = createTestQueryClient();

  const wrappedRoutes: Parameters<typeof createRoutesStub>[0] =
    auth === null
      ? routes
      : [
          {
            Component: () => (
              <AuthContext.Provider value={auth ?? mockAuthContext}>
                <ActiveAccountProvider>
                  <Outlet />
                </ActiveAccountProvider>
              </AuthContext.Provider>
            ),
            children: routes,
          },
        ];

  const Stub = createRoutesStub(wrappedRoutes);

  const tree = (
    <QueryClientProvider client={queryClient}>
      <Stub initialEntries={initialEntries} />
    </QueryClientProvider>
  );

  const result = render(tree, renderOptions);

  return { ...result, queryClient };
}
