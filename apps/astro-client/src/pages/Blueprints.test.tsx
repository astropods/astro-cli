import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockBlueprints } from '@/test/msw/handlers';
import { renderRoute } from '@/test/test-utils';
import Explore from './Explore';
import type { Blueprint } from '@/lib/api';

// RTL auto-cleanup requires vitest globals — run it explicitly.
afterEach(cleanup);

function renderDiscover({ initialEntries = ['/explore'] } = {}) {
  return renderRoute(
    [
      {
        path: '/explore',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: Explore as any,
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
    it('shows the public Explore heading and subtitle', async () => {
      renderDiscover();
      await waitForAgents();

      expect(screen.getByRole('heading', { level: 1, name: 'Explore' })).toBeInTheDocument();
      expect(screen.getByText('Public agent configurations available to deploy in your account or organization.')).toBeInTheDocument();
    });
  });

  describe('sort', () => {
    const sortableBlueprints: Blueprint[] = [
      {
        name: 'gamma-bot',
        account: 'testuser',
        registry: 'registry.example.com',
        visibility: 'public',
        versions: [{
          build_id: 'g1',
          spec: {},
          published_at: '2025-02-01T00:00:00Z',
          agent_card: { tags: ['productivity', 'coding'] },
        }],
        heart_count: 90,
        metrics: { deploy_count: 3, lifetime_messages: 0 },
      },
      {
        name: 'alpha-bot',
        account: 'testuser',
        registry: 'registry.example.com',
        visibility: 'public',
        versions: [{
          build_id: 'a1',
          spec: {},
          published_at: '2025-03-01T00:00:00Z',
          agent_card: { tags: ['workspace', 'productivity'] },
        }],
        heart_count: 10,
        metrics: { deploy_count: 1, lifetime_messages: 0 },
      },
      {
        name: 'beta-bot',
        account: 'testuser',
        registry: 'registry.example.com',
        visibility: 'public',
        versions: [{
          build_id: 'b1',
          spec: {},
          published_at: '2025-01-01T00:00:00Z',
          agent_card: { tags: ['mcp'] },
        }],
        heart_count: 40,
        metrics: { deploy_count: 12, lifetime_messages: 0 },
      },
    ];

    function mockSortableBlueprints() {
      server.use(
        http.get('/api/v1/agents', () =>
          HttpResponse.json({ agents: sortableBlueprints, count: sortableBlueprints.length }),
        ),
      );
    }

    function cardNames() {
      return screen.getAllByRole('heading', { level: 3 }).map((el) => el.textContent);
    }

    async function chooseSort(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
      await user.click(screen.getByRole('combobox'));
      await user.click(await screen.findByRole('option', { name }));
    }

    it('sorts by deploys by default', async () => {
      mockSortableBlueprints();

      renderDiscover();

      await waitFor(() => {
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });
    });

    it('sorts by most hearted', async () => {
      mockSortableBlueprints();
      const user = userEvent.setup();

      renderDiscover();

      await waitFor(() => {
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });

      await chooseSort(user, /most hearted/i);

      await waitFor(() => {
        expect(cardNames()).toEqual(['gamma-bot', 'beta-bot', 'alpha-bot']);
      });
    });

    it('sorts by last updated', async () => {
      mockSortableBlueprints();
      const user = userEvent.setup();

      renderDiscover();

      await waitFor(() => {
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });

      await chooseSort(user, /last updated/i);

      await waitFor(() => {
        expect(cardNames()).toEqual(['alpha-bot', 'gamma-bot', 'beta-bot']);
      });
    });

    it('sorts by name A to Z', async () => {
      mockSortableBlueprints();
      const user = userEvent.setup();

      renderDiscover();

      await waitFor(() => {
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });

      await chooseSort(user, /name \(a-z\)/i);

      await waitFor(() => {
        expect(cardNames()).toEqual(['alpha-bot', 'beta-bot', 'gamma-bot']);
      });
    });

    it('filters by search below the title', async () => {
      mockSortableBlueprints();
      const user = userEvent.setup();

      renderDiscover();

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Search blueprints...')).toBeInTheDocument();
      });

      await user.type(screen.getByPlaceholderText('Search blueprints...'), 'workspace');

      await waitFor(() => {
        expect(cardNames()).toEqual(['alpha-bot']);
      });
    });

    it('shows category counts in a dropdown and supports multi-category filters', async () => {
      mockSortableBlueprints();
      const user = userEvent.setup();

      renderDiscover();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /filter categories/i })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /filter categories/i }));
      expect(document.querySelector('[data-slot="multi-select-list"]')).toHaveClass('max-h-72');

      await user.click(await screen.findByRole('button', { name: 'Productivity 2' }));

      await waitFor(() => {
        expect(cardNames()).toEqual(['gamma-bot', 'alpha-bot']);
      });

      await user.click(await screen.findByRole('button', { name: 'MCP 1' }));

      await waitFor(() => {
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });

      expect(screen.getByText('Filter (2)')).toBeInTheDocument();

      await user.click(await screen.findByRole('button', { name: 'Remove filters' }));

      await waitFor(() => {
        expect(screen.queryByText('Filter (2)')).not.toBeInTheDocument();
        expect(cardNames()).toEqual(['beta-bot', 'gamma-bot', 'alpha-bot']);
      });
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
