import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { act, waitFor, cleanup, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { useLocation } from 'react-router';
import { server } from '@/test/msw/server';
import { mockBlueprints } from '@/test/msw/handlers';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { allAccountKeys } from '@/api/queries/keys';
import Blueprints from './Blueprints';

function LocationSearch() {
  return <span data-testid="location-search">{useLocation().search}</span>;
}

afterEach(() => {
  cleanup();
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

const twoAccountAuth = {
  ...mockAuthContext,
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' },
    { id: 'acct-2', name: 'orgaccount', type: 'org' },
  ],
};

// Serves a distinct blueprint per account so cross-account merge / filtering is observable.
function mockPerAccountBlueprints(onRequest?: (account: string) => void) {
  server.use(
    http.get('/api/v1/agents/:account', ({ params }) => {
      const account = params.account as string;
      onRequest?.(account);
      if (account === 'orgaccount') {
        return HttpResponse.json({
          agents: [{ ...mockBlueprints[0], name: 'org-only-agent', account: 'orgaccount' }],
          count: 1,
        });
      }
      const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
      return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
    }),
  );
}

describe('Blueprints – cross-account listing', () => {
  it('merges blueprints from every account the user belongs to', async () => {
    mockPerAccountBlueprints();

    renderBlueprintsPage({ auth: twoAccountAuth });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    });
  });

  it('keeps successful blueprints visible when another account fails', async () => {
    server.use(
      http.get('/api/v1/agents/:account', ({ params }) => {
        if (params.account === 'orgaccount') {
          return HttpResponse.json({ error: 'unavailable' }, { status: 503 });
        }
        const agents = mockBlueprints.filter((blueprint) => blueprint.account === 'testuser');
        return HttpResponse.json({ agents, count: agents.length });
      }),
    );

    renderBlueprintsPage({ auth: twoAccountAuth });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.getByText("Couldn't load orgaccount")).toBeInTheDocument();
    });
    expect(screen.queryByText('Failed to load blueprints')).not.toBeInTheDocument();
  });
});

describe('Blueprints – ?account= param handling', () => {
  it('applies ?account= as the initial account filter', async () => {
    mockPerAccountBlueprints();

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=orgaccount'],
      auth: twoAccountAuth,
    });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    });
    // testuser's blueprints are hidden while the filter is scoped to orgaccount.
    expect(screen.queryByRole('heading', { name: /code-reviewer/i })).not.toBeInTheDocument();
  });

  it('loads and searches only the selected account catalogs', async () => {
    const user = userEvent.setup();
    const requestedAccounts: string[] = [];
    mockPerAccountBlueprints((account) => requestedAccounts.push(account));
    const auth = {
      ...twoAccountAuth,
      accounts: [
        ...twoAccountAuth.accounts,
        { id: 'acct-3', name: 'thirdaccount', type: 'org' },
      ],
    };

    renderBlueprintsPage({
      initialEntries: [
        '/blueprints?account=testuser&account=orgaccount',
      ],
      auth,
    });

    expect(
      await screen.findByRole('heading', { name: /org-only-agent/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    expect(requestedAccounts.sort()).toEqual(['orgaccount', 'testuser']);
    expect(requestedAccounts).not.toContain('thirdaccount');

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'org-only');
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: /code-reviewer/i })).not.toBeInTheDocument();
    });
    expect(requestedAccounts.sort()).toEqual(['orgaccount', 'testuser']);
  });

  it('ignores ?account= that does not match one of the user accounts', async () => {
    mockPerAccountBlueprints();

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=ghost'],
      auth: twoAccountAuth,
    });

    // Unknown account → no filter applied → all accounts still merged.
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    });
  });
});

describe('Blueprints – account filter control', () => {
  it('narrows the list from the cached full catalog without refetching', async () => {
    const user = userEvent.setup();
    const requestedAccounts: string[] = [];
    mockPerAccountBlueprints((account) => requestedAccounts.push(account));

    const { container, queryClient } = renderBlueprintsPage({ auth: twoAccountAuth });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    });
    const initialReveal = container.querySelector(
      "[data-slot='result-set-reveal']",
    );
    const pagination = container.querySelector<HTMLElement>(
      "nav[aria-label='Blueprint list pagination']",
    );
    expect(initialReveal).not.toContainElement(pagination);
    expect(requestedAccounts.sort()).toEqual(['orgaccount', 'testuser']);

    queryClient.setDefaultOptions({
      queries: {
        retry: false,
        gcTime: Infinity,
        staleTime: 60_000,
      },
      mutations: {
        retry: false,
      },
    });
    requestedAccounts.length = 0;

    await user.click(screen.getByRole('button', { name: /filter by account/i }));
    await user.click(await screen.findByRole('button', { name: /orgaccount/i }));

    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: /code-reviewer/i })).not.toBeInTheDocument();
    });
    expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    expect(
      container.querySelector("[data-slot='result-set-reveal']"),
    ).toBe(initialReveal);
    expect(requestedAccounts).toEqual([]);
  });

  it('keeps the selected account in the URL without dropping other filters', async () => {
    const user = userEvent.setup();
    mockPerAccountBlueprints();

    renderRoute(
      [{
        path: '/blueprints',
        Component: () => (
          <>
            <LocationSearch />
            {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
            {(() => { const Page = Blueprints as any; return <Page />; })()}
          </>
        ),
      }],
      {
        initialEntries: ['/blueprints?q=agent'],
        auth: twoAccountAuth,
      },
    );

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /org-only-agent/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /filter by account/i }));
    await user.click(await screen.findByRole('button', { name: /orgaccount/i }));

    await waitFor(() => {
      const search = screen.getByTestId('location-search').textContent ?? '';
      expect(search).toContain('q=agent');
      expect(search).toContain('account=orgaccount');
    });
  });
});

describe('Blueprints – search (client-side)', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('filters the merged list to matching blueprints without refetching', async () => {
    const user = userEvent.setup();
    let requestCount = 0;

    server.use(
      http.get('/api/v1/agents/:account', () => {
        requestCount += 1;
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: /data-analyst/i })).toBeInTheDocument();
    });

    const requestsBeforeSearch = requestCount;

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'code');
    await vi.advanceTimersByTimeAsync(350);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: /data-analyst/i })).not.toBeInTheDocument();
    });

    // Search is purely client-side over the already-fetched merge — no new requests.
    expect(requestCount).toBe(requestsBeforeSearch);
  });

  it('shows the filtered empty state when search has no matches', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('/api/v1/agents/:account', () => {
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'zzznomatch');
    await vi.advanceTimersByTimeAsync(350);

    await waitFor(() => {
      expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    await vi.advanceTimersByTimeAsync(350);
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });
  });

  it('searches the complete catalog beyond the first 100 blueprints', async () => {
    const user = userEvent.setup();
    const offsets: number[] = [];
    const agents = Array.from({ length: 101 }, (_, index) => ({
      ...mockBlueprints[0],
      name: index === 100 ? 'catalog-tail' : `agent-${index + 1}`,
      account: 'testuser',
    }));

    server.use(
      http.get('/api/v1/agents/:account', ({ request }) => {
        const offset = Number(new URL(request.url).searchParams.get('offset') ?? 0);
        offsets.push(offset);
        const page = agents.slice(offset, offset + 100);
        return HttpResponse.json({
          agents: page,
          count: agents.length,
          has_more: offset + page.length < agents.length,
        });
      }),
    );

    renderBlueprintsPage();

    await waitFor(() => {
      expect(offsets).toEqual([0, 100]);
    });

    await user.type(screen.getByPlaceholderText(/search blueprints/i), 'catalog-tail');
    await vi.advanceTimersByTimeAsync(350);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'catalog-tail' })).toBeInTheDocument();
    });
  });

  it('restores the full list after clearing the search', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('/api/v1/agents/:account', () => {
        const accountBlueprints = mockBlueprints.filter((b) => b.account === 'testuser');
        return HttpResponse.json({ agents: accountBlueprints, count: accountBlueprints.length });
      }),
    );

    renderBlueprintsPage();

    const input = screen.getByPlaceholderText(/search blueprints/i);
    await user.type(input, 'zzznomatch');
    await vi.advanceTimersByTimeAsync(350);
    await waitFor(() => {
      expect(screen.getByText('No blueprints match your filters.')).toBeInTheDocument();
    });

    await user.clear(input);
    await vi.advanceTimersByTimeAsync(350);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /code-reviewer/i })).toBeInTheDocument();
    });
  });
});

describe('Blueprints – pagination (client-side)', () => {
  function mockAccountBlueprints(total: number) {
    const blueprints = Array.from({ length: total }, (_, index) => ({
      ...mockBlueprints[0],
      name: `agent-${index + 1}`,
      account: 'testuser',
    }));
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json({ agents: blueprints, count: blueprints.length }),
      ),
    );
    return blueprints;
  }

  it('hides pagination controls when the merged list fits in one page (≤ 50)', async () => {
    mockAccountBlueprints(40);

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-1$/i })).toBeInTheDocument();
    });

    expect(screen.queryByRole('button', { name: 'Page 1' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Previous page/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Next page/i })).not.toBeInTheDocument();
  });

  it('paginates the merged list client-side when it exceeds the page size', async () => {
    const user = userEvent.setup();
    mockAccountBlueprints(80);

    renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: /blueprint list pagination/i })).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: 'Page 1' })).toHaveAttribute('aria-current', 'page');
    // agent-51 lives on the second client-side page.
    expect(screen.queryByRole('heading', { name: /^agent-51$/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Page 2' }));

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-51$/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
    });
  });

  it('clamps the current page when a background update shrinks the list', async () => {
    const user = userEvent.setup();
    const blueprints = mockAccountBlueprints(120);
    const { queryClient } = renderBlueprintsPage();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Page 3' })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Page 3' }));
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-101$/i })).toBeInTheDocument();
    });

    act(() => {
      queryClient.setQueryData(
        allAccountKeys.list('blueprints', ['testuser']),
        {
          results: [{
            account: 'testuser',
            data: { agents: blueprints.slice(0, 80), count: 80 },
          }],
          failed_accounts: [],
          rejected_accounts: [],
        },
      );
    });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /^agent-51$/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
    });
    expect(screen.queryByRole('button', { name: 'Page 3' })).not.toBeInTheDocument();
  });
});
