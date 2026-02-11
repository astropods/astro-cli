import { describe, it, expect, vi, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Routes, Route, Outlet } from 'react-router-dom';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockAgents } from '@/test/msw/handlers';
import { renderWithProviders } from '@/test/test-utils';
import { Hire } from './Hire';

// RTL auto-cleanup requires vitest globals — run it explicitly.
afterEach(cleanup);

// Hire calls useOutletContext(), so we render it inside a parent route
// that provides the context via <Outlet context={...} />.
function renderHire({
  initialEntries = ['/hire'],
  openAuthModal = vi.fn(),
}: { initialEntries?: string[]; openAuthModal?: ReturnType<typeof vi.fn> } = {}) {
  const ParentLayout = () => <Outlet context={{ openAuthModal }} />;

  const result = renderWithProviders(
    <Routes>
      <Route element={<ParentLayout />}>
        <Route path="/hire" element={<Hire />} />
      </Route>
    </Routes>,
    { initialEntries },
  );

  return { ...result, openAuthModal };
}

/** Wait for agents to finish loading. */
async function waitForAgents() {
  await waitFor(() => {
    expect(screen.getByText('code-reviewer')).toBeInTheDocument();
  });
}

// ── Rendering & Data Loading ────────────────────────────────────────

describe('Hire page', () => {
  describe('rendering & data loading', () => {
    it('renders agent cards after data loads', async () => {
      renderHire();
      await waitForAgents();

      expect(screen.getByText('data-analyst')).toBeInTheDocument();
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

  // ── Search ──────────────────────────────────────────────────────────

  describe('search', () => {
    it('filters agents by name when typing in search', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.type(screen.getByPlaceholderText(/search/i), 'data');

      expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
      expect(screen.getByText('data-analyst')).toBeInTheDocument();
    });

    it('search is case-insensitive', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.type(screen.getByPlaceholderText(/search/i), 'CODE');

      expect(screen.getByText('code-reviewer')).toBeInTheDocument();
      expect(screen.queryByText('data-analyst')).not.toBeInTheDocument();
    });

    it('shows all agents when search is cleared', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      const input = screen.getByPlaceholderText(/search/i);
      await user.type(input, 'data');
      expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();

      await user.clear(input);
      expect(screen.getByText('code-reviewer')).toBeInTheDocument();
      expect(screen.getByText('data-analyst')).toBeInTheDocument();
    });
  });

  // ── Category Filter ─────────────────────────────────────────────────

  describe('category filter', () => {
    it('populates category dropdown from agent tags', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.click(screen.getByRole('button', { name: /industry/i }));

      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: 'All' })).toBeInTheDocument();
      });
      expect(screen.getByRole('menuitem', { name: 'Analytics' })).toBeInTheDocument();
      expect(screen.getByRole('menuitem', { name: 'Developer Tools' })).toBeInTheDocument();
      expect(screen.getByRole('menuitem', { name: 'Security' })).toBeInTheDocument();
    });

    it('filters agents by selected category', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.click(screen.getByRole('button', { name: /industry/i }));
      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: 'Analytics' })).toBeInTheDocument();
      });
      await user.click(screen.getByRole('menuitem', { name: 'Analytics' }));

      await waitFor(() => {
        expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
      });
      expect(screen.getByText('data-analyst')).toBeInTheDocument();
    });

    it('"All" resets the category filter', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      // Filter by Analytics first
      await user.click(screen.getByRole('button', { name: /industry/i }));
      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: 'Analytics' })).toBeInTheDocument();
      });
      await user.click(screen.getByRole('menuitem', { name: 'Analytics' }));

      await waitFor(() => {
        expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
      });

      // Reset — button now shows the selected category name
      await user.click(screen.getByRole('button', { name: 'Analytics' }));
      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: 'All' })).toBeInTheDocument();
      });
      await user.click(screen.getByRole('menuitem', { name: 'All' }));

      await waitFor(() => {
        expect(screen.getByText('code-reviewer')).toBeInTheDocument();
      });
      expect(screen.getByText('data-analyst')).toBeInTheDocument();
    });
  });

  // ── Combined Filters ────────────────────────────────────────────────

  describe('combined filters', () => {
    it('applies search and category filter together', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      // Select "Developer Tools" category
      await user.click(screen.getByRole('button', { name: /industry/i }));
      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: 'Developer Tools' })).toBeInTheDocument();
      });
      await user.click(screen.getByRole('menuitem', { name: 'Developer Tools' }));

      await waitFor(() => {
        expect(screen.queryByText('data-analyst')).not.toBeInTheDocument();
      });
      expect(screen.getByText('code-reviewer')).toBeInTheDocument();

      // Now search for something that doesn't match the remaining agent
      await user.type(screen.getByPlaceholderText(/search/i), 'data');

      expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
      expect(screen.queryByText('data-analyst')).not.toBeInTheDocument();
    });

    it('shows empty grid when search matches no agents', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      await user.type(screen.getByPlaceholderText(/search/i), 'zzz-no-match');

      expect(screen.queryByText('code-reviewer')).not.toBeInTheDocument();
      expect(screen.queryByText('data-analyst')).not.toBeInTheDocument();
      // The agent grid should be empty but the page should not show the "No agents available" empty state
      // (that state is for when the API returns zero agents)
      expect(screen.queryByText('No agents available')).not.toBeInTheDocument();
    });
  });

  // ── Sorting ─────────────────────────────────────────────────────────

  describe('sorting', () => {
    it('defaults to most-recent-first based on published_at', async () => {
      renderHire();
      await waitForAgents();

      // data-analyst (March) is more recent than code-reviewer (Feb)
      const headings = screen.getAllByRole('heading', { level: 3 });
      const names = headings.map((h) => h.textContent);
      expect(names).toEqual(['data-analyst', 'code-reviewer']);
    });

    it('sorts alphabetically by name when "Name A-Z" is selected', async () => {
      const user = userEvent.setup();
      renderHire();
      await waitForAgents();

      // Default is most-recent-first: data-analyst before code-reviewer
      const headingsBefore = screen.getAllByRole('heading', { level: 3 });
      expect(headingsBefore.map((h) => h.textContent)).toEqual(['data-analyst', 'code-reviewer']);

      await user.click(screen.getByRole('button', { name: /most recent/i }));
      await waitFor(() => {
        expect(screen.getByRole('menuitem', { name: /name a-z/i })).toBeInTheDocument();
      });
      await user.click(screen.getByRole('menuitem', { name: /name a-z/i }));

      // A-Z flips the order
      const headingsAfter = screen.getAllByRole('heading', { level: 3 });
      expect(headingsAfter.map((h) => h.textContent)).toEqual(['code-reviewer', 'data-analyst']);
    });
  });

  // ── Agent Card Interactions ─────────────────────────────────────────

  describe('agent card interactions', () => {
    it('"View details" links to /hire/{slug}', async () => {
      renderHire();
      await waitForAgents();

      const links = screen.getAllByRole('link', { name: /view details/i });
      // Default sort is most-recent-first, so data-analyst appears first
      expect(links[0]).toHaveAttribute('href', '/hire/data-analyst');
    });

    it('"Install Agent" triggers the auth modal callback', async () => {
      const user = userEvent.setup();
      const { openAuthModal } = renderHire();
      await waitForAgents();

      const installButtons = screen.getAllByRole('button', { name: /install agent/i });
      await user.click(installButtons[0]);

      expect(openAuthModal).toHaveBeenCalledTimes(1);
      expect(openAuthModal).toHaveBeenCalledWith();
    });
  });

  // ── Wizard Overlay ──────────────────────────────────────────────────

  describe('wizard overlay', () => {
    it('opens wizard when URL has ?start=true', async () => {
      renderHire({ initialEntries: ['/hire?start=true'] });

      await waitFor(() => {
        expect(screen.getByRole('dialog', { name: /find agents wizard/i })).toBeInTheDocument();
      });
    });

    it('does not show wizard by default', async () => {
      renderHire();
      await waitForAgents();

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });
});
