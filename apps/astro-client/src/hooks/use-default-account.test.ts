import { describe, it, expect, afterEach, beforeEach, beforeAll, afterAll, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { createElement, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { AuthContext, type AuthContextType } from '@/lib/auth-context';
import { mockAuthContext } from '@/test/test-utils';
import { useDefaultAccount } from './use-default-account';

const orgAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    { id: 'acct-1', name: 'testuser', type: 'personal' as const },
    { id: 'org-1', name: 'my-org', type: 'organization' as const },
  ],
};

function makeWrapper(auth: AuthContextType) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) =>
    createElement(AuthContext.Provider, { value: auth },
      createElement(QueryClientProvider, { client: qc },
        createElement(MemoryRouter, null, children),
      ),
    );
}

// Node.js 22 exposes a built-in localStorage with a limited API that overrides
// jsdom's. Stub it with a proper in-memory implementation so all Storage methods work.
const store = new Map<string, string>();
const localStorageMock = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, value: string) => { store.set(key, value); },
  removeItem: (key: string) => { store.delete(key); },
  clear: () => { store.clear(); },
};

beforeAll(() => vi.stubGlobal('localStorage', localStorageMock));
afterAll(() => vi.unstubAllGlobals());
beforeEach(() => store.clear());
afterEach(() => store.clear());

describe('useDefaultAccount', () => {
  describe('defaultAccount', () => {
    it('returns personalAccount name when nothing is stored', () => {
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(mockAuthContext) });
      expect(result.current.defaultAccount).toBe('testuser');
    });

    it('returns stored org when it is a valid account', () => {
      localStorage.setItem('astro:default-account', 'my-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      expect(result.current.defaultAccount).toBe('my-org');
    });

    it('falls back to personalAccount when stored account is not in the accounts list', () => {
      localStorage.setItem('astro:default-account', 'unknown-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(mockAuthContext) });
      expect(result.current.defaultAccount).toBe('testuser');
    });
  });

  describe('validStoredDefault', () => {
    it('is null when nothing is stored', () => {
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(mockAuthContext) });
      expect(result.current.validStoredDefault).toBeNull();
    });

    it('is the stored account name when it matches a known account', () => {
      localStorage.setItem('astro:default-account', 'my-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      expect(result.current.validStoredDefault).toBe('my-org');
    });

    it('is null when the stored account is not in the accounts list', () => {
      localStorage.setItem('astro:default-account', 'gone-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(mockAuthContext) });
      expect(result.current.validStoredDefault).toBeNull();
    });
  });
});
