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

const makeAuth = (role: string, org = orgAccount): AuthContextType => ({
  ...mockAuthContext,
  role,
  organizationId: org.organization_id,
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' },
    org,
  ],
});

function renderLayout(role: string, org = orgAccount) {
  return renderRoute(
    [
      {
        path: '/settings/org/:orgSlug',
        Component: OrgSettingsLayout,
        children: [
          { path: 'general', Component: () => <div>General Page</div> },
          { path: 'members', Component: () => <div>Members Page</div> },
          { path: 'secrets', Component: () => <div>Secrets Page</div> },
          { path: 'experiments', Component: () => <div>Experiments Page</div> },
        ],
      },
    ],
    {
      initialEntries: [`/settings/org/${org.name}/general`],
      auth: makeAuth(role, org),
    },
  );
}

describe('OrgSettingsLayout', () => {
  it('shows Variables & Secrets nav for admin', async () => {
    renderLayout('admin');
    await waitFor(() => {
      expect(screen.getAllByText('Variables & Secrets').length).toBeGreaterThan(0);
    });
  });

  it('shows Variables & Secrets nav for owner', async () => {
    renderLayout('owner');
    await waitFor(() => {
      expect(screen.getAllByText('Variables & Secrets').length).toBeGreaterThan(0);
    });
  });

  it('hides Variables & Secrets nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      // Account nav is always present — use it to confirm the layout rendered
      expect(screen.getAllByText('Account').length).toBeGreaterThan(0);
    });
    expect(screen.queryAllByText('Variables & Secrets')).toHaveLength(0);
  });

  it('shows Billing nav for admin and owner', async () => {
    renderLayout('admin');
    await waitFor(() => {
      expect(screen.getAllByText('Billing').length).toBeGreaterThan(0);
    });
    cleanup();
    renderLayout('owner');
    await waitFor(() => {
      expect(screen.getAllByText('Billing').length).toBeGreaterThan(0);
    });
  });

  it('shows Usage nav for admin and owner, hides it from members', async () => {
    renderLayout('admin');
    await waitFor(() => {
      expect(screen.getAllByText('Usage').length).toBeGreaterThan(0);
    });
    cleanup();
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('Account').length).toBeGreaterThan(0);
    });
    expect(screen.queryAllByText('Usage')).toHaveLength(0);
  });

  it('shows Experiments last for admins and hides it from members', async () => {
    renderLayout('admin');
    await waitFor(() => expect(screen.getAllByText('Experiments').length).toBeGreaterThan(0));

    const navText = screen.getAllByRole('link').map((link) => link.textContent?.trim());
    expect(navText[navText.length - 1]).toBe('ExperimentsBeta');

    cleanup();
    renderLayout('member');
    await waitFor(() => expect(screen.getAllByText('Account').length).toBeGreaterThan(0));
    expect(screen.queryAllByText('Experiments')).toHaveLength(0);
  });

  it('hides Billing nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('Account').length).toBeGreaterThan(0);
    });
    expect(screen.queryAllByText('Billing')).toHaveLength(0);
  });

  it('always shows Account and Members nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('Account').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Members').length).toBeGreaterThan(0);
    });
  });

  it('groups nav items under section labels', async () => {
    renderLayout('admin');
    await waitFor(() => expect(screen.getAllByText('Manage').length).toBeGreaterThan(0));
    expect(screen.getAllByText('Access').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Integrations').length).toBeGreaterThan(0);
  });

  it('omits Connectors, which has no org-scoped route', async () => {
    renderLayout('admin');
    await waitFor(() => expect(screen.getAllByText('Integrations').length).toBeGreaterThan(0));
    expect(screen.queryAllByText('Connectors')).toHaveLength(0);
  });

  it('names the org in the scope selector and keeps long names inside it', async () => {
    const longName = 'hereisaninsanelylongorgnamehereisaninsanelylongorgnamehereisanin';
    renderLayout('admin', {
      ...orgAccount,
      name: longName,
      display_name: longName,
    });

    await waitFor(() => {
      expect(screen.getByText(longName)).toBeInTheDocument();
    });

    const name = screen.getByText(longName);
    expect(name).toHaveClass('truncate');
    expect(name.closest('[aria-label="Settings scope"]')).toBeInTheDocument();
  });
});
