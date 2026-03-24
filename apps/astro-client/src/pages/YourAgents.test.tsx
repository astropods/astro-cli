import { describe, it, expect, afterEach, vi } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute } from '@/test/test-utils';
import YourAgents from './YourAgents';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function renderYourAgents() {
  return renderRoute(
    [
      {
        path: '/agents',
        Component: YourAgents,
      },
    ],
    { initialEntries: ['/agents'] },
  );
}

describe('YourAgents page', () => {
  it('renders display_name when available instead of slug', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [
            {
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

    renderYourAgents();

    await waitFor(() => {
      expect(screen.getByText('My Code Reviewer')).toBeInTheDocument();
    });
    // The slug should not appear as the card title
    expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
  });

  it('shows request count from observability summary', async () => {
    server.use(
      http.get('/api/v1/deployments/:id/observability/summary', () =>
        HttpResponse.json({
          total_traces: 42,
          time_range: { start: '2025-04-01T00:00:00Z', end: '2025-04-08T00:00:00Z' },
          metrics: { avg_latency_ms: 0, p95_latency_ms: 0, total_tokens: 0, error_rate: 0, traces_per_hour: 0 },
        }),
      ),
    );

    renderYourAgents();

    await waitFor(() => {
      expect(screen.getAllByText('42')[0]).toBeInTheDocument();
    });
  });

  it('shows last active as relative time from most recent trace', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2025-04-08T12:00:00Z'));

    server.use(
      http.get('/api/v1/deployments/:id/observability/traces', () =>
        HttpResponse.json({
          traces: [{ trace_id: 't1', name: 'run', status: 'success', latency_ms: 100, input: '', output: '', timestamp: '2025-04-08T10:00:00Z' }],
          total: 1,
          limit: 1,
          offset: 0,
        }),
      ),
    );

    renderYourAgents();

    await waitFor(() => {
      expect(screen.getAllByText('2 hours ago')[0]).toBeInTheDocument();
    });
  });

  it('shows last active as "less than a minute ago" for very recent traces', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2025-04-08T12:00:30Z'));

    server.use(
      http.get('/api/v1/deployments/:id/observability/traces', () =>
        HttpResponse.json({
          traces: [{ trace_id: 't1', name: 'run', status: 'success', latency_ms: 50, input: '', output: '', timestamp: '2025-04-08T12:00:00Z' }],
          total: 1,
          limit: 1,
          offset: 0,
        }),
      ),
    );

    renderYourAgents();

    await waitFor(() => {
      expect(screen.getAllByText('less than a minute ago')[0]).toBeInTheDocument();
    });
  });

  it('shows — for last active when no traces exist', async () => {
    renderYourAgents();

    await waitFor(() => {
      expect(screen.getAllByText('—')[0]).toBeInTheDocument();
    });
  });

  it('falls back to name when display_name is absent', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [
            {
              name: 'code-reviewer',
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

    renderYourAgents();

    await waitFor(() => {
      expect(screen.getByText('code-reviewer')).toBeInTheDocument();
    });
  });
});
