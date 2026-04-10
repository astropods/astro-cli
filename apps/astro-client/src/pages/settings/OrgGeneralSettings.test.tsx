import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
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
