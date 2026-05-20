import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { waitFor, cleanup, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockBlueprints } from '@/test/msw/handlers';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import Blueprints from './Blueprints';

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

function renderBlueprintsPage({
  initialEntries = ['/blueprints'],
  auth = mockAuthContext,
}: {
  initialEntries?: string[];
  auth?: typeof mockAuthContext;
} = {}) {
  return renderRoute(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    [{ path: '/blueprints', Component: Blueprints as any }],
    { initialEntries, auth },
  );
}

describe('Blueprints – ?account= param handling', () => {
  it('sets active account from ?account param once accounts have loaded', async () => {
    const auth = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { id: 'acct-2', name: 'orgaccount', type: 'org' },
      ],
    };

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=orgaccount'],
      auth,
    });

    await waitFor(() => {
      expect(localStorage.getItem('astro:default-account')).toBe('orgaccount');
    });
  });

  it('does not consume ?account param before accounts have loaded (no-flicker)', async () => {
    // Simulate the initial render before auth resolves — accounts is empty.
    // The old code would call setSearchParams({}) here, stripping the param so
    // it could never be processed once accounts populated. The guard fixes this.
    const auth = { ...mockAuthContext, accounts: [] };

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=testuser'],
      auth,
    });

    // Allow any queued effects to flush.
    await new Promise((resolve) => setTimeout(resolve, 50));

    // setActiveAccount must not have been called — param is preserved for when accounts load.
    expect(localStorage.getItem('astro:default-account')).toBeNull();
  });
});

describe('Blueprints – search', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows only matching blueprints after search resolves', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('/api/v1/agents/:account', ({ request }) => {
        const url = new URL(request.url);
        const q = url.searchParams.get('q');
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        if (q?.toLowerCase() === 'development') {
          return HttpResponse.json({
            agents: accountBlueprints.filter((b) => b.name === 'code-reviewer'),
            count: 1,
          });
        }
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'development');
    await vi.advanceTimersByTimeAsync(350);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: /data-analyst/i })).not.toBeInTheDocument();
    });
  });

  it('sends q query param when searching', async () => {
    const user = userEvent.setup();
    let lastUrl = '';

    server.use(
      http.get('/api/v1/agents/:account', ({ request }) => {
        lastUrl = request.url;
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'code');

    await waitFor(
      () => {
        expect(lastUrl).toContain('q=code');
      },
      { timeout: 2000 },
    );
  });

  it('does not flash registry empty state while clearing search', async () => {
    const user = userEvent.setup();
    let requestCount = 0;

    server.use(
      http.get('/api/v1/agents/:account', async ({ request }) => {
        requestCount += 1;
        const url = new URL(request.url);
        const q = url.searchParams.get('q');
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        if (q) {
          await new Promise((resolve) => setTimeout(resolve, 100));
          return HttpResponse.json({ agents: [], count: 0 });
        }
        await new Promise((resolve) => setTimeout(resolve, 50));
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText(/search blueprints/i);
    await user.type(input, 'nomatch');
    await waitFor(() => {
      expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
    });

    await user.clear(input);
    expect(screen.queryByText(/no blueprints yet/i)).not.toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });
    expect(requestCount).toBeGreaterThan(1);
  });

  it('shows filtered empty state when search has no matches', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('/api/v1/agents/:account', ({ request }) => {
        const url = new URL(request.url);
        if (url.searchParams.get('q') === 'nomatch') {
          return HttpResponse.json({ agents: [], count: 0 });
        }
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'nomatch');

    await waitFor(() => {
      expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
    });
  });
});

describe('Blueprints – page size preference', () => {
  it('restores page size from localStorage', async () => {
    localStorage.setItem('astro:blueprints:page-size', '20');

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.getByLabelText('20 per page')).toHaveAttribute('data-state', 'on');
    });
  });

  it('persists page size when changed', async () => {
    const user = userEvent.setup();
    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText('10 per page'));

    expect(localStorage.getItem('astro:blueprints:page-size')).toBe('10');
  });
});

describe('Blueprints – pagination', () => {
  it('shows page controls and fetches the selected offset', async () => {
    const user = userEvent.setup();
    localStorage.setItem('astro:blueprints:page-size', '10');
    const accountBlueprints = Array.from({ length: 25 }, (_, index) => ({
      ...mockBlueprints[0],
      name: `agent-${index + 1}`,
    }));

    server.use(
      http.get('/api/v1/agents/:account', ({ request }) => {
        const url = new URL(request.url);
        const limit = Number(url.searchParams.get('limit') ?? 50);
        const offset = Number(url.searchParams.get('offset') ?? 0);
        const page = accountBlueprints.slice(offset, offset + limit);
        return HttpResponse.json({
          agents: page,
          count: accountBlueprints.length,
          limit,
          offset,
          has_more: offset + page.length < accountBlueprints.length,
        });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: /blueprint list pagination/i })).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: 'Page 1' })).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Page 2' }));

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /agent-11/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
    });
  });
});
