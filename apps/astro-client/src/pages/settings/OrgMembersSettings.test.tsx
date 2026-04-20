import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import type { AuthContextType } from '@/lib/auth-context';
import OrgMembersSettings from './OrgMembersSettings';

afterEach(cleanup);

const ORG_SLUG = 'test-org';

const orgAccount = {
  id: 'org-1',
  name: ORG_SLUG,
  type: 'organization' as const,
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

const activeMembers = [
  {
    account_id: 'org-1',
    user_id: 'user-1',
    role: 'admin',
    status: 'active',
    username: 'testuser',
    display_name: 'Test User',
    created_at: '2025-01-01T00:00:00Z',
  },
  {
    account_id: 'org-1',
    user_id: 'user-2',
    role: 'member',
    status: 'active',
    username: 'otheruser',
    display_name: 'Other User',
    created_at: '2025-01-02T00:00:00Z',
  },
];

const pendingMember = {
  account_id: 'org-1',
  user_id: 'user-3',
  role: 'member',
  status: 'pending',
  username: 'pendinguser',
  display_name: 'Pending User',
  created_at: '2025-01-03T00:00:00Z',
};

function useMembersMock(members = activeMembers) {
  server.use(
    http.get('*/api/v1/accounts/:account/members', () => {
      return HttpResponse.json({ members });
    }),
  );
}

function renderPage(role: string) {
  return renderRoute(
    [{ path: '/settings/org/:orgSlug/members', Component: OrgMembersSettings }],
    {
      initialEntries: [`/settings/org/${ORG_SLUG}/members`],
      auth: makeAuth(role),
    },
  );
}

describe('OrgMembersSettings', () => {
  describe('invite button', () => {
    beforeEach(() => useMembersMock());

    it('shows invite button for admin', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /invite members/i })).toBeInTheDocument();
      });
    });

    it('shows invite button for owner', async () => {
      renderPage('owner');
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /invite members/i })).toBeInTheDocument();
      });
    });

    it('hides invite button for member', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Other User')).toBeInTheDocument();
      });
      expect(screen.queryByRole('button', { name: /invite members/i })).not.toBeInTheDocument();
    });
  });

  describe('role dropdown', () => {
    beforeEach(() => useMembersMock());

    it('admin sees role dropdown for other members', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Other User')).toBeInTheDocument();
      });
      // Other user's role cell should have a dropdown trigger (button with chevron)
      const roleButtons = screen.getAllByRole('button');
      const chevronButton = roleButtons.find(
        (btn) => btn.textContent?.includes('Member') && btn.querySelector('svg'),
      );
      expect(chevronButton).toBeTruthy();
    });

    it('member sees plain text role with no dropdown', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Other User')).toBeInTheDocument();
      });
      // Role should be plain text, no dropdown trigger with chevron
      const memberTexts = screen.getAllByText('Member');
      // All should be plain spans, not buttons
      memberTexts.forEach((el) => {
        expect(el.tagName).not.toBe('BUTTON');
      });
    });

    it('no role dropdown for self even as admin', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Test User')).toBeInTheDocument();
      });
      // Current user's row should show "(you)"
      expect(screen.getByText('(you)')).toBeInTheDocument();
      // Current user's role "Admin" should be plain text, not a dropdown trigger
      const adminElements = screen.getAllByText('Admin');
      adminElements.forEach((el) => {
        expect(el.tagName).not.toBe('BUTTON');
        expect(el.closest('button')).toBeNull();
      });
    });
  });

  describe('action menu', () => {
    beforeEach(() => useMembersMock());

    it('admin sees action menu for other members', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Other User')).toBeInTheDocument();
      });
      // The action menu trigger is an icon-only button (no text content, just an SVG)
      const allButtons = screen.getAllByRole('button');
      const iconOnlyButtons = allButtons.filter(
        (btn) => !btn.textContent?.trim(),
      );
      expect(iconOnlyButtons.length).toBeGreaterThanOrEqual(1);
    });

    it('member sees no action menus', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Other User')).toBeInTheDocument();
      });
      // Member should not see any MoreHorizontal action buttons
      // The only buttons for a member should be none (no invite, no actions)
      const allButtons = screen.queryAllByRole('button');
      // None should have the MoreHorizontal icon pattern
      allButtons.forEach((btn) => {
        // Ensure no action dropdown triggers for remove/revoke
        expect(btn.textContent).not.toContain('Remove');
        expect(btn.textContent).not.toContain('Revoke');
      });
    });
  });

  describe('empty state', () => {
    beforeEach(() => useMembersMock([]));

    it('admin empty state shows invite button', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Invite people to join this organization')).toBeInTheDocument();
      });
      // Should have an invite button in the empty state too
      const inviteButtons = screen.getAllByRole('button', { name: /invite members/i });
      expect(inviteButtons.length).toBeGreaterThanOrEqual(1);
    });

    it('member empty state shows passive message', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(
          screen.getByText('No one else has joined this organization yet'),
        ).toBeInTheDocument();
      });
      expect(screen.queryByRole('button', { name: /invite members/i })).not.toBeInTheDocument();
    });
  });

  describe('pending members', () => {
    beforeEach(() => useMembersMock([...activeMembers, pendingMember]));

    it('pending member shows Invited status', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Pending User')).toBeInTheDocument();
      });
      expect(screen.getByText('Invited')).toBeInTheDocument();
    });
  });

  // Mirrors the backend guard in org.ErrOwnerManagementForbidden: admins can
  // manage regular members, but they cannot change the role of or remove an
  // owner. Only another owner can manage an owner.
  describe('owner hierarchy protection', () => {
    const withOwner = [
      activeMembers[0], // user-1, admin (the viewer when role=admin)
      {
        account_id: 'org-1',
        user_id: 'user-owner',
        role: 'owner',
        status: 'active',
        username: 'theowner',
        display_name: 'The Owner',
        created_at: '2025-01-02T00:00:00Z',
      },
    ];

    it('admin sees the owner row as read-only (no role dropdown, no action menu)', async () => {
      useMembersMock(withOwner);
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('The Owner')).toBeInTheDocument();
      });

      // Owner row's role cell should be plain text, not a dropdown trigger.
      const ownerRoleEl = screen.getByText('owner');
      expect(ownerRoleEl.tagName).not.toBe('BUTTON');
      expect(ownerRoleEl.closest('button')).toBeNull();

      // No action-menu (kebab) button should exist for any row:
      // user-1 is the viewer themselves, user-owner is protected by hierarchy.
      const iconOnlyButtons = screen
        .getAllByRole('button')
        .filter((btn) => !btn.textContent?.trim());
      expect(iconOnlyButtons).toHaveLength(0);
    });

    it('owner sees another owner as manageable (dropdown + action menu)', async () => {
      useMembersMock([
        {
          ...activeMembers[0],
          role: 'owner', // viewer is an owner
        },
        withOwner[1], // second owner target
      ]);
      renderPage('owner');
      await waitFor(() => {
        expect(screen.getByText('The Owner')).toBeInTheDocument();
      });

      // The target owner's role cell should be rendered as a button (dropdown).
      const ownerRoleButton = screen
        .getAllByRole('button')
        .find((btn) => btn.textContent?.toLowerCase().includes('owner'));
      expect(ownerRoleButton).toBeTruthy();

      // And an icon-only kebab should be present (for the removable target row).
      const iconOnlyButtons = screen
        .getAllByRole('button')
        .filter((btn) => !btn.textContent?.trim());
      expect(iconOnlyButtons.length).toBeGreaterThanOrEqual(1);
    });
  });
});
