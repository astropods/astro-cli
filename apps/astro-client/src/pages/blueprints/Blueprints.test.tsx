import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { waitFor, cleanup } from '@testing-library/react';
import { renderRoute, mockAuthContext } from '@/test/test-utils';
import Blueprints from './Blueprints';

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

function renderBlueprintsPage({
  initialEntries = ['/blueprints'],
  auth = mockAuthContext,
}: {
  initialEntries?: string[];
  auth?: typeof mockAuthContext;
} = {}) {
  return renderRoute(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    [{ path: '/blueprints', Component: Blueprints as any }],
    { initialEntries, auth },
  );
}

describe('Blueprints – ?account= param handling', () => {
  it('sets active account from ?account param once accounts have loaded', async () => {
    const auth = {
      ...mockAuthContext,
      accounts: [
        { id: 'acct-1', name: 'testuser', type: 'personal' },
        { id: 'acct-2', name: 'orgaccount', type: 'org' },
      ],
    };

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=orgaccount'],
      auth,
    });

    await waitFor(() => {
      expect(localStorage.getItem('astro:default-account')).toBe('orgaccount');
    });
  });

  it('does not consume ?account param before accounts have loaded (no-flicker)', async () => {
    // Simulate the initial render before auth resolves — accounts is empty.
    // The old code would call setSearchParams({}) here, stripping the param so
    // it could never be processed once accounts populated. The guard fixes this.
    const auth = { ...mockAuthContext, accounts: [] };

    renderBlueprintsPage({
      initialEntries: ['/blueprints?account=testuser'],
      auth,
    });

    // Allow any queued effects to flush.
    await new Promise((resolve) => setTimeout(resolve, 50));

    // setActiveAccount must not have been called — param is preserved for when accounts load.
    expect(localStorage.getItem('astro:default-account')).toBeNull();
  });
});
