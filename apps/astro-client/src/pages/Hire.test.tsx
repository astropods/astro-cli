import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockAgents } from '@/test/msw/handlers';
import { renderRoute } from '@/test/test-utils';
import Hire from './Hire';

// RTL auto-cleanup requires vitest globals — run it explicitly.
afterEach(cleanup);

function renderHire({ initialEntries = ['/hire'] } = {}) {
  return renderRoute(
    [
      {
        path: '/hire',
        // @ts-expect-error: `matches` won't align between test code and app code
        Component: Hire,
      },
    ],
    { initialEntries },
  );
}

/** Wait for agents to finish loading. */
async function waitForAgents() {
  await waitFor(() => {
    expect(screen.getByRole('heading', { name: /code-reviewer/ })).toBeInTheDocument();
  });
}

// ── Rendering & Data Loading ────────────────────────────────────────

describe('Hire page', () => {
  describe('rendering & data loading', () => {
    it('renders agent cards after data loads', async () => {
      renderHire();
      await waitForAgents();

      expect(screen.getByRole('heading', { name: /data-analyst/ })).toBeInTheDocument();
    });

    it('shows a loading spinner while fetching', () => {
      renderHire();

      expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument();
    });

    it('shows an error state with retry button on fetch failure', async () => {
      server.use(
        http.get('/api/v1/agents', () =>
          HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
        ),
      );

      renderHire();

      await waitFor(() => {
        expect(screen.getByText('Failed to load agents')).toBeInTheDocument();
      });
      expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
    });

    it('shows empty state when no agents exist', async () => {
      server.use(
        http.get('/api/v1/agents', () =>
          HttpResponse.json({ agents: [], count: 0 }),
        ),
      );

      renderHire();

      await waitFor(() => {
        expect(screen.getByText('No agents available')).toBeInTheDocument();
      });
    });

    it('refetches when retry button is clicked', async () => {
      let callCount = 0;
      server.use(
        http.get('/api/v1/agents', () => {
          callCount++;
          if (callCount === 1) {
            return HttpResponse.json({ error: 'internal_error' }, { status: 500 });
          }
          return HttpResponse.json({ agents: mockAgents, count: mockAgents.length });
        }),
      );

      const user = userEvent.setup();
      renderHire();

      await waitFor(() => {
        expect(screen.getByText('Failed to load agents')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /retry/i }));

      await waitForAgents();
    });
  });

  // ── Category Filter ─────────────────────────────────────────────────

  describe('category filter', () => {
    it('renders category sidebar from agent tags', async () => {
      renderHire();
      await waitForAgents();

      expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Analytics' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Developer Tools' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Security' })).toBeInTheDocument();
    });

    it('filters agents by selected category', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.click(screen.getByRole('button', { name: 'Analytics' }));

      await waitFor(() => {
        expect(screen.queryByRole('heading', { name: /code-reviewer/ })).not.toBeInTheDocument();
      });
      expect(screen.getByRole('heading', { name: /data-analyst/ })).toBeInTheDocument();
    });

    it('"All" resets the category filter', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      // Filter by Analytics first
      await user.click(screen.getByRole('button', { name: 'Analytics' }));
      await waitFor(() => {
        expect(screen.queryByRole('heading', { name: /code-reviewer/ })).not.toBeInTheDocument();
      });

      // Reset with All
      await user.click(screen.getByRole('button', { name: 'All' }));
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: /code-reviewer/ })).toBeInTheDocument();
      });
      expect(screen.getByRole('heading', { name: /data-analyst/ })).toBeInTheDocument();
    });
  });

  // ── Agent Card Links ──────────────────────────────────────────────────

  describe('agent card links', () => {
    it('agent cards link to /hire/{account}/{name}', async () => {
      renderHire();
      await waitForAgents();

      const links = screen.getAllByRole('link');
      const agentLinks = links.filter((l) => l.getAttribute('href')?.startsWith('/hire/'));
      expect(agentLinks).toHaveLength(2);

      const hrefs = agentLinks.map((l) => l.getAttribute('href'));
      expect(hrefs).toContain('/hire/testuser/code-reviewer');
      expect(hrefs).toContain('/hire/testuser/data-analyst');
    });
  });
});
