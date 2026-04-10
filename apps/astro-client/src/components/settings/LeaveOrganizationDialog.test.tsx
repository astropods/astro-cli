import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import type { AuthContextType } from '@/lib/auth-context';
import { LeaveOrganizationDialog } from './LeaveOrganizationDialog';

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

function useMembersMock(members: Array<{ user_id: string; role: string; status: string; username: string; display_name: string }>) {
  server.use(
    http.get('*/api/v1/accounts/:account/members', () => {
      return HttpResponse.json({
        members: members.map((m) => ({
          account_id: 'org-1',
          created_at: '2025-01-01T00:00:00Z',
          ...m,
        })),
      });
    }),
  );
}

function renderDialog(role: string) {
  // Render the dialog in open state wrapped in a minimal route
  return renderRoute(
    [
      {
        path: '/',
        Component: () => (
          <LeaveOrganizationDialog orgSlug={ORG_SLUG} open onOpenChange={() => {}} />
        ),
      },
    ],
    {
      initialEntries: ['/'],
      auth: makeAuth(role),
    },
  );
}

describe('LeaveOrganizationDialog', () => {
  it('sole member cannot leave — button is disabled', async () => {
    useMembersMock([
      { user_id: 'user-1', role: 'admin', status: 'active', username: 'testuser', display_name: 'Test User' },
    ]);
    renderDialog('admin');

    await waitFor(() => {
      expect(screen.getByText(/you are the only member/i)).toBeInTheDocument();
    });

    const leaveButton = screen.getByRole('button', { name: /leave/i });
    expect(leaveButton).toBeDisabled();
  });

  it('last admin must promote someone before leaving', async () => {
    useMembersMock([
      { user_id: 'user-1', role: 'admin', status: 'active', username: 'testuser', display_name: 'Test User' },
      { user_id: 'user-2', role: 'member', status: 'active', username: 'otheruser', display_name: 'Other User' },
    ]);
    renderDialog('admin');

    await waitFor(() => {
      expect(screen.getByText(/last admin/i)).toBeInTheDocument();
    });

    // Should show the member selection UI (New admin label)
    expect(screen.getByText('New admin')).toBeInTheDocument();
  });

  it('non-admin can leave directly', async () => {
    useMembersMock([
      { user_id: 'user-1', role: 'member', status: 'active', username: 'testuser', display_name: 'Test User' },
      { user_id: 'user-2', role: 'admin', status: 'active', username: 'adminuser', display_name: 'Admin User' },
    ]);
    renderDialog('member');

    await waitFor(() => {
      expect(screen.getByText(/are you sure you want to leave/i)).toBeInTheDocument();
    });

    // Should NOT show admin promotion UI
    expect(screen.queryByText('New admin')).not.toBeInTheDocument();
  });

  it('admin with other admins can leave directly', async () => {
    useMembersMock([
      { user_id: 'user-1', role: 'admin', status: 'active', username: 'testuser', display_name: 'Test User' },
      { user_id: 'user-2', role: 'admin', status: 'active', username: 'otheradmin', display_name: 'Other Admin' },
    ]);
    renderDialog('admin');

    await waitFor(() => {
      expect(screen.getByText(/are you sure you want to leave/i)).toBeInTheDocument();
    });

    // Should NOT show admin promotion UI since there's another admin
    expect(screen.queryByText('New admin')).not.toBeInTheDocument();
  });
});
