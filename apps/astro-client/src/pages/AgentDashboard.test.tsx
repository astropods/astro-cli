import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, waitFor, cleanup, fireEvent, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AgentDashboard from './AgentDashboard';

vi.mock('@/components/DeploymentAgentCard', () => ({
  DeploymentAgentCard: ({
    deployment,
  }: {
    deployment: { display_name?: string; name: string };
  }) => (
    <div data-testid="deployment-card">
      {deployment.display_name ?? deployment.name}
    </div>
  ),
}));

vi.mock('@/components/ui/LiveRevealOverlay', () => ({
  LiveRevealOverlay: ({
    deployment,
    onDismiss,
    onViewDeployment,
  }: {
    deployment: { display_name?: string; name: string };
    onDismiss: () => void;
    onViewDeployment: () => void;
  }) => (
    <div data-testid="live-reveal-overlay">
      <span>{deployment.display_name ?? deployment.name}</span>
      <button onClick={onDismiss}>Dismiss</button>
      <button onClick={onViewDeployment}>View deployment</button>
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});


function renderDashboard(path = '/agents', auth = mockAuthContext) {
  return renderRoute(
    [
      {
        path: '/agents',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: AgentDashboard as any,
      },
    ],
    { initialEntries: [path], auth },
  );
}

describe('AgentDashboard page', () => {
  it('renders the Agents heading', async () => {
    renderDashboard();
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: 'Agents' })).toBeInTheDocument();
    });
  });

  it('does not render a status filter', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
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
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
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


  it('shows empty state when no agents are deployed', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ deployments: [], count: 0 }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('No agents deployed yet')).toBeInTheDocument();
    });
  });


  it('filters agents by search text', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
            { id: 'dep-2', name: 'data-analyst', display_name: 'Data Analyst', build_id: 'b2', namespace: 'ns-2', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-02T00:00:00Z', components: [] },
          ],
          count: 2,
        }),
      ),
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
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [
            { id: 'dep-1', name: 'code-reviewer', display_name: 'Code Reviewer', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
          ],
          count: 1,
        }),
      ),
    );

    renderDashboard();
    await waitFor(() => expect(screen.getByText('Code Reviewer')).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), { target: { value: 'zzz-no-match' } });

    await waitFor(() => {
      expect(screen.getByText('No agents match your filters.')).toBeInTheDocument();
    });
  });
});

describe('AgentDashboard – cross-account', () => {
  const twoAccountAuth = {
    ...mockAuthContext,
    accounts: [
      { id: 'acct-1', name: 'testuser', type: 'personal' },
      { id: 'acct-2', name: 'orgaccount', type: 'org' },
    ],
  };

  function mockPerAccountDeployments(onRequest?: (account: string) => void) {
    server.use(
      http.get('/api/v1/deployments', ({ request }) => {
        const account = new URL(request.url).searchParams.get('account');
        if (account) onRequest?.(account);
        const dep = (id: string, name: string, display: string) => ({
          id,
          name,
          display_name: display,
          build_id: 'b1',
          namespace: `ns-${id}`,
          status: 'Running',
          replicas: 1,
          ready: 1,
          created_at: '2025-04-01T00:00:00Z',
          components: [],
        });
        if (account === 'orgaccount') {
          return HttpResponse.json({ deployments: [dep('dep-org', 'org-agent', 'Org Agent')], count: 1 });
        }
        return HttpResponse.json({ deployments: [dep('dep-user', 'user-agent', 'User Agent')], count: 1 });
      }),
    );
  }

  function renderTwoAccountDashboard() {
    return renderRoute(
      [
        {
          path: '/agents',
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: AgentDashboard as any,
        },
      ],
      { initialEntries: ['/agents'], auth: twoAccountAuth },
    );
  }

  it('merges deployments from every account the user belongs to', async () => {
    mockPerAccountDeployments();

    renderTwoAccountDashboard();

    await waitFor(() => {
      expect(screen.getByText('User Agent')).toBeInTheDocument();
      expect(screen.getByText('Org Agent')).toBeInTheDocument();
    });
  });

  it('stops after a short final deployment page when has_more is omitted', async () => {
    const offsets: number[] = [];
    server.use(
      http.get(
        '/api/v1/accounts/:account/observability/deployment-summaries',
        () => HttpResponse.json({ summaries: {} }),
      ),
      http.get('/api/v1/me/deployments', ({ request }) => {
        const offset = Number(new URL(request.url).searchParams.get('offset') ?? 0);
        offsets.push(offset);
        const deployments = Array.from(
          { length: offset === 0 ? 100 : 1 },
          (_, index) => {
            const number = offset + index;
            return {
              id: `dep-${number}`,
              name: `agent-${number}`,
              display_name: `Deployment ${number}`,
              build_id: 'b1',
              namespace: `ns-${number}`,
              status: 'Running',
              created_at: '2025-04-01T00:00:00Z',
            };
          },
        );
        return HttpResponse.json({
          results: [{
            account: 'testuser',
            data: {
              deployments,
              count: 101,
            },
            count: 101,
            limit: 100,
            offset,
          }],
          failed_accounts: [],
          rejected_accounts: [],
        });
      }),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Deployment 100')).toBeInTheDocument();
    });
    expect(screen.getAllByTestId('deployment-card')).toHaveLength(101);
    for (let number = 0; number < 101; number++) {
      expect(screen.getByText(`Deployment ${number}`)).toBeInTheDocument();
    }
    expect(offsets).toEqual([0, 100]);
    expect(screen.queryByText(/Couldn't load testuser/)).not.toBeInTheDocument();
  });

  it('stops and de-duplicates when a deployment page repeats', async () => {
    const offsets: number[] = [];
    const deployments = Array.from({ length: 100 }, (_, number) => ({
      id: `dep-${number}`,
      name: `agent-${number}`,
      display_name: `Deployment ${number}`,
      build_id: 'b1',
      namespace: `ns-${number}`,
      status: 'Running',
      created_at: '2025-04-01T00:00:00Z',
    }));
    server.use(
      http.get(
        '/api/v1/accounts/:account/observability/deployment-summaries',
        () => HttpResponse.json({ summaries: {} }),
      ),
      http.get('/api/v1/me/deployments', ({ request }) => {
        const offset = Number(new URL(request.url).searchParams.get('offset') ?? 0);
        offsets.push(offset);
        return HttpResponse.json({
          results: [{
            account: 'testuser',
            data: { deployments, count: 200 },
            count: 200,
            limit: 100,
            offset,
            has_more: true,
          }],
          failed_accounts: [],
          rejected_accounts: [],
        });
      }),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getAllByTestId('deployment-card')).toHaveLength(100);
      expect(offsets).toEqual([0, 100]);
    });
    expect(screen.queryByText(/Couldn't load testuser/)).not.toBeInTheDocument();
  });

  it('narrows to the selected account via the account filter', async () => {
    const user = userEvent.setup();
    const requestedAccounts: string[] = [];
    mockPerAccountDeployments((account) => requestedAccounts.push(account));

    renderTwoAccountDashboard();

    await waitFor(() => {
      expect(screen.getByText('Org Agent')).toBeInTheDocument();
    });
    requestedAccounts.length = 0;

    await user.click(screen.getByRole('button', { name: /filter by account/i }));
    await user.click(await screen.findByRole('button', { name: /orgaccount/i }));

    await waitFor(() => {
      expect(screen.queryByText('User Agent')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Org Agent')).toBeInTheDocument();
    expect(requestedAccounts).toEqual(['orgaccount']);
  });

  it('keeps successful agents visible and retries failed accounts', async () => {
    const user = userEvent.setup();
    let orgFails = true;

    server.use(
      http.get('/api/v1/deployments', ({ request }) => {
        const account = new URL(request.url).searchParams.get('account');
        if (account === 'orgaccount') {
          return orgFails
            ? HttpResponse.json({ error: 'unavailable' }, { status: 503 })
            : HttpResponse.json({
                deployments: [{
                  id: 'dep-org',
                  name: 'org-agent',
                  display_name: 'Org Agent',
                  build_id: 'b1',
                  namespace: 'ns-org',
                  status: 'Running',
                  created_at: '2025-04-01T00:00:00Z',
                }],
                count: 1,
              });
        }
        return HttpResponse.json({
          deployments: [{
            id: 'dep-user',
            name: 'user-agent',
            display_name: 'User Agent',
            build_id: 'b1',
            namespace: 'ns-user',
            status: 'Running',
            created_at: '2025-04-01T00:00:00Z',
          }],
          count: 1,
        });
      }),
    );

    renderTwoAccountDashboard();

    await waitFor(() => {
      expect(screen.getByText('User Agent')).toBeInTheDocument();
      expect(screen.getByText("Couldn't load orgaccount")).toBeInTheDocument();
    });
    expect(screen.queryByText('No agents deployed yet')).not.toBeInTheDocument();

    orgFails = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    await waitFor(() => {
      expect(screen.getByText('Org Agent')).toBeInTheDocument();
      expect(screen.queryByText("Couldn't load orgaccount")).not.toBeInTheDocument();
    });
  });

  it('does not show the onboarding empty state when every account read fails', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ error: 'unavailable' }, { status: 503 }),
      ),
    );

    renderTwoAccountDashboard();

    await waitFor(() => {
      expect(screen.getByText("Couldn't load 2 accounts")).toBeInTheDocument();
    });
    expect(screen.queryByText('No agents deployed yet')).not.toBeInTheDocument();
  });
});

describe('reveal overlay after deploy', () => {
   
  function renderDashboardWithReveal(revealState: Record<string, string | null>) {
    return renderRoute(
      [
        {
          path: '/agents',
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: AgentDashboard as any,
        },
        {
          // `deploymentPath(account, id)` with no tab now resolves directly
          // to the `/deployments` segment (previously bare, routed through
          // `AgentDetailRedirect`). Match that target here.
          path: '/:account/agents/:deploymentId/deployments',
          Component: () => <div data-testid="deployment-detail">Deployment Detail</div>,
        },
      ],
      { initialEntries: [{ pathname: '/agents', state: revealState }] as unknown as string[] },
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
      expect(screen.getByRole('heading', { level: 1, name: 'Agents' })).toBeInTheDocument();
    });
    expect(screen.queryByTestId('live-reveal-overlay')).not.toBeInTheDocument();
  });

  it('uses display name from state as optimistic fallback before deployments load', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ deployments: [], count: 0 }),
      ),
    );

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent Display',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(screen.getByText('My Agent Display')).toBeInTheDocument();
    });
  });

  it('keeps location state display_name even after deployment query loads', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
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

    renderDashboardWithReveal({
      revealDeploymentId: 'dep-new',
      revealAgentName: 'my-agent',
      revealDisplayName: 'My Agent',
      revealAvatarUrl: null,
    });

    await waitFor(() => {
      expect(screen.getByTestId('live-reveal-overlay')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /view deployment/i }));

    await waitFor(() => {
      expect(screen.getByTestId('deployment-detail')).toBeInTheDocument();
    });
  });
});
