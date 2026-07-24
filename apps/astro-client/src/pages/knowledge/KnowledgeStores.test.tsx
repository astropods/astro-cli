import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import KnowledgeStores from './KnowledgeStores';
import type { KnowledgeStore } from '@/lib/api';

afterEach(() => cleanup());

const twoAccountAuth = {
  ...mockAuthContext,
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' },
    { id: 'acct-2', name: 'orgaccount', type: 'org' },
  ],
};

function store(name: string): KnowledgeStore {
  return {
    id: `id-${name}`,
    arn: `arn-${name}`,
    name,
    provider: 'postgres',
    mode: 'managed',
    status: 'ready',
    storage: '10GB',
    created_at: '2025-04-01T00:00:00Z',
    updated_at: '2025-04-01T00:00:00Z',
  };
}

function mockPerAccountKnowledge(onRequest?: (account: string) => void) {
  server.use(
    http.get('/api/v1/accounts/:account/knowledge', ({ params }) => {
      const account = params.account as string;
      onRequest?.(account);
      if (account === 'orgaccount') return HttpResponse.json([store('org-store')]);
      return HttpResponse.json([store('user-store')]);
    }),
  );
}

function renderKnowledge(auth = twoAccountAuth) {
  return renderRoute(
    [
      {
        path: '/knowledge',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: KnowledgeStores as any,
      },
    ],
    { initialEntries: ['/knowledge'], auth },
  );
}

describe('KnowledgeStores – cross-account', () => {
  it('merges stores from every account and shows an account column', async () => {
    mockPerAccountKnowledge();

    renderKnowledge();

    await waitFor(() => {
      expect(screen.getByText('user-store')).toBeInTheDocument();
      expect(screen.getByText('org-store')).toBeInTheDocument();
    });
    // Owning account is surfaced in the table.
    expect(screen.getAllByText('orgaccount').length).toBeGreaterThan(0);
  });

  it('walks every knowledge page when has_more is omitted', async () => {
    const offsets: number[] = [];
    server.use(
      http.get('/api/v1/me/knowledge', ({ request }) => {
        const offset = Number(new URL(request.url).searchParams.get('offset') ?? 0);
        offsets.push(offset);
        const stores = Array.from(
          { length: offset === 0 ? 100 : 1 },
          (_, index) => store(`store-${offset + index}`),
        );
        return HttpResponse.json({
          results: [{
            account: 'testuser',
            data: stores,
            count: 101,
            limit: 100,
            offset,
          }],
          failed_accounts: [],
          rejected_accounts: [],
        });
      }),
    );

    renderKnowledge(mockAuthContext);

    expect(await screen.findByText('store-100')).toBeInTheDocument();
    expect(offsets).toEqual([0, 100]);
  });

  it('narrows to the selected account via the account filter', async () => {
    const user = userEvent.setup();
    const requestedAccounts: string[] = [];
    mockPerAccountKnowledge((account) => requestedAccounts.push(account));

    renderKnowledge();

    await waitFor(() => {
      expect(screen.getByText('org-store')).toBeInTheDocument();
    });
    requestedAccounts.length = 0;

    await user.click(screen.getByRole('button', { name: /filter by account/i }));
    await user.click(await screen.findByRole('button', { name: /orgaccount/i }));

    await waitFor(() => {
      expect(screen.queryByText('user-store')).not.toBeInTheDocument();
    });
    expect(screen.getByText('org-store')).toBeInTheDocument();
    expect(requestedAccounts).toEqual(['orgaccount']);
  });

  it('shows a clearable filtered-empty state instead of onboarding', async () => {
    const user = userEvent.setup();
    server.use(
      http.get('/api/v1/accounts/:account/knowledge', ({ params }) =>
        HttpResponse.json(params.account === 'testuser' ? [store('user-store')] : []),
      ),
    );

    renderKnowledge();

    await screen.findByText('user-store');
    await user.click(screen.getByRole('button', { name: /filter by account/i }));
    await user.click(await screen.findByRole('button', { name: /orgaccount/i }));

    expect(await screen.findByText('No knowledge stores match your filters.')).toBeInTheDocument();
    expect(screen.queryByText('No knowledge stores yet')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(await screen.findByText('user-store')).toBeInTheDocument();
  });

  it('keeps successful rows visible and retries only failed accounts', async () => {
    const user = userEvent.setup();
    let orgFails = true;

    server.use(
      http.get('/api/v1/accounts/:account/knowledge', ({ params }) => {
        if (params.account === 'orgaccount') {
          return orgFails
            ? HttpResponse.json({ error: 'unavailable' }, { status: 503 })
            : HttpResponse.json([store('org-store')]);
        }
        return HttpResponse.json([store('user-store')]);
      }),
    );

    renderKnowledge();

    await waitFor(() => {
      expect(screen.getByText('user-store')).toBeInTheDocument();
      expect(screen.getByText("Couldn't load orgaccount")).toBeInTheDocument();
    });
    expect(screen.queryByText('No knowledge stores yet')).not.toBeInTheDocument();

    orgFails = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    await waitFor(() => {
      expect(screen.getByText('org-store')).toBeInTheDocument();
      expect(screen.queryByText("Couldn't load orgaccount")).not.toBeInTheDocument();
    });
  });

  it('does not show the onboarding empty state when every account read fails', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/knowledge', () =>
        HttpResponse.json({ error: 'unavailable' }, { status: 503 }),
      ),
    );

    renderKnowledge();

    await waitFor(() => {
      expect(screen.getByText("Couldn't load 2 accounts")).toBeInTheDocument();
    });
    expect(screen.queryByText('No knowledge stores yet')).not.toBeInTheDocument();
  });

  it('shows a retryable fallback when the aggregate request fails', async () => {
    const user = userEvent.setup();
    let unavailable = true;
    server.use(
      http.get('/api/v1/me/knowledge', () => {
        if (unavailable) {
          return HttpResponse.json({ error: 'unavailable' }, { status: 503 });
        }
        return HttpResponse.json({
          results: [{ account: 'testuser', data: [store('user-store')] }],
          failed_accounts: [],
          rejected_accounts: [],
        });
      }),
    );

    renderKnowledge();

    expect(await screen.findByText("Couldn't load knowledge stores")).toBeInTheDocument();
    unavailable = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByText('user-store')).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load knowledge stores")).not.toBeInTheDocument();
  });
});
