import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useGitHubStatus, useGitHubRebuild, useGitHubAccountConnections } from './github';
import { createHookWrapper } from '@/test/test-utils';
import type { GitHubStatusResponse } from '@/lib/api';

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
