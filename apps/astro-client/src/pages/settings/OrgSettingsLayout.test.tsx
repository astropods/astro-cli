import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach } from 'vitest';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import type { AuthContextType } from '@/lib/auth-context';
import OrgSettingsLayout from './OrgSettingsLayout';

afterEach(cleanup);

const ORG_SLUG = 'test-org';

const orgAccount = {
  id: 'org-1',
  name: ORG_SLUG,
  type: 'organization' as const,
  display_name: 'Test Org',
  organization_id: 'wos-org-1',
};

const makeAuth = (role: string): AuthContextType => ({
  ...mockAuthContext,
  role,
  organizationId: 'wos-org-1',
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' },
    orgAccount,
  ],
});

function renderLayout(role: string) {
  return renderRoute(
    [
      {
        path: '/settings/org/:orgSlug',
        Component: OrgSettingsLayout,
        children: [
          { path: 'general', Component: () => <div>General Page</div> },
          { path: 'members', Component: () => <div>Members Page</div> },
          { path: 'secrets', Component: () => <div>Secrets Page</div> },
        ],
      },
    ],
    {
      initialEntries: [`/settings/org/${ORG_SLUG}/general`],
      auth: makeAuth(role),
    },
  );
}

describe('OrgSettingsLayout', () => {
  it('shows Secrets & Variables nav for admin', async () => {
    renderLayout('admin');
    await waitFor(() => {
      expect(screen.getAllByText('Secrets & Variables').length).toBeGreaterThan(0);
    });
  });

  it('shows Secrets & Variables nav for owner', async () => {
    renderLayout('owner');
    await waitFor(() => {
      expect(screen.getAllByText('Secrets & Variables').length).toBeGreaterThan(0);
    });
  });

  it('hides Secrets & Variables nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      // General nav is always present — use it to confirm the layout rendered
      expect(screen.getAllByText('General').length).toBeGreaterThan(0);
    });
    expect(screen.queryAllByText('Secrets & Variables')).toHaveLength(0);
  });

  it('always shows General and Members nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('General').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Members').length).toBeGreaterThan(0);
    });
  });

  it('shows EU badge when org cluster_id is eu', async () => {
    const auth: AuthContextType = {
      ...makeAuth('admin'),
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { ...orgAccount, cluster_id: 'eu' },
      ],
    };
    renderRoute(
      [
        {
          path: '/settings/org/:orgSlug',
          Component: OrgSettingsLayout,
          children: [{ path: 'general', Component: () => <div>General Page</div> }],
        },
      ],
      {
        initialEntries: [`/settings/org/${ORG_SLUG}/general`],
        auth,
      },
    );
    await waitFor(() => {
      expect(screen.getByText('EU')).toBeInTheDocument();
    });
  });

  it('does not show EU badge for primary-cluster orgs', async () => {
    renderLayout('admin');
    await waitFor(() => {
      expect(screen.getByText('Test Org')).toBeInTheDocument();
    });
    expect(screen.queryByText('EU')).not.toBeInTheDocument();
  });
});
