import { describe, it, expect, beforeAll, beforeEach, afterEach } from 'vitest';
import { screen, cleanup, within, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import { ActiveDetailView } from './ActiveDetailView';
import type { AgentDeployment, BlueprintsListResponse } from '@/lib/api';

beforeEach(cleanup);
afterEach(cleanup);
afterEach(() => server.resetHandlers());

// jsdom does not implement matchMedia
beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
});

const OLD_BUILD = 'aabbccdd11223344';
const NEW_BUILD = 'eeff001122334455';

const baseDeployment: AgentDeployment = {
  id: 'dep-1',
  name: 'my-agent',
  display_name: 'My Agent',
  build_id: OLD_BUILD,
  namespace: 'astro-test',
  status: 'Running',
  replicas: 1,
  ready: 1,
  created_at: '2026-01-01T00:00:00Z',
  components: ['deployment'],
};

function setupBlueprintsHandler(agents: BlueprintsListResponse['agents']) {
  server.use(
    http.get('/api/v1/agents/:account', () =>
      HttpResponse.json<BlueprintsListResponse>({ agents, count: agents.length }),
    ),
    // DeploymentsTab fetches deployment history
    http.get('/api/v1/agents/:account/:name/deployment/history', () =>
      HttpResponse.json({ deployments: [], count: 0 }),
    ),
  );
}

function renderView(deployment: AgentDeployment = baseDeployment) {
  return renderWithProviders(
    <ActiveDetailView deployment={deployment} account="testuser" isPersonal />,
    { initialEntries: ['/deployments/dep-1'] },
  );
}

/** Wait for the account blueprints query to reach success state. */
async function waitForBlueprintsLoaded(queryClient: ReturnType<typeof renderWithProviders>['queryClient']) {
  await waitFor(() => {
    const state = queryClient.getQueryState(['agents', 'account', 'testuser']);
    expect(state?.status).toBe('success');
  });
}

describe('new build available banner', () => {
  it('shows when a newer build has been published', async () => {
    setupBlueprintsHandler([{
      name: 'my-agent',
      account: 'testuser',
      registry: 'registry.example.com',
      versions: [
        { build_id: OLD_BUILD, published_at: '2026-04-01T00:00:00Z', spec: {} },
        { build_id: NEW_BUILD, published_at: '2026-04-22T00:00:00Z', spec: {} },
      ],
    }]);

    renderView();

    await screen.findByText('A new build number is available for this agent.');
  });

  it('shows the current and new build hashes inside the banner', async () => {
    setupBlueprintsHandler([{
      name: 'my-agent',
      account: 'testuser',
      registry: 'registry.example.com',
      versions: [
        { build_id: OLD_BUILD, published_at: '2026-04-01T00:00:00Z', spec: {} },
        { build_id: NEW_BUILD, published_at: '2026-04-22T00:00:00Z', spec: {} },
      ],
    }]);

    renderView();

    const titleEl = await screen.findByText('A new build number is available for this agent.');
    // Walk up to the ActionPanel container (rounded outer div)
    const panel = titleEl.closest('[class*="rounded-\\[6px\\]"]') ?? titleEl.parentElement!.parentElement!.parentElement!;
    expect(within(panel as HTMLElement).getByText(OLD_BUILD.slice(0, 8))).toBeInTheDocument();
    expect(within(panel as HTMLElement).getByText(NEW_BUILD.slice(0, 8))).toBeInTheDocument();
  });

  it('does not show when already on the latest build', async () => {
    setupBlueprintsHandler([{
      name: 'my-agent',
      account: 'testuser',
      registry: 'registry.example.com',
      versions: [
        { build_id: OLD_BUILD, published_at: '2026-04-01T00:00:00Z', spec: {} },
      ],
    }]);

    const { queryClient } = renderView();
    await waitForBlueprintsLoaded(queryClient);

    expect(screen.queryByText('A new build number is available for this agent.')).not.toBeInTheDocument();
  });

  it('does not show when there is no matching blueprint', async () => {
    setupBlueprintsHandler([{
      name: 'different-agent',
      account: 'testuser',
      registry: 'registry.example.com',
      versions: [
        { build_id: NEW_BUILD, published_at: '2026-04-22T00:00:00Z', spec: {} },
      ],
    }]);

    const { queryClient } = renderView();
    await waitForBlueprintsLoaded(queryClient);

    expect(screen.queryByText('A new build number is available for this agent.')).not.toBeInTheDocument();
  });

  it('does not show when the deployed build is already the newest by date', async () => {
    setupBlueprintsHandler([{
      name: 'my-agent',
      account: 'testuser',
      registry: 'registry.example.com',
      versions: [
        { build_id: NEW_BUILD, published_at: '2026-04-22T00:00:00Z', spec: {} },
        { build_id: OLD_BUILD, published_at: '2026-04-01T00:00:00Z', spec: {} },
      ],
    }]);

    const { queryClient } = renderView({ ...baseDeployment, build_id: NEW_BUILD });
    await waitForBlueprintsLoaded(queryClient);

    expect(screen.queryByText('A new build number is available for this agent.')).not.toBeInTheDocument();
  });
});
