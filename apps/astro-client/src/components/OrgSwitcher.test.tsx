import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { AuthContext } from '@/lib/auth-context';
import { mockAuthContext } from '@/test/test-utils';
import { OrgSwitcher } from './OrgSwitcher';

afterEach(cleanup);

const personal = { id: 'acct-1', name: 'testuser', type: 'personal' as const };
const org = { id: 'org-1', name: 'my-org', type: 'organization' as const };
const personalWithDisplay = { id: 'acct-2', name: 'janedoe', display_name: 'Jane Doe', type: 'personal' as const };
const orgWithDisplay = { id: 'org-2', name: 'acme-corp', display_name: 'Acme Corp', type: 'organization' as const };

function renderSwitcher({
  activeAccount = 'testuser',
  onChange = vi.fn(),
  accounts = [personal, org],
}: {
  activeAccount?: string;
  onChange?: (account: string) => void;
  accounts?: { id: string; name: string; type: string; display_name?: string }[];
} = {}) {
  render(
    <AuthContext.Provider value={{ ...mockAuthContext, accounts }}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter>
          <OrgSwitcher
            activeAccount={activeAccount}
            onChange={onChange}
          />
        </MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>
  );
  return { onChange };
}

const triggerName = /switch account/i;

describe('OrgSwitcher', () => {
  describe('trigger', () => {
    it('shows the active account name', () => {
      renderSwitcher({ activeAccount: 'testuser' });
      expect(screen.getByText('testuser')).toBeInTheDocument();
    });

    it('shows the active org name', () => {
      renderSwitcher({ activeAccount: 'my-org' });
      expect(screen.getByText('my-org')).toBeInTheDocument();
    });

    it('prefers display_name over name for personal accounts', () => {
      renderSwitcher({ activeAccount: 'janedoe', accounts: [personalWithDisplay, org] });
      expect(screen.getByText('Jane Doe')).toBeInTheDocument();
      expect(screen.queryByText('janedoe')).not.toBeInTheDocument();
    });

    it('prefers display_name over name for organization accounts', () => {
      renderSwitcher({ activeAccount: 'acme-corp', accounts: [personal, orgWithDisplay] });
      expect(screen.getByText('Acme Corp')).toBeInTheDocument();
      expect(screen.queryByText('acme-corp')).not.toBeInTheDocument();
    });
  });

  describe('dropdown', () => {
    it('shows all accounts when opened', async () => {
      const user = userEvent.setup();
      renderSwitcher({ activeAccount: 'testuser' });

      await user.click(screen.getByRole('button', { name: triggerName }));

      expect(screen.getAllByText('testuser').length).toBeGreaterThan(0);
      expect(screen.getByText('my-org')).toBeInTheDocument();
    });

    it('shows display_name for accounts that have one in the dropdown', async () => {
      const user = userEvent.setup();
      renderSwitcher({ activeAccount: 'janedoe', accounts: [personalWithDisplay, orgWithDisplay] });

      await user.click(screen.getByRole('button', { name: triggerName }));

      expect(screen.getAllByText('Jane Doe').length).toBeGreaterThan(0);
      expect(screen.getByText('Acme Corp')).toBeInTheDocument();
      expect(screen.queryByText('janedoe')).not.toBeInTheDocument();
      expect(screen.queryByText('acme-corp')).not.toBeInTheDocument();
    });

    it('calls onChange with the account name when an item is selected', async () => {
      const user = userEvent.setup();
      const onChange = vi.fn();
      renderSwitcher({ activeAccount: 'testuser', onChange });

      await user.click(screen.getByRole('button', { name: triggerName }));
      await user.click(screen.getByText('my-org'));

      expect(onChange).toHaveBeenCalledWith('my-org');
    });
  });
});
