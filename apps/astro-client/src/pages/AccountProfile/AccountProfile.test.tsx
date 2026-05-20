import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import AccountProfile from './AccountProfile';
import type { AccountPublic, AccountOrgsResponse, BlueprintsListResponse, AccountMembersResponse } from '@/lib/api';

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

const mockOrg: AccountPublic = {
  id: 'acct-org',
  name: 'testorg',
  type: 'organization',
  display_name: 'Test Org',
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
        // Mirror the real loader shape (see AccountProfile.tsx loader) so
        // usePrimeQueryCache runs against realistic data — otherwise the
        // cache-priming code path stays untested in component tests.
        loader: () => ({
          account: loaderAccount,
          orgs: null,
          members: null,
          blueprints: null,
          deployments: null,
          hearted: null,
        }),
      },
    ],
    { initialEntries: [path], auth },
  );
}

function renderOrgProfile(loaderAccount: AccountPublic = mockOrg) {
  return renderProfile(`/${loaderAccount.name}`, { loaderAccount });
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
      // Rendered as <Button asChild><Link> so the DOM element is an <a>, role="link"
      expect(screen.getByRole('link', { name: /view as visitor/i })).toBeInTheDocument();
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
      // isDraft=true (versions:[]) means heading is "public-bot Finish setup" — match by prefix
      expect(screen.getByRole('heading', { name: /^public-bot/ })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: /^secret-bot/ })).toBeInTheDocument();
    });
  });

  it('visitor sees only public blueprints', async () => {
    server.use(
      http.get('/api/v1/accounts/otheruser', () =>
        HttpResponse.json({ ...otherAccount, name: 'otheruser' } satisfies AccountPublic),
      ),
    );
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
      expect(screen.getByRole('heading', { name: /^public-bot/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole('heading', { name: /^secret-bot/ })).not.toBeInTheDocument();
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

  // Non-member visitor: canViewDeployments is false in AccountProfile's component
  // scope, so isInternalView is false at the SidebarRenderOpts boundary and the
  // sidebar would hide Agents regardless. Guards the default (non-member) path.
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

  // Owner with ?visitor: canViewDeployments is true in AccountProfile's scope
  // (isSelf=true) but ProfileLayout's isInternalView is false. Without the
  // SidebarRenderOpts.isInternalView plumbing, the sidebar would render Agents
  // using the stale component-scope flag. Guards the visitor-mode gating path.
  it('hides the Agents stat when the owner views as visitor (?visitor)', async () => {
    renderProfile('/testuser?visitor');

    await waitFor(() => {
      // Wait for the profile to finish loading (sidebar heading present)
      expect(screen.getByRole('heading', { name: 'Test User' })).toBeInTheDocument();
    });
    expect(screen.queryByText('Agents')).not.toBeInTheDocument();
  });
});

// ── Org admin permissions ────────────────────────────────────────────────────

// mockAuthContext.user.id = 'user-1' — the same ID used in the admin member fixture below.
// mockAuthContext.accounts contains only 'acct-1' (personal), so isSelf = false for the org.
// isOrgAdmin becomes true when the members API returns user-1 with role=admin.

const orgAdminMember = {
  account_id: 'acct-org',
  user_id: 'user-1',
  role: 'admin',
  status: 'active',
  username: 'testuser',
  display_name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  slack_workspaces: [],
};

const orgMemberBlueprint = {
  name: 'org-bot',
  account: 'testorg',
  registry: 'reg.example.com',
  visibility: 'public' as const,
  versions: [{ build_id: 'b1', published_at: '2025-01-01T00:00:00Z', spec: {} }],
};

describe('AccountProfile org admin', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/accounts/testorg', () => HttpResponse.json<AccountPublic>(mockOrg)),
      http.get('/api/v1/accounts/testorg/members', () =>
        HttpResponse.json<AccountMembersResponse>({ members: [orgAdminMember] }),
      ),
      http.get('/api/v1/agents/testorg', () =>
        HttpResponse.json<BlueprintsListResponse>({ agents: [orgMemberBlueprint], count: 1 }),
      ),
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ deployments: [], count: 0 }),
      ),
    );
  });

  it('shows the Customize Order button for an org admin', async () => {
    renderOrgProfile();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /customize order/i })).toBeInTheDocument();
    });
  });

  it('shows the Agents tab for an org admin', async () => {
    renderOrgProfile();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument();
    });
  });

  it('loads and renders deployments for an org admin', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({
          deployments: [{
            id: 'd-org-1',
            name: 'org-agent',
            display_name: 'Org Agent',
            status: 'Running',
            replicas: 1,
            ready: 1,
            build_id: 'b1',
            namespace: 'ns1',
            created_at: '2025-01-01T00:00:00Z',
            components: [],
          }],
          count: 1,
        }),
      ),
    );

    renderOrgProfile();

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /^agents/i }));

    await waitFor(() => {
      expect(screen.getByText('Org Agent')).toBeInTheDocument();
    });
  });

  it('shows Agents tab but hides Customize Order for a non-admin org member', async () => {
    server.use(
      http.get('/api/v1/accounts/testorg/members', () =>
        HttpResponse.json<AccountMembersResponse>({
          members: [{ ...orgAdminMember, role: 'member' }],
        }),
      ),
    );

    renderOrgProfile();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^agents/i })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /customize order/i })).not.toBeInTheDocument();
  });

  it('hides Agents tab for a visitor (non-member) of an org', async () => {
    server.use(
      http.get('/api/v1/accounts/testorg/members', () =>
        HttpResponse.json<AccountMembersResponse>({ members: [] }),
      ),
    );

    // Render as a different user who is not a member
    renderProfile('/testorg', {
      auth: { ...mockAuthContext, accounts: [] },
      loaderAccount: mockOrg,
    });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Org' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /^agents/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /customize order/i })).not.toBeInTheDocument();
  });

  // Members API requires membership (server-side), so non-members see an empty list
  // and the Members section hides naturally. The owner/admin in ?visitor mode is
  // still a member server-side, so the API returns data; the UI must hide the
  // section explicitly to match what a true non-member would see.
  it('hides the Members section when the org admin views as visitor (?visitor)', async () => {
    renderProfile('/testorg?visitor', { loaderAccount: mockOrg });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Org' })).toBeInTheDocument();
    });
    expect(screen.queryByText('Members')).not.toBeInTheDocument();
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
      // React Router <Link> renders as <a> with an href that does not trigger full reload
      expect(screen.getByRole('link', { name: /acme corp/i })).toHaveAttribute('href', '/acme-corp');
    });
  });
});
