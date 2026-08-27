import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, afterEach } from 'vitest';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import type { AuthContextType } from '@/lib/auth-context';
import { SidebarNavItem } from '@/components/ui/sidebar-layout';
import { SettingsSidebar } from './SettingsSidebar';

afterEach(cleanup);

const auth: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal', display_name: 'Test User' },
    { id: 'org-1', name: 'acme', type: 'organization', display_name: 'Acme', organization_id: 'wos-1' },
  ],
};

function renderSidebar(account: string, path: string) {
  return renderRoute(
    [
      {
        path: '/settings/*',
        Component: () => (
          <SettingsSidebar account={account}>
            <SidebarNavItem to="/settings/billing">Billing</SidebarNavItem>
          </SettingsSidebar>
        ),
      },
      { path: '/settings/org/:orgSlug/billing', Component: () => <div>Org Billing Page</div> },
      { path: '/settings/account', Component: () => <div>Personal Account Page</div> },
    ],
    { initialEntries: [path], auth },
  );
}

describe('SettingsSidebar', () => {
  it('shows the current scope in the selector', async () => {
    renderSidebar('acme', '/settings/org/acme/usage');
    await waitFor(() => expect(screen.getByLabelText('Settings scope')).toBeInTheDocument());
    expect(screen.getByLabelText('Settings scope')).toHaveTextContent('Acme');
  });

  it('carries the current section across a scope switch', async () => {
    const user = userEvent.setup();
    renderSidebar('testuser', '/settings/billing');

    await user.click(await screen.findByLabelText('Settings scope'));
    await user.click(await screen.findByRole('option', { name: /Acme/ }));

    await waitFor(() => expect(screen.getByText('Org Billing Page')).toBeInTheDocument());
  });

  it('falls back to the landing section when the counterpart is missing', async () => {
    const user = userEvent.setup();
    renderSidebar('acme', '/settings/org/acme/members');

    await user.click(await screen.findByLabelText('Settings scope'));
    await user.click(await screen.findByRole('option', { name: /Test User/ }));

    await waitFor(() => expect(screen.getByText('Personal Account Page')).toBeInTheDocument());
  });
});
