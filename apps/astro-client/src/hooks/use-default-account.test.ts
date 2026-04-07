import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
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

beforeEach(() => localStorage.clear());
afterEach(() => localStorage.clear());

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

  describe('handleSetDefault', () => {
    it('stores an org as the default', () => {
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      act(() => result.current.handleSetDefault('my-org'));
      expect(localStorage.getItem('astro:default-account')).toBe('my-org');
    });

    it('clears the stored default when the current default org is toggled off', () => {
      localStorage.setItem('astro:default-account', 'my-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      act(() => result.current.handleSetDefault('my-org'));
      expect(localStorage.getItem('astro:default-account')).toBeNull();
    });

    it('clears the stored org default when personal account is passed', () => {
      localStorage.setItem('astro:default-account', 'my-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      act(() => result.current.handleSetDefault('testuser'));
      expect(localStorage.getItem('astro:default-account')).toBeNull();
    });

    it('does nothing when personal account is already the natural default', () => {
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(mockAuthContext) });
      act(() => result.current.handleSetDefault('testuser'));
      expect(localStorage.getItem('astro:default-account')).toBeNull();
    });

    it('updates validStoredDefault reactively after setting a default', () => {
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      expect(result.current.validStoredDefault).toBeNull();
      act(() => result.current.handleSetDefault('my-org'));
      expect(result.current.validStoredDefault).toBe('my-org');
    });

    it('clears validStoredDefault reactively after removing a default', () => {
      localStorage.setItem('astro:default-account', 'my-org');
      const { result } = renderHook(() => useDefaultAccount(), { wrapper: makeWrapper(orgAuth) });
      expect(result.current.validStoredDefault).toBe('my-org');
      act(() => result.current.handleSetDefault('my-org'));
      expect(result.current.validStoredDefault).toBeNull();
    });
  });
});
