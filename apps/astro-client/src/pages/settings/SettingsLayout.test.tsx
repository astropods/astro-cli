import { screen, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, afterEach } from 'vitest';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import SettingsLayout from './SettingsLayout';

afterEach(cleanup);

const auth = { ...mockAuthContext, isAuthenticated: true };

function renderLayout() {
  return renderRoute(
    [
      {
        path: '/settings',
        Component: SettingsLayout,
        children: [
          { path: 'account', Component: () => <div>AccountContent</div> },
          { path: 'usage', Component: () => <div>UsageContent</div> },
          { path: 'secrets', Component: () => <div>SecretsContent</div> },
          { path: 'organizations', Component: () => <div>OrganizationsContent</div> },
        ],
      },
    ],
    { initialEntries: ['/settings/account'], auth },
  );
}

describe('SettingsLayout', () => {
  it('renders all expected nav items', async () => {
    renderLayout();
    await waitFor(() => {
      expect(screen.getAllByText('Account').length).toBeGreaterThan(0);
    });
    expect(screen.getAllByText('Usage').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Variables & Secrets').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Organizations').length).toBeGreaterThan(0);
    expect(screen.queryByText('Experiments')).toBeNull();
  });
});
