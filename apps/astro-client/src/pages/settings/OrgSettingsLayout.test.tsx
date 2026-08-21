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

  it('shows Experiments after Audit Log for admins and hides it from members', async () => {
    renderLayout('admin');
    await waitFor(() => expect(screen.getAllByText('Experiments').length).toBeGreaterThan(0));

    const navText = screen.getAllByRole('link').map((link) => link.textContent?.trim());
    expect(navText.indexOf('Experiments')).toBe(navText.indexOf('Audit Log') + 1);

    cleanup();
    renderLayout('member');
    await waitFor(() => expect(screen.getAllByText('General').length).toBeGreaterThan(0));
    expect(screen.queryAllByText('Experiments')).toHaveLength(0);
  });

  it('hides Billing nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('General').length).toBeGreaterThan(0);
    });
    expect(screen.queryAllByText('Billing')).toHaveLength(0);
  });

  it('always shows General and Members nav for member', async () => {
    renderLayout('member');
    await waitFor(() => {
      expect(screen.getAllByText('General').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Members').length).toBeGreaterThan(0);
    });
  });

  it('keeps long organization names inside the settings sidebar', async () => {
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
    expect(name).toHaveClass('min-w-0', 'max-w-full', 'hyphens-auto');
    expect(name.className).toContain('[overflow-wrap:anywhere]');
    expect(name.closest('h1')).toHaveClass('max-w-full');
  });
});
