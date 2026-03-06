import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute } from '@/test/test-utils';
import YourAgents from './YourAgents';

afterEach(cleanup);

function renderYourAgents() {
  return renderRoute(
    [
      {
        path: '/agents',
        // @ts-expect-error: `matches` won't align between test code and app code
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
