import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { waitFor, cleanup, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockBlueprints } from '@/test/msw/handlers';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import Blueprints from './Blueprints';
import type { Blueprint, UserResourcePage } from '@/lib/api';

function userBlueprints(
  blueprints: Blueprint[],
  page: UserResourcePage = { limit: 50 },
) {
  return HttpResponse.json({
    blueprints,
    page,
    scope: { accounts: ['testuser'], all: true },
  });
}

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
  it('keeps ?account page-local instead of changing the active account', async () => {
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

    await waitFor(() => expect(screen.getByRole('button', { name: 'Filter by account' })).toHaveTextContent('orgaccount'));
    expect(localStorage.getItem('astro:default-account')).toBeNull();
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
      http.get('/api/v1/me/blueprints', ({ request }) => {
        const url = new URL(request.url);
        const q = url.searchParams.get('q');
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        if (q?.toLowerCase() === 'development') {
          return userBlueprints(accountBlueprints.filter((b) => b.name === 'code-reviewer'));
        }
        return userBlueprints(accountBlueprints);
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
      http.get('/api/v1/me/blueprints', ({ request }) => {
        lastUrl = request.url;
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return userBlueprints(accountBlueprints);
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
      http.get('/api/v1/me/blueprints', async ({ request }) => {
        requestCount += 1;
        const url = new URL(request.url);
        const q = url.searchParams.get('q');
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        if (q) {
          await new Promise((resolve) => setTimeout(resolve, 100));
          return userBlueprints([]);
        }
        await new Promise((resolve) => setTimeout(resolve, 50));
        return userBlueprints(accountBlueprints);
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
      http.get('/api/v1/me/blueprints', ({ request }) => {
        const url = new URL(request.url);
        if (url.searchParams.get('q') === 'nomatch') {
          return userBlueprints([]);
        }
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return userBlueprints(accountBlueprints);
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

describe('Blueprints – empty states', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/me/blueprints', () => userBlueprints([])),
    );
  });

  it('shows onboarding for an empty implicit personal scope', async () => {
    renderBlueprintsPage();

    expect(await screen.findByText('No blueprints yet')).toBeInTheDocument();
    expect(screen.queryByText('No blueprints match your filters.')).not.toBeInTheDocument();
    expect(screen.queryByRole('navigation', { name: 'Blueprint list pagination' })).not.toBeInTheDocument();
  });

  it('shows the filtered empty state for an explicit scope', async () => {
    renderBlueprintsPage({ initialEntries: ['/blueprints?scope=all'] });

    expect(await screen.findByText('No blueprints match your filters.')).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/search blueprints/i)).toBeInTheDocument();
    expect(screen.queryByText('No blueprints yet')).not.toBeInTheDocument();
  });
});

describe('Blueprints – pagination', () => {
  function mockAccountBlueprints(total: number) {
    const requests: string[] = [];
    const blueprints = Array.from({ length: total }, (_, index) => ({
      ...mockBlueprints[0],
      name: `agent-${index + 1}`,
    }));
    server.use(
      http.get('/api/v1/me/blueprints', ({ request }) => {
        const url = new URL(request.url);
        requests.push(url.search);
        const limit = Number(url.searchParams.get('limit') ?? 50);
        const offset = Number(url.searchParams.get('cursor') ?? 0);
        const page = blueprints.slice(offset, offset + limit);
        const hasMore = offset + page.length < blueprints.length;
        return userBlueprints(page, {
          limit,
          ...(hasMore ? { next_cursor: String(offset + page.length) } : {}),
        });
      }),
    );
    return { blueprints, requests };
  }

  it('hides pagination controls when total count fits in one page (≤ 50)', async () => {
    mockAccountBlueprints(40);

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-1$/i })).toBeInTheDocument();
    });

    expect(screen.queryByRole('navigation', { name: 'Blueprint list pagination' })).not.toBeInTheDocument();
  });

  it('uses numbered controls and reuses a previously loaded keyset page', async () => {
    const user = userEvent.setup();
    const { requests } = mockAccountBlueprints(120);

    renderBlueprintsPage();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Page 2' })).toBeInTheDocument());
    expect(screen.getByRole('heading', { name: /^agent-1$/i })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: /^agent-51$/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Page 2' }));

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-51$/i })).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: /^agent-1$/i })).not.toBeInTheDocument();
    });
    expect(requests).toHaveLength(2);

    await user.click(screen.getByRole('button', { name: 'Page 1' }));
    expect(screen.getByRole('heading', { name: /^agent-1$/i })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: /^agent-51$/i })).not.toBeInTheDocument();
    expect(requests).toHaveLength(2);
  });
});
