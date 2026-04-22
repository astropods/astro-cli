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

function renderSwitcher({
  activeAccount = 'testuser',
  onChange = vi.fn(),
  accounts = [personal, org],
}: {
  activeAccount?: string;
  onChange?: (account: string) => void;
  accounts?: { id: string; name: string; type: string }[];
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
  });

  describe('dropdown', () => {
    it('shows all accounts when opened', async () => {
      const user = userEvent.setup();
      renderSwitcher({ activeAccount: 'testuser' });

      await user.click(screen.getByRole('button', { name: triggerName }));

      expect(screen.getAllByText('testuser').length).toBeGreaterThan(0);
      expect(screen.getByText('my-org')).toBeInTheDocument();
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
