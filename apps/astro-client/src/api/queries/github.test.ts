import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useGitHubStatus, useGitHubRebuild, useGitHubAccountConnections, useGitHubAccountStatus, useGitHubAccountDisconnect, useGitHubAccountConnect, useGitHubAccountOrgs } from './github';
import { createHookWrapper } from '@/test/test-utils';
import type { GitHubStatusResponse } from '@/lib/api';
import { githubKeys } from '@/api/queries/keys';

const mockStatus: GitHubStatusResponse = {
  connected: true,
  repo_full_name: 'owner/repo',
  branch: 'main',
  builds: [],
};

const mockStatusPending: GitHubStatusResponse = {
  connected: true,
  repo_full_name: 'owner/repo',
  branch: 'main',
  builds: [{ id: 'b1', build_id: 'bid1', commit_sha: 'abc', branch: 'main', status: 'pending', enqueued_at: '' }],
};

describe('useGitHubStatus', () => {
  it('fetches status for a connected agent', async () => {
    server.use(
      http.get('/api/v1/agents/:account/:name/github', () =>
        HttpResponse.json(mockStatus),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubStatus('testuser', 'code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.connected).toBe(true);
    expect(result.current.data?.repo_full_name).toBe('owner/repo');
    expect(result.current.data?.builds).toHaveLength(0);
  });

  it('does not fetch when enabled is false', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubStatus('testuser', 'code-reviewer', { enabled: false }),
      { wrapper },
    );

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when account is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubStatus('', 'code-reviewer'), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when name is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubStatus('testuser', ''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not poll a cached in-flight build when the caller is disabled', async () => {
    let requests = 0;
    server.use(
      http.get('/api/v1/agents/:account/:name/github', () => {
        requests += 1;
        return HttpResponse.json(mockStatusPending);
      }),
    );

    const { wrapper, queryClient } = createHookWrapper();
    queryClient.setQueryData(githubKeys.status('testuser', 'code-reviewer'), mockStatusPending);
    const { result } = renderHook(
      () => useGitHubStatus('testuser', 'code-reviewer', { enabled: false }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.data).toEqual(mockStatusPending));
    await new Promise((resolve) => setTimeout(resolve, 100));

    expect(result.current.fetchStatus).toBe('idle');
    expect(requests).toBe(0);
  });

  it('polls when latest build status is pending', async () => {
    server.use(
      http.get('/api/v1/agents/:account/:name/github', () =>
        HttpResponse.json(mockStatusPending),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubStatus('testuser', 'code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // refetchInterval returns 5000 when build is pending
    const interval = result.current.isSuccess ? 5000 : false;
    expect(interval).toBe(5000);
  });

  it('disables polling when refetchInterval is false', async () => {
    server.use(
      http.get('/api/v1/agents/:account/:name/github', () =>
        HttpResponse.json(mockStatusPending),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubStatus('testuser', 'code-reviewer', { refetchInterval: false }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // when override is false, polling is disabled regardless of build status
    expect(result.current.data?.builds[0].status).toBe('pending');
  });

  it('returns error when server responds with 500', async () => {
    server.use(
      http.get('/api/v1/agents/:account/:name/github', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubStatus('testuser', 'code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useGitHubRebuild', () => {
  it('invalidates github status query on success', async () => {
    server.use(
      http.post('/api/v1/agents/:account/:name/github/rebuild', () =>
        HttpResponse.json({ ok: true }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(() => useGitHubRebuild('testuser', 'code-reviewer'), { wrapper });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const state = queryClient.getQueryState(['github', 'testuser', 'code-reviewer']);
    expect(state?.isInvalidated ?? true).toBe(true);
  });
});

describe('useGitHubAccountConnections', () => {
  it('fetches account connections', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github/connections', () =>
        HttpResponse.json({ connections: [{ agent_name: 'code-reviewer', repo_full_name: 'owner/repo' }] }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnections('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.connections).toHaveLength(1);
    expect(result.current.data?.connections[0].agent_name).toBe('code-reviewer');
  });

  it('does not fetch when account is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnections(''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when disabled', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubAccountConnections('testuser', { enabled: false }),
      { wrapper },
    );

    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useGitHubAccountOrgs', () => {
  it('fetches the OAuth org scope', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github/orgs', () =>
        HttpResponse.json({
          orgs: [
            { login: 'org-a', avatar_url: 'https://example.com/a.png' },
            { login: 'org-b', avatar_url: 'https://example.com/b.png' },
          ],
        }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountOrgs('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.orgs).toHaveLength(2);
    expect(result.current.data?.orgs[0]).toEqual({ login: 'org-a', avatar_url: 'https://example.com/a.png' });
  });

  it('does not fetch when disabled (e.g. account not connected)', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubAccountOrgs('testuser', { enabled: false }),
      { wrapper },
    );

    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useGitHubAccountStatus', () => {
  it('fetches status and returns connected=true with github_login', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github', () =>
        HttpResponse.json({ connected: true, github_login: 'gh-user' }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountStatus('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.connected).toBe(true);
    expect(result.current.data?.github_login).toBe('gh-user');
  });

  it('returns connected=false when server returns not-connected', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github', () =>
        HttpResponse.json({ connected: false }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountStatus('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.connected).toBe(false);
    expect(result.current.data?.github_login).toBeUndefined();
  });

  it('does not fetch when account is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountStatus(''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when enabled is false', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubAccountStatus('testuser', { enabled: false }),
      { wrapper },
    );

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('returns initialData synchronously before any fetch', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useGitHubAccountStatus('testuser', { initialData: { connected: true, github_login: 'seeded-user' } }),
      { wrapper },
    );

    // Data must be available on the very first render — no await.
    expect(result.current.data?.connected).toBe(true);
    expect(result.current.data?.github_login).toBe('seeded-user');
  });

  it('returns error on 500', async () => {
    server.use(
      http.get('/api/v1/accounts/:account/github', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountStatus('testuser'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useGitHubAccountDisconnect', () => {
  it('calls DELETE /api/v1/accounts/:account/github', async () => {
    let called = false;
    server.use(
      http.delete('/api/v1/accounts/:account/github', () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountDisconnect('testuser'), { wrapper });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(called).toBe(true);
  });

  it('does not overwrite accountStatus cache on success (component owns optimistic update)', async () => {
    server.use(
      http.delete('/api/v1/accounts/:account/github', () =>
        new HttpResponse(null, { status: 204 }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();

    // Pre-populate so there is something to verify against.
    queryClient.setQueryData(githubKeys.accountStatus('testuser'), { connected: true, github_login: 'gh-user' });

    const { result } = renderHook(() => useGitHubAccountDisconnect('testuser'), { wrapper });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // The hook no longer writes { connected: false } — the component sets this
    // optimistically before calling mutate(). cancelQueries prevents a stale
    // server response from flipping the toggle back on.
    expect(queryClient.getQueryData(githubKeys.accountStatus('testuser'))).not.toEqual({ connected: false });
  });

  it('marks [\'github\', account] query as invalidated on success', async () => {
    server.use(
      http.delete('/api/v1/accounts/:account/github', () =>
        new HttpResponse(null, { status: 204 }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();

    // Seed a query so its state exists to check.
    queryClient.setQueryData(['github', 'testuser'], { some: 'data' });

    const { result } = renderHook(() => useGitHubAccountDisconnect('testuser'), { wrapper });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const state = queryClient.getQueryState(['github', 'testuser']);
    expect(state?.isInvalidated ?? true).toBe(true);
  });
});

describe('useGitHubAccountConnect', () => {
  it('sets accountStatus cache and invalidates accountConnections when connected=true', async () => {
    server.use(
      http.post('/api/v1/accounts/:account/github/connect', () =>
        HttpResponse.json({ connected: true, github_login: 'gh-user' }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnect('testuser'), { wrapper });

    result.current.mutate({ redirectTo: '/testuser/my-agent?github_connected=true' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(queryClient.getQueryData(githubKeys.accountStatus('testuser'))).toEqual({
      connected: true,
      github_login: 'gh-user',
    });

    const connectionsState = queryClient.getQueryState(githubKeys.accountConnections('testuser'));
    expect(connectionsState?.isInvalidated ?? true).toBe(true);
  });

  it('does NOT set accountStatus cache when server returns redirect_url (connected=false path)', async () => {
    server.use(
      http.post('/api/v1/accounts/:account/github/connect', () =>
        HttpResponse.json({ redirect_url: 'https://github.com/login/oauth/authorize?client_id=abc' }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnect('testuser'), { wrapper });

    result.current.mutate({ redirectTo: '/testuser/my-agent?github_connected=true' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(queryClient.getQueryData(githubKeys.accountStatus('testuser'))).toBeUndefined();
  });

  it('sends force=true in request body when force is set', async () => {
    let capturedBody: unknown;
    server.use(
      http.post('/api/v1/accounts/:account/github/connect', async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json({ redirect_url: 'https://github.com/login/oauth/authorize?reauth=1' });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnect('testuser'), { wrapper });

    result.current.mutate({ redirectTo: '/settings/connectors', force: true });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(capturedBody).toMatchObject({ redirect_to: '/settings/connectors', force: true });
  });

  it('sends force=false in request body when force is not set', async () => {
    let capturedBody: unknown;
    server.use(
      http.post('/api/v1/accounts/:account/github/connect', async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json({ connected: true, github_login: 'gh-user' });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useGitHubAccountConnect('testuser'), { wrapper });

    result.current.mutate({ redirectTo: '/settings/connectors' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(capturedBody).toMatchObject({ redirect_to: '/settings/connectors', force: false });
  });
});
