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
          { path: 'connectors', Component: () => <div>ConnectorsContent</div> },
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
    expect(screen.getAllByText('Billing').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Variables & Secrets').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Connectors').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Organizations').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Experiments').length).toBeGreaterThan(0);
  });

  it('groups nav items under section labels', async () => {
    renderLayout();
    await waitFor(() => expect(screen.getAllByText('Manage').length).toBeGreaterThan(0));
    expect(screen.getAllByText('Access').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Integrations').length).toBeGreaterThan(0);
  });

  it('offers the scope selector for the personal account', async () => {
    renderLayout();
    await waitFor(() => expect(screen.getByLabelText('Settings scope')).toBeInTheDocument());
    expect(screen.getByLabelText('Settings scope')).toHaveTextContent('testuser');
  });

  it('links personal Connectors rather than marking it coming soon', async () => {
    renderLayout();
    await waitFor(() => expect(screen.getAllByText('Connectors').length).toBeGreaterThan(0));
    expect(screen.getAllByText('Connectors')[0].closest('a')).toHaveAttribute(
      'href',
      '/settings/connectors',
    );
    expect(screen.queryAllByText('Coming soon')).toHaveLength(0);
  });
});
