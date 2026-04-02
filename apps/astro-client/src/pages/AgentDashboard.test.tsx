import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup, fireEvent } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute } from '@/test/test-utils';
import AgentDashboard from './AgentDashboard';

afterEach(() => {
  cleanup();
});

function renderDashboard(path = '/dashboard') {
  return renderRoute(
    [
      {
        path: '/dashboard',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: AgentDashboard as any,
        loader: () => null,
      },
    ],
    { initialEntries: [path] },
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
            {
              id: 'dep-1',
              name: 'alpha',
              display_name: 'Alpha',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-01T00:00:00Z',
              components: [],
            },
            {
              id: 'dep-2',
              name: 'beta',
              display_name: 'Beta',
              build_id: 'b2',
              namespace: 'ns-2',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-02T00:00:00Z',
              components: [],
            },
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
            {
              id: 'dep-1',
              name: 'code-reviewer',
              display_name: 'Code Reviewer',
              build_id: 'b1',
              namespace: 'ns-1',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-01T00:00:00Z',
              components: [],
            },
            {
              id: 'dep-2',
              name: 'data-analyst',
              display_name: 'Data Analyst',
              build_id: 'b2',
              namespace: 'ns-2',
              status: 'Running',
              replicas: 1,
              ready: 1,
              created_at: '2025-04-02T00:00:00Z',
              components: [],
            },
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

    const searchInput = screen.getByPlaceholderText('Search agents...');
    fireEvent.change(searchInput, { target: { value: 'code' } });

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
            {
              id: 'dep-1',
              name: 'code-reviewer',
              display_name: 'Code Reviewer',
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

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Code Reviewer')).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText('Search agents...'), {
      target: { value: 'zzz-no-match' },
    });

    await waitFor(() => {
      expect(screen.getByText('No agents match your filters.')).toBeInTheDocument();
    });
  });
});
