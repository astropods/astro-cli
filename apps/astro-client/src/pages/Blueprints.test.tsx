import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockBlueprints } from '@/test/msw/handlers';
import { renderRoute } from '@/test/test-utils';
import Discover from './blueprints/Discover';

// RTL auto-cleanup requires vitest globals — run it explicitly.
afterEach(cleanup);

function renderDiscover({ initialEntries = ['/blueprints/discover'] } = {}) {
  return renderRoute(
    [
      {
        path: '/blueprints/discover',
        Component: Discover,
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

describe('Blueprints – Discover page', () => {
  describe('rendering & data loading', () => {
    it('renders agent cards after data loads', async () => {
      renderDiscover();
      await waitForAgents();

      expect(screen.getByRole('heading', { name: /data-analyst/ })).toBeInTheDocument();
    });

    it('shows a loading spinner while fetching', () => {
      renderDiscover();

      expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument();
    });

    it('shows an error state with retry button on fetch failure', async () => {
      server.use(
        http.get('/api/v1/agents', () =>
          HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
        ),
      );

      renderDiscover();

      await waitFor(() => {
        expect(screen.getByText('Failed to load blueprints')).toBeInTheDocument();
      });
      expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
    });

    it('shows empty state when no agents exist', async () => {
      server.use(
        http.get('/api/v1/agents', () =>
          HttpResponse.json({ agents: [], count: 0 }),
        ),
      );

      renderDiscover();

      await waitFor(() => {
        expect(screen.getByText('No blueprints available')).toBeInTheDocument();
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
          return HttpResponse.json({ agents: mockBlueprints, count: mockBlueprints.length });
        }),
      );

      const user = userEvent.setup();
      renderDiscover();

      await waitFor(() => {
        expect(screen.getByText('Failed to load blueprints')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /retry/i }));

      await waitForAgents();
    });
  });

  // ── Page Content ──────────────────────────────────────────────

  describe('page content', () => {
    it('shows Discover heading', async () => {
      renderDiscover();
      await waitForAgents();

      expect(screen.getByRole('heading', { level: 1, name: 'Discover blueprints' })).toBeInTheDocument();
    });
  });

  // ── Agent Card Links ──────────────────────────────────────────────────

  describe('agent card links', () => {
    it('agent cards link to /{account}/{name}', async () => {
      renderDiscover();
      await waitForAgents();

      const links = screen.getAllByRole('link');
      const agentLinks = links.filter((l) => {
        const href = l.getAttribute('href');
        return href?.startsWith('/testuser/');
      });
      expect(agentLinks).toHaveLength(2);

      const hrefs = agentLinks.map((l) => l.getAttribute('href'));
      expect(hrefs).toContain('/testuser/code-reviewer');
      expect(hrefs).toContain('/testuser/data-analyst');
    });
  });
});
