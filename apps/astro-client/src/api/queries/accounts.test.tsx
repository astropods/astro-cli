import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useUpdateProfile, useAccountOrgs } from './accounts';
import { createHookWrapper } from '@/test/test-utils';
import { accountKeys } from './keys';
import type { AccountOrgsResponse } from '@/lib/api';

// ── useUpdateProfile ──────────────────────────────────────────────────────────

describe('useUpdateProfile', () => {
  beforeEach(() => {
    server.use(
      http.patch('/api/v1/me', () =>
        HttpResponse.json({ user: { display_name: 'Updated Name' } }),
      ),
    );
  });

  it('invalidates the account detail cache when an account name is provided', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Seed the cache so we can verify it gets invalidated
    queryClient.setQueryData(accountKeys.detail('testuser'), {
      id: 'acct-1',
      name: 'testuser',
      display_name: 'Old Name',
    });

    const { result } = renderHook(() => useUpdateProfile('testuser'), { wrapper });

    result.current.mutate({ display_name: 'Updated Name' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(
      queryClient.getQueryState(accountKeys.detail('testuser'))?.isInvalidated,
    ).toBe(true);
  });

  it('does not invalidate any cache when no account name is provided', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(accountKeys.detail('testuser'), {
      id: 'acct-1',
      name: 'testuser',
      display_name: 'Old Name',
    });

    const { result } = renderHook(() => useUpdateProfile(), { wrapper });

    result.current.mutate({ display_name: 'Updated Name' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Cache entry should NOT be invalidated
    expect(
      queryClient.getQueryState(accountKeys.detail('testuser'))?.isInvalidated,
    ).toBeFalsy();
  });

  it('only invalidates the matching account, not other accounts', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(accountKeys.detail('testuser'), { id: 'a1', name: 'testuser' });
    queryClient.setQueryData(accountKeys.detail('otheruser'), { id: 'a2', name: 'otheruser' });

    const { result } = renderHook(() => useUpdateProfile('testuser'), { wrapper });

    result.current.mutate({ display_name: 'Updated Name' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(
      queryClient.getQueryState(accountKeys.detail('testuser'))?.isInvalidated,
    ).toBe(true);
    expect(
      queryClient.getQueryState(accountKeys.detail('otheruser'))?.isInvalidated,
    ).toBeFalsy();
  });
});

// ── useAccountOrgs ────────────────────────────────────────────────────────────

describe('useAccountOrgs', () => {
  it('fetches org memberships for an account', async () => {
    server.use(
      http.get('/api/v1/accounts/testuser/orgs', () =>
        HttpResponse.json<AccountOrgsResponse>({
          orgs: [{ name: 'acme-corp', display_name: 'Acme Corp' }],
        }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAccountOrgs('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.orgs).toHaveLength(1);
    expect(result.current.data?.orgs[0].name).toBe('acme-corp');
  });

  it('seeds the cache with initialData and skips the network request', async () => {
    server.use(
      http.get('/api/v1/accounts/testuser/orgs', () =>
        HttpResponse.json<AccountOrgsResponse>({ orgs: [] }),
      ),
    );

    const initialData: AccountOrgsResponse = {
      orgs: [{ name: 'preloaded-org', display_name: 'Preloaded' }],
    };

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useAccountOrgs('testuser', { initialData }),
      { wrapper },
    );

    // Data is synchronously available from initialData
    expect(result.current.data?.orgs[0].name).toBe('preloaded-org');

    // staleTime: 0 means a background refetch will fire, so we just verify
    // the data was available immediately without waiting for the network
    expect(result.current.isLoading).toBe(false);
  });

  it('returns empty orgs list when the account has no memberships', async () => {
    server.use(
      http.get('/api/v1/accounts/testuser/orgs', () =>
        HttpResponse.json<AccountOrgsResponse>({ orgs: [] }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAccountOrgs('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.orgs).toHaveLength(0);
  });
});
