import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import userEvent from '@testing-library/user-event';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import { server } from '@/test/msw/server';
import type { AuthContextType } from '@/lib/auth-context';
import OrgGeneralSettings from './OrgGeneralSettings';

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

function renderPage(role: string) {
  return renderRoute(
    [{ path: '/settings/org/:orgSlug/general', Component: OrgGeneralSettings }],
    {
      initialEntries: [`/settings/org/${ORG_SLUG}/general`],
      auth: makeAuth(role),
    },
  );
}

beforeEach(() => {
  server.use(
    http.get('*/api/v1/accounts/:account/members', () => {
      return HttpResponse.json({ members: [] });
    }),
  );
});

describe('OrgGeneralSettings', () => {
  describe('profile section', () => {
    it('admin can edit display name', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByDisplayValue('Test Org')).toBeInTheDocument();
      });
      const input = screen.getByDisplayValue('Test Org');
      expect(input).not.toBeDisabled();
    });

    it('member sees read-only display name', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByDisplayValue('Test Org')).toBeInTheDocument();
      });
      const input = screen.getByDisplayValue('Test Org');
      expect(input).toBeDisabled();
    });

    it('patches and refreshes auth account data after saving a display name', async () => {
      const user = userEvent.setup();
      const patchAccount = vi.fn();
      const refreshUserData = vi.fn().mockResolvedValue(undefined);

      server.use(
        http.patch('/api/v1/accounts/:account', () =>
          HttpResponse.json({ message: 'profile updated' }),
        ),
      );

      renderRoute(
        [{ path: '/settings/org/:orgSlug/general', Component: OrgGeneralSettings }],
        {
          initialEntries: [`/settings/org/${ORG_SLUG}/general`],
          auth: {
            ...makeAuth('admin'),
            patchAccount,
            refreshUserData,
          },
        },
      );

      await user.clear(await screen.findByDisplayValue('Test Org'));
      await user.type(screen.getByPlaceholderText('Add a display name'), 'New Org');
      await user.click(screen.getByRole('button', { name: /^save$/i }));

      await waitFor(() => {
        expect(patchAccount).toHaveBeenCalledWith(ORG_SLUG, { display_name: 'New Org' });
      });
      expect(refreshUserData).toHaveBeenCalledOnce();
    });
  });

  describe('username section', () => {
    it('admin can click Change username', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Change username')).toBeInTheDocument();
      });
      const button = screen.getByText('Change username').closest('button')!;
      expect(button).not.toBeDisabled();
    });

    it('member sees disabled Change username', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Change username')).toBeInTheDocument();
      });
      const button = screen.getByText('Change username').closest('button')!;
      expect(button).toBeDisabled();
    });
  });

  describe('danger zone', () => {
    it('delete button enabled for admin', async () => {
      renderPage('admin');
      await waitFor(() => {
        expect(screen.getByText('Delete organization')).toBeInTheDocument();
      });
      const deleteButton = screen.getByRole('button', { name: 'Delete' });
      expect(deleteButton).not.toBeDisabled();
    });

    it('delete button disabled for member', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Delete organization')).toBeInTheDocument();
      });
      const deleteButton = screen.getByRole('button', { name: 'Delete' });
      expect(deleteButton).toBeDisabled();
    });

    it('leave button always visible for member', async () => {
      renderPage('member');
      await waitFor(() => {
        expect(screen.getByText('Leave organization')).toBeInTheDocument();
      });
      const leaveButton = screen.getByRole('button', { name: 'Leave' });
      expect(leaveButton).not.toBeDisabled();
    });
  });
});
