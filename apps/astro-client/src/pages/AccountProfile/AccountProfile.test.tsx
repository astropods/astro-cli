import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AccountProfile from './AccountProfile';
import type { AccountPublic, AccountOrgsResponse, BlueprintsListResponse } from '@/lib/api';

afterEach(cleanup);

// ── Fixture data ────────────────────────────────────────────────────────────

const mockAccount: AccountPublic = {
  id: 'acct-1',
  name: 'testuser',
  type: 'personal',
  display_name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

const otherAccount: AccountPublic = {
  id: 'acct-2',
  name: 'otheruser',
  type: 'personal',
  display_name: 'Other User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

const publicBlueprint = {
  name: 'public-bot',
  account: 'testuser',
  registry: 'reg.example.com',
  visibility: 'public' as const,
  versions: [],
};

const privateBlueprint = {
  name: 'secret-bot',
  account: 'testuser',
  registry: 'reg.example.com',
  visibility: 'private' as const,
  versions: [],
};

// ── Render helper ────────────────────────────────────────────────────────────

function renderProfile(
  path = '/testuser',
  {
    auth = mockAuthContext,
    loaderAccount = mockAccount as AccountPublic | null,
  }: {
    auth?: typeof mockAuthContext;
    loaderAccount?: AccountPublic | null;
  } = {},
) {
  return renderRoute(
    [
      {
        path: '/:account',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Component: AccountProfile as any,
        loader: () => ({
          account: loaderAccount,
          blueprints: null,
          orgs: null,
          deployments: null,
        }),
      },
    ],
    { initialEntries: [path], auth },
  );
}

// ── Loading / not found ──────────────────────────────────────────────────────

describe('AccountProfile loading and error states', () => {
  it('shows "Account not found" immediately when the loader provides no account', async () => {
    // loaderAccount: null means SSR could not resolve the account.
    // The component renders "not found" without waiting for a client fetch.
    renderProfile('/testuser', { loaderAccount: null });

    await waitFor(() => {
      expect(screen.getByText('Account not found')).toBeInTheDocument();
    });
  });

  it('shows "Account not found" when the account does not exist', async () => {
    server.use(
      http.get('/api/v1/accounts/:account', () =>
        HttpResponse.json({ error: 'not_found' }, { status: 404 }),
      ),
    );

    renderProfile('/nobody', { loaderAccount: null });

    await waitFor(() => {
      expect(screen.getByText('Account not found')).toBeInTheDocument();
    });
  });
});

// ── Owner vs visitor view mode ───────────────────────────────────────────────

describe('AccountProfile view mode toggle', () => {
  it('shows the "View as visitor" button for the owner', async () => {
    renderProfile('/testuser');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /view as visitor/i })).toBeInTheDocument();
    });
  });

  it('hides the "View as visitor" button for a visitor', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json(otherAccount),
      ),
    );

    renderProfile('/otheruser');

    await waitFor(() => {
      // Wait for the profile to load (sidebar shows display name)
      expect(screen.getByRole('heading', { name: 'Other User' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /view as visitor/i })).not.toBeInTheDocument();
  });

  it('shows the Agents tab for the owner in internal view', async () => {
    renderProfile('/testuser');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument();
    });
  });

  it('hides the Agents tab for a visitor', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json(otherAccount),
      ),
    );

    renderProfile('/otheruser');

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Other User' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /^agents/i })).not.toBeInTheDocument();
  });
});

// ── Blueprint visibility gating ──────────────────────────────────────────────

describe('AccountProfile blueprint visibility', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/agents/:account', () =>
        HttpResponse.json<BlueprintsListResponse>({
          agents: [publicBlueprint, privateBlueprint],
          count: 2,
        }),
      ),
    );
  });

  it('owner sees both public and private blueprints', async () => {
    renderProfile('/testuser');

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'public-bot' })).toBeInTheDocument();
      // secret-bot has a PrivacyBadge inside the h3, so accessible name is "secret-bot Private"
      expect(screen.getByRole('heading', { name: /secret-bot/ })).toBeInTheDocument();
    });
  });

  it('visitor sees only public blueprints', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json({ ...otherAccount, name: 'otheruser' } satisfies AccountPublic),
      ),
    );
    // Blueprints belong to otheruser's account
    server.use(
      http.get('/api/v1/agents/otheruser', () =>
        HttpResponse.json<BlueprintsListResponse>({
          agents: [
            { ...publicBlueprint, account: 'otheruser' },
            { ...privateBlueprint, account: 'otheruser' },
          ],
          count: 2,
        }),
      ),
    );

    renderProfile('/otheruser');

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'public-bot' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('heading', { name: /secret-bot/ })).not.toBeInTheDocument();
  });
});

// ── Edit profile button ──────────────────────────────────────────────────────

describe('AccountProfile edit profile button', () => {
  it('shows "Edit profile" button for the owner', async () => {
    renderProfile('/testuser');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /edit profile/i })).toBeInTheDocument();
    });
  });

  it('hides "Edit profile" button for a visitor', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json(otherAccount),
      ),
    );

    renderProfile('/otheruser');

    await waitFor(() => {
      // Wait for the profile to finish loading (sidebar heading present)
      expect(screen.getByRole('heading', { name: 'Other User' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /edit profile/i })).not.toBeInTheDocument();
  });
});

// ── Agents stat ──────────────────────────────────────────────────────────────

describe('AccountProfile agents stat', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ deployments: [{ id: 'd1', name: 'bot', status: 'Running', replicas: 1, ready: 1, build_id: 'b1', namespace: 'ns1', created_at: '2025-01-01T00:00:00Z', components: [] }], count: 1 }),
      ),
    );
  });

  it('shows the Agents stat for the owner', async () => {
    renderProfile('/testuser');

    await waitFor(() => {
      // sidebar stat label for owner
      expect(screen.getAllByText('Agents').length).toBeGreaterThan(0);
    });
  });

  it('hides the Agents stat for a visitor', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json(otherAccount),
      ),
    );

    renderProfile('/otheruser');

    await waitFor(() => {
      // Wait for the profile to finish loading (sidebar heading present)
      expect(screen.getByRole('heading', { name: 'Other User' })).toBeInTheDocument();
    });
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});

// ── Org navigation ───────────────────────────────────────────────────────────

describe('AccountProfile organization links', () => {
  it('renders org links using React Router <Link> (no full-page reload)', async () => {
    server.use(
      http.get('/api/v1/accounts/testuser/orgs', () =>
        HttpResponse.json<AccountOrgsResponse>({
          orgs: [{ name: 'acme-corp', display_name: 'Acme Corp' }],
        }),
      ),
    );

    renderProfile('/testuser');

    await waitFor(() => {
      const orgLink = screen.getByTitle('Acme Corp');
      // React Router <Link> renders as <a> with an href that does not trigger full reload
      expect(orgLink.closest('a')).toHaveAttribute('href', '/acme-corp');
    });
  });
});
