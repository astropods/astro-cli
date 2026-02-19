import { type ReactNode } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { createRoutesStub } from 'react-router';

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

// Render a route using createRoutesStub (RR v7 test API)
export function renderRoute(
  routes: Parameters<typeof createRoutesStub>[0],
  options?: { initialEntries?: string[] } & Omit<RenderOptions, 'wrapper'>,
) {
  const { initialEntries = ['/'], ...renderOptions } = options ?? {};
  const queryClient = createTestQueryClient();
  const Stub = createRoutesStub(routes);

  const result = render(
    <QueryClientProvider client={queryClient}>
      <Stub initialEntries={initialEntries} />
    </QueryClientProvider>,
    renderOptions,
  );

  return { ...result, queryClient };
}
