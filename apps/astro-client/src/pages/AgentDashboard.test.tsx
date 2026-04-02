import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, waitFor, cleanup, fireEvent, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AgentDashboard from './AgentDashboard';

vi.mock('@/components/deployed-agent/detail/LiveRevealOverlay', () => ({
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

const orgAuth = {
  ...mockAuthContext,
  accounts: [
    { id: 'org-1', name: 'my-org', type: 'organization' as const },
    { id: 'acct-1', name: 'testuser', type: 'personal' as const },
  ],
};

function renderDashboard(path = '/dashboard', auth = mockAuthContext) {
  return renderRoute(
    [
      {
        path: '/dashboard',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: AgentDashboard as any,
        loader: () => ({ count: 0 }),
      },
    ],
    { initialEntries: [path], auth },
  );
}

describe('AgentDashboard page', () => {
  it('renders the dashboard heading', async () => {
    renderDashboard();
    await waitFor(() => {
      expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument();
    });
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

  it('shows agent count in stats area', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [
            { id: 'dep-1', name: 'alpha', display_name: 'Alpha', build_id: 'b1', namespace: 'ns-1', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-01T00:00:00Z', components: [] },
            { id: 'dep-2', name: 'beta', display_name: 'Beta', build_id: 'b2', namespace: 'ns-2', status: 'Running', replicas: 1, ready: 1, created_at: '2025-04-02T00:00:00Z', components: [] },
          ],
          count: 2,
        }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('2 agents')).toBeInTheDocument();
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

  it('shows blueprint count in stats area', async () => {
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json({
          agents: [
            { name: 'agent-1', account: 'testuser', registry: 'r', versions: [] },
            { name: 'agent-2', account: 'testuser', registry: 'r', versions: [] },
          ],
          count: 2,
        }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('2 blueprints')).toBeInTheDocument();
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

describe('navigation buttons', () => {
  it('shows Settings button for personal account', async () => {
    renderDashboard('/dashboard');
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());
    expect(screen.getAllByRole('link', { name: /settings/i })).not.toHaveLength(0);
  });

  it('hides Settings button for org account', async () => {
    renderDashboard('/dashboard?account=my-org', orgAuth);
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());
    expect(screen.queryByRole('link', { name: /settings/i })).not.toBeInTheDocument();
  });

  it('shows Browse blueprints button for both personal and org accounts', async () => {
    renderDashboard('/dashboard');
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());
    expect(screen.getAllByRole('link', { name: /browse blueprints/i })).not.toHaveLength(0);
  });
});

describe('org switcher', () => {
  it('renders the active account name', async () => {
    renderDashboard('/dashboard');
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());
    expect(screen.getByText('testuser')).toBeInTheDocument();
  });

  it('lists personal account first in dropdown regardless of array order', async () => {
    const user = userEvent.setup();
    renderDashboard('/dashboard?account=my-org', orgAuth);
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());

    // Open the switcher — orgAuth has org first, personal second in the accounts array
    await user.click(screen.getByRole('button', { name: /my-org/i }));

    const items = await screen.findAllByRole('menuitem');
    const accountItems = items.filter((el) =>
      el.textContent?.includes('testuser') || el.textContent?.includes('my-org'),
    );
    expect(accountItems[0]).toHaveTextContent('testuser');
    expect(accountItems[1]).toHaveTextContent('my-org');
  });

  it('shows member count label for org accounts', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/members', () =>
        HttpResponse.json({
          members: [
            { id: 'u1', email: 'a@a.com', role: 'member' },
            { id: 'u2', email: 'b@b.com', role: 'admin' },
          ],
        }),
      ),
    );

    renderDashboard('/dashboard?account=my-org', orgAuth);

    await waitFor(() => {
      expect(screen.getByText('2 members')).toBeInTheDocument();
    });
  });

  it('hides member count label for personal accounts', async () => {
    renderDashboard('/dashboard');
    await waitFor(() => expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument());
    expect(screen.queryByText(/\d+ members?/)).not.toBeInTheDocument();
  });
});

describe('stats', () => {
  it('shows TOTAL TOKENS and TOTAL REQUESTS labels', async () => {
    renderDashboard();
    await waitFor(() => {
      expect(screen.getByText('TOTAL TOKENS')).toBeInTheDocument();
      expect(screen.getByText('TOTAL REQUESTS')).toBeInTheDocument();
    });
  });

  it('shows token value as sum of input and output tokens', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/observability/summary', () =>
        HttpResponse.json({
          total_traces: 0,
          input_tokens: 800,
          output_tokens: 200,
          time_range: { start: '2025-01-01T00:00:00Z', end: '2025-01-02T00:00:00Z' },
        }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getAllByText('1,000')[0]).toBeInTheDocument();
    });
  });

  it('shows request count from observability summary', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/observability/summary', () =>
        HttpResponse.json({
          total_traces: 42,
          input_tokens: 0,
          output_tokens: 0,
          time_range: { start: '2025-01-01T00:00:00Z', end: '2025-01-02T00:00:00Z' },
        }),
      ),
    );

    renderDashboard();

    await waitFor(() => {
      expect(screen.getAllByText('42')[0]).toBeInTheDocument();
    });
  });

  it('shows no trend indicators', async () => {
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('TOTAL TOKENS')).toBeInTheDocument();
    });
    expect(screen.queryByText('↑')).not.toBeInTheDocument();
    expect(screen.queryByText('↓')).not.toBeInTheDocument();
  });
});

describe('reveal overlay after deploy', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function renderDashboardWithReveal(revealState: Record<string, string | null>) {
    return renderRoute(
      [
        {
          path: '/dashboard',
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: AgentDashboard as any,
          loader: () => null,
        },
        {
          path: '/:account/agents/:deploymentId',
          Component: () => <div data-testid="deployment-detail">Deployment Detail</div>,
        },
      ],
      { initialEntries: [{ pathname: '/dashboard', state: revealState }] as unknown as string[] },
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
      expect(screen.getByText(/good (morning|afternoon|evening)/i)).toBeInTheDocument();
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

  it('uses real deployment display_name when deployment is in the list', async () => {
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
      expect(within(overlay).getByText('Real Display Name')).toBeInTheDocument();
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
