import { describe, it, expect, afterEach, vi } from 'vitest';
import { act, screen, waitFor, cleanup, fireEvent, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AgentDashboard from './AgentDashboard';
import type { AgentDeploymentSummary, UserResourcePage } from '@/lib/api';

function userDeployments(response: {
  deployments: Array<AgentDeploymentSummary & Record<string, unknown>>;
  count: number;
}, page: UserResourcePage = { limit: 50 }) {
  return HttpResponse.json({
    ...response,
    page,
    scope: { accounts: ['testuser'], all: true },
  });
}

vi.mock('@/components/ui/LiveRevealOverlay', () => ({
  LiveRevealOverlay: ({
    deployment,
    accessReady,
    accessDelayed,
    accessStalled,
    onDismiss,
    onViewDeployment,
    onRetryAccess,
  }: {
    deployment: { display_name?: string; name: string };
    accessReady?: boolean;
    accessDelayed?: boolean;
    accessStalled?: boolean;
    onDismiss: () => void;
    onViewDeployment: () => void;
    onRetryAccess?: () => void;
  }) => (
    <div data-testid="live-reveal-overlay">
      <span>{deployment.display_name ?? deployment.name}</span>
      <span>{accessReady ? 'Access ready' : accessStalled ? 'Access setup stalled' : accessDelayed ? 'Still setting up access' : 'Setting up access'}</span>
      <button onClick={onDismiss}>Dismiss</button>
      {accessStalled ? (
        <button onClick={onRetryAccess}>Retry access setup</button>
      ) : (
        <button disabled={!accessReady} onClick={onViewDeployment}>View deployment</button>
      )}
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.useRealTimers();
  vi.restoreAllMocks();
});


function renderDashboard(path = '/agents', auth = mockAuthContext) {
  return renderRoute(
    [
      {
        path: '/agents',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: AgentDashboard as any,
        loader: () => ({ scope: null, data: null }),
      },
    ],
    { initialEntries: [path], auth },
  );
}

describe('AgentDashboard page', () => {
  it('renders the Agents heading', async () => {
    renderDashboard();
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: 'Agents for' })).toBeInTheDocument();
    });
  });

  it('does not render a status filter', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({
          deployments: [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboard();
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search agents...')).toBeInTheDocument();
    });
    expect(screen.queryByText('All statuses')).not.toBeInTheDocument();
  });

  it('shows deployed agent cards with display_name', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({
          deployments: [
            {
              id: 'dep-1',
              name: 'code-reviewer',
              display_name: 'My Code Reviewer',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-01T00:00:00Z',
              components: ['deployment'],
            },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('My Code Reviewer')).toBeInTheDocument();
    });
  });


  it('offers every membership in the org switcher when the account has no agents', async () => {
    const user = userEvent.setup();
    const multiAccountAuth = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-1', name: 'testuser', display_name: 'Test User', type: 'personal' as const },
        { id: 'acct-2', name: 'acme', display_name: 'Acme', type: 'organization' as const },
      ],
    };
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );

    renderDashboard('/agents', multiAccountAuth);

    await waitFor(() => {
      expect(screen.getByText('No agents deployed yet')).toBeInTheDocument();
    });
    expect(screen.queryByText('No agents match your filters.')).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText('Search agents...')).not.toBeInTheDocument();

    const orgSwitcher = screen.getByRole('combobox', { name: 'Scope by account' });
    expect(orgSwitcher).toHaveTextContent('Test User');
    await user.click(orgSwitcher);
    expect(screen.getByRole('option', { name: /Test User/ })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /Acme/ })).toBeInTheDocument();
  });

  it('keeps the toolbar and clears back to the account when a search yields no agents', async () => {
    const user = userEvent.setup();
    server.use(
      http.get('/api/v1/me/deployments', ({ request }) => {
        const q = new URL(request.url).searchParams.get('q');
        return userDeployments({
          deployments: q ? [] : [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
          ],
          count: q ? 0 : 1,
        });
      }),
    );

    renderDashboard();
    await waitFor(() => expect(screen.getByText('Code Reviewer')).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), { target: { value: 'zzz-no-match' } });
    await waitFor(() => {
      expect(screen.getByText('No agents match your filters.')).toBeInTheDocument();
    });
    expect(screen.getByPlaceholderText('Search agents...')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    await waitFor(() => {
      expect(screen.getByText('Code Reviewer')).toBeInTheDocument();
    });
  });

  it('adopts an account deep link as the org scope', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );
    const multiAccountAuth = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-1', name: 'testuser', display_name: 'Test User', type: 'personal' as const },
        { id: 'acct-2', name: 'acme', display_name: 'Acme', type: 'organization' as const },
      ],
    };

    renderDashboard('/agents?account=acme', multiAccountAuth);

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Scope by account' })).toHaveTextContent('Acme');
    });
  });


  it('filters agents by search text', async () => {
    server.use(
      http.get('/api/v1/me/deployments', ({ request }) => {
        const q = new URL(request.url).searchParams.get('q');
        const code = { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running' as const, replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] };
        const data = { id: 'dep-2', name: 'data-analyst', display_name: 'Data Analyst', build_id: 'b2', namespace: 'ns-2', status: 'Running' as const, replicas: 1, ready: 1, created_at: '2025-04-02T00:00:00Z', components: [] };
        return userDeployments({ deployments: q === 'code' ? [code] : [code, data], count: q === 'code' ? 1 : 2 });
      }),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Code Reviewer')).toBeInTheDocument();
      expect(screen.getByText('Data Analyst')).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), { target: { value: 'code' } });

    await waitFor(() => {
      expect(screen.getByText('Code Reviewer')).toBeInTheDocument();
      expect(screen.queryByText('Data Analyst')).not.toBeInTheDocument();
    });
  });

  it('shows "no agents match" message when filter yields no results', async () => {
    server.use(
      http.get('/api/v1/me/deployments', ({ request }) => {
        const q = new URL(request.url).searchParams.get('q');
        return userDeployments({
          deployments: q ? [] : [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
          ],
          count: q ? 0 : 1,
        });
      }),
    );

    renderDashboard();
    await waitFor(() => expect(screen.getByText('Code Reviewer')).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), { target: { value: 'zzz-no-match' } });

    await waitFor(() => {
      expect(screen.getByText('No agents match your filters.')).toBeInTheDocument();
    });
    expect(screen.getByPlaceholderText('Search agents...')).toHaveValue('zzz-no-match');
  });

  it('searches all deployment pages on the server and resets numbered pagination', async () => {
    const user = userEvent.setup();
    const searches: string[] = [];
    server.use(
      http.get('/api/v1/me/deployments', ({ request }) => {
        const url = new URL(request.url);
        searches.push(url.search);
        if (url.searchParams.get('q') === 'data') {
          return userDeployments({
            deployments: [
              { id: 'dep-2', name: 'data-analyst', display_name: 'Data Analyst', build_id: 'b2', namespace: 'ns-2', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-02T00:00:00Z', components: [] },
            ],
            count: 1,
          });
        }
        return userDeployments({
          deployments: [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
          ],
          count: 1,
        }, { limit: 50, next_cursor: 'page-2' });
      }),
    );

    renderDashboard();
    await waitFor(() => expect(screen.getByText('Code Reviewer')).toBeInTheDocument());
    await user.type(screen.getByPlaceholderText('Search agents...'), 'data');

    await waitFor(() => expect(screen.getByText('Data Analyst')).toBeInTheDocument());
    expect(screen.queryByText('Code Reviewer')).not.toBeInTheDocument();
    expect(searches.some((search) => new URLSearchParams(search).get('q') === 'data')).toBe(true);
    expect(screen.queryByRole('button', { name: 'Page 2' })).not.toBeInTheDocument();
  });
});

describe('reveal overlay after deploy', () => {
   
  function renderDashboardWithReveal(
    revealState: Record<string, string | null>,
    options: { path?: string; auth?: typeof mockAuthContext } = {},
  ) {
    const url = new URL(options.path ?? '/agents', 'http://astro.local');
    return renderRoute(
      [
        {
          path: '/agents',
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: AgentDashboard as any,
          loader: () => ({ scope: null, data: null }),
        },
        {
          // `deploymentPath(account, id)` with no tab now resolves directly
          // to the `/deployments` segment (previously bare, routed through
          // `AgentDetailRedirect`). Match that target here.
          path: '/:account/agents/:deploymentId/deployments',
          Component: () => <div data-testid="deployment-detail">Deployment Detail</div>,
        },
      ],
      {
        initialEntries: [{ pathname: url.pathname, search: url.search, state: revealState }] as unknown as string[],
        auth: options.auth,
      },
    );
  }

  it('shows reveal overlay when location state has revealDeploymentId', async () => {
    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(screen.getByTestId('live-reveal-overlay')).toBeInTheDocument();
    });
  });

  it('does not show reveal overlay without reveal state', async () => {
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: 'Agents for' })).toBeInTheDocument();
    });
    expect(screen.queryByTestId('live-reveal-overlay')).not.toBeInTheDocument();
  });

  it('uses display name from state as optimistic fallback before deployments load', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent Display',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(within(screen.getByTestId('live-reveal-overlay')).getByText('My Agent Display')).toBeInTheDocument();
    });
  });

  it('keeps the create receipt visible and blocks navigation while access is provisioning', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAccount: 'testuser',
      revealAvatarUrl: null,
    });

    const overlay = await screen.findByTestId('live-reveal-overlay');
    expect(within(overlay).getByText('Setting up access')).toBeInTheDocument();
    expect(within(overlay).getByRole('button', { name: 'View deployment' })).toBeDisabled();

    const pendingCard = document.querySelector('[data-deployment-id="dep-new"]');
    expect(pendingCard).not.toBeNull();
    expect(within(pendingCard as HTMLElement).getByText('Setting up access')).toBeInTheDocument();
    expect(within(pendingCard as HTMLElement).getByRole('status', { name: 'Deployment access is being configured' })).toHaveTextContent('Updates automatically');
  });

  it('hides the synthetic receipt when it does not match the active search', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAccount: 'testuser',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(document.querySelector('[data-deployment-id="dep-new"]')).not.toBeNull();
    });

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), {
      target: { value: 'different-agent' },
    });

    await waitFor(() => {
      expect(document.querySelector('[data-deployment-id="dep-new"]')).toBeNull();
    });
  });

  it('hides the synthetic receipt outside the selected account', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );
    const multiAccountAuth = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-1', name: 'testuser', display_name: 'Test User', type: 'personal' as const },
        { id: 'acct-2', name: 'acme', display_name: 'Acme', type: 'organization' as const },
      ],
    };

    renderDashboardWithReveal(
      {
        revealDeploymentId: 'dep-new',
        revealAgentName: 'my-agent',
        revealDisplayName: 'My Agent',
        revealAccount: 'testuser',
        revealAvatarUrl: null,
      },
      { path: '/agents?account=acme', auth: multiAccountAuth },
    );

    await screen.findByTestId('live-reveal-overlay');
    await waitFor(() => {
      expect(document.querySelector('[data-deployment-id="dep-new"]')).toBeNull();
    });
  });

  it('stops automatic access setup after two minutes and offers a retry', async () => {
    const user = userEvent.setup();
    const setTimeoutSpy = vi.spyOn(window, 'setTimeout');
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({ deployments: [], count: 0 }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAccount: 'testuser',
      revealAvatarUrl: null,
    });

    const overlay = await screen.findByTestId('live-reveal-overlay');
    const stalledTimer = setTimeoutSpy.mock.calls.find(([, delay]) => delay === 120_000)?.[0];
    expect(stalledTimer).toBeTypeOf('function');

    act(() => (stalledTimer as () => void)());

    expect(within(overlay).getByText('Access setup stalled')).toBeInTheDocument();
    const retry = within(overlay).getByRole('button', { name: 'Retry access setup' });
    expect(retry).toBeEnabled();

    await user.click(retry);
    await waitFor(() => {
      expect(within(overlay).getByText('Setting up access')).toBeInTheDocument();
    });
  });

  it('keeps navigation blocked when the resource is visible but creator access has not converged', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({
          deployments: [
            {
              id: 'dep-new',
              name: 'my-agent',
              display_name: 'My Agent',
              account_name: 'testuser',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              access_ready: false,
              created_at: '2025-04-01T00:00:00Z',
            },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAccount: 'testuser',
      revealAvatarUrl: null,
    });

    const overlay = await screen.findByTestId('live-reveal-overlay');
    expect(within(overlay).getByText('Setting up access')).toBeInTheDocument();
    expect(within(overlay).getByRole('button', { name: 'View deployment' })).toBeDisabled();
    expect(document.querySelectorAll('[data-deployment-id="dep-new"]')).toHaveLength(1);
  });

  it('keeps location state display_name even after deployment query loads', async () => {
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({
          deployments: [
            {
              id: 'dep-new',
              name: 'my-agent',
              display_name: 'Real Display Name',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-01T00:00:00Z',
              components: [],
            },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'State Display Name',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      const overlay = screen.getByTestId('live-reveal-overlay');
      expect(within(overlay).getByText('State Display Name')).toBeInTheDocument();
    });
  });

  it('hides the overlay when dismissed', async () => {
    const user = userEvent.setup();

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(screen.getByTestId('live-reveal-overlay')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /dismiss/i }));

    expect(screen.queryByTestId('live-reveal-overlay')).not.toBeInTheDocument();
  });

  it('navigates to deployment detail when view deployment is clicked', async () => {
    const user = userEvent.setup();
    server.use(
      http.get('/api/v1/me/deployments', () =>
        userDeployments({
          deployments: [
            {
              id: 'dep-new',
              name: 'my-agent',
              display_name: 'My Agent',
              account_name: 'testuser',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              access_ready: true,
              created_at: '2025-04-01T00:00:00Z',
            },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAvatarUrl: null,
    });

    const overlay = await screen.findByTestId('live-reveal-overlay');
    await waitFor(() => {
      expect(within(overlay).getByRole('button', { name: /view deployment/i })).toBeEnabled();
    });

    await user.click(within(overlay).getByRole('button', { name: /view deployment/i }));

    await waitFor(() => {
      expect(screen.getByTestId('deployment-detail')).toBeInTheDocument();
    });
  });
});
