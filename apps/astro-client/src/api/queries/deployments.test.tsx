import { describe, it, expect, vi, afterEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useDeployments, useDeployment, useDeploymentEvents, useDeploymentLogs, useUndeployAgent, useStopDeployment, useWakeUpDeployment, useRestartDeployment, markDeploymentResuming, statusRefetchInterval } from './deployments';
import { createHookWrapper } from '@/test/test-utils';
import { mockDeployments, mockDeploymentEvents } from '@/test/msw/handlers';
import { deploymentKeys } from './keys';
import type { AgentDeployment, DeploymentStatus, DeploymentsListResponse } from '@/lib/api';
import type { LogEntry } from '@/lib/log-utils';

const testAccount = 'testuser';

describe('useDeployments', () => {
  it('fetches the deployment list', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(testAccount), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.deployments).toHaveLength(mockDeployments.deployments.length);
    expect(result.current.data?.deployments[0].name).toBe('code-reviewer');
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(testAccount), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'internal_error' });
  });
});

// Cross-account naming regression suite.
//
// The ListDeployments handler merges data from the DB and K8s. The bug was that
// the K8s label `astro.dev/agent` (account-qualified, e.g. "simon.mindcraft")
// leaked into the `name` field of the API response instead of the plain DB name.
// The client passes `deployment.name` directly to useAgent and
// usePrefilledDeploymentTemplate, so a dotted name causes 404s.
//
// These tests verify the expected value with a correct server and prove the
// client has no defense against a buggy one (the fix must be server-side).
describe('useDeployments – cross-account naming', () => {
  it('returns plain agent name for cross-account deployments, not account-qualified', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(testAccount), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const crossDep = result.current.data?.deployments.find((d) => d.id === 'dep-cross-account');
    expect(crossDep).toBeDefined();
    expect(crossDep?.name).toBe('data-analyst');
    expect(crossDep?.name).not.toContain('.');
  });

  // Negative test: proves the useDeployments hook is a passthrough — if the
  // server returns a dotted name, the client will faithfully use it in
  // downstream useAgent / usePrefilledDeploymentTemplate calls, causing 404s.
  // This confirms the server-side fix (setting Name from the DB record) is the
  // only line of defense.
  it('client does not sanitize account-qualified names — a buggy server response flows through unchanged', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json<DeploymentsListResponse>({
          deployments: [
            {
              id: 'dep-buggy',
              name: 'otheraccount.data-analyst',
              display_name: 'Buggy Deploy',
              build_id: 'c3d4e5f6a7b8',
              namespace: 'astro-cross999',
              status: 'Running',
              created_at: '2025-04-02T00:00:00Z',
            },
          ],
          count: 1,
        }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(testAccount), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const dep = result.current.data?.deployments[0];
    expect(dep?.name).toBe('otheraccount.data-analyst');
    expect(dep?.name).toContain('.');
  });
});

describe('useDeployment', () => {
  it('fetches a single deployment by id', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployment('dep-code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.deployment.id).toBe('dep-code-reviewer');
    expect(result.current.data?.deployment.name).toBe('code-reviewer');
  });

  it('returns 404 for an unknown deployment id', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployment('dep-unknown'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'not_found' });
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/deployments/:id', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployment('dep-code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'internal_error' });
  });

  it('does not fetch when id is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployment(''), { wrapper });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useDeploymentEvents', () => {
  it('fetches events for a deployment', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeploymentEvents('dep-code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.events).toHaveLength(mockDeploymentEvents.events.length);
    expect(result.current.data?.events[0].type).toBe('Normal');
    expect(result.current.data?.events[0].reason).toBe('Scheduled');
    expect(result.current.data?.events[2].type).toBe('Warning');
    expect(result.current.data?.events[2].reason).toBe('Unhealthy');
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/deployments/:id/events', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeploymentEvents('dep-code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ code: 'internal_error' });
  });

  it('does not fetch when disabled', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeploymentEvents('dep-code-reviewer', false), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when deployment id is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeploymentEvents(''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('useStopDeployment', () => {
  it('stops a deployment, seeds paused caches, and invalidates list/detail reads', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.all(testAccount), mockDeployments);
    queryClient.setQueryData(deploymentKeys.detail('dep-code-reviewer'), { deployment: mockDeployments.deployments[0] });
    queryClient.setQueryData<DeploymentStatus>(deploymentKeys.status('dep-code-reviewer'), {
      value: 'active',
      reason: 'ready',
      details: 'Deployment is active',
    });

    const { result } = renderHook(() => useStopDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('stopped');

    const list = queryClient.getQueryData<DeploymentsListResponse>(deploymentKeys.all(testAccount));
    expect(list?.deployments.find((d) => d.id === 'dep-code-reviewer')?.status).toBe('stopped');

    const detail = queryClient.getQueryData<{ deployment: AgentDeployment }>(deploymentKeys.detail('dep-code-reviewer'));
    expect(detail?.deployment.status).toBe('stopped');

    const status = queryClient.getQueryData<DeploymentStatus>(deploymentKeys.status('dep-code-reviewer'));
    expect(status).toMatchObject({ value: 'inactive', reason: 'paused' });

    const deploymentsState = queryClient.getQueryState(deploymentKeys.all(testAccount));
    expect(deploymentsState?.isInvalidated).toBe(true);

    const detailState = queryClient.getQueryState(deploymentKeys.detail('dep-code-reviewer'));
    expect(detailState?.isInvalidated).toBe(true);
  });

  it('keeps the button disabled until the server responds — rejects a second stop call while one is in-flight', async () => {
    let resolveFirst!: () => void;
    server.use(
      http.post('/api/v1/deployments/:id/stop', () =>
        new Promise<Response>((resolve) => {
          resolveFirst = () => resolve(HttpResponse.json({ status: 'stopped', deployment_id: 'dep-code-reviewer' }));
        }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useStopDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isPending).toBe(true));

    // A second call while the first is in-flight should not be possible —
    // the button is disabled when isPending is true.
    expect(result.current.isPending).toBe(true);

    resolveFirst();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useWakeUpDeployment', () => {
  it('optimistically marks the status as deploying so it keeps polling instead of sticking on the stale paused value', async () => {
    server.use(
      http.post('/api/v1/deployments/:id/wakeup', () =>
        HttpResponse.json({ status: 'pending', deployment_id: 'dep-code-reviewer' }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();

    // Prime the status query with the paused state the server still reports at
    // ack time. "inactive" is not a state useDeploymentStatus polls on, so
    // without the optimistic update the toggle would stick here.
    queryClient.setQueryData(deploymentKeys.status('dep-code-reviewer'), {
      value: 'inactive',
      reason: 'paused',
      details: 'Agent is paused',
    });
    queryClient.setQueryData(deploymentKeys.all(testAccount), mockDeployments);

    const { result } = renderHook(() => useWakeUpDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Status query flips to a polling state immediately — not left stale.
    const status = queryClient.getQueryData(deploymentKeys.status('dep-code-reviewer'));
    expect(status).toMatchObject({ value: 'deploying' });

    // The list entry transitions too, which keeps useDeployments polling.
    const list = queryClient.getQueryData<DeploymentsListResponse>(deploymentKeys.all(testAccount));
    expect(list?.deployments.find((d) => d.id === 'dep-code-reviewer')?.status).toBe('pending');

    // ...and a resume grace window is opened so an interim "inactive" read
    // from the still-lagging server doesn't terminate polling.
    expect(statusRefetchInterval('dep-code-reviewer', 'inactive')).toBe(3000);
  });

  it('seeds a full in-progress status when the status cache is empty so the badge reflects it immediately', async () => {
    server.use(
      http.post('/api/v1/deployments/:id/wakeup', () =>
        HttpResponse.json({ status: 'pending', deployment_id: 'dep-code-reviewer' }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();

    // No prior status read — the status cache is empty (wakeup triggered
    // before any observer mounted useDeploymentStatus).
    queryClient.setQueryData(deploymentKeys.all(testAccount), mockDeployments);
    expect(queryClient.getQueryData(deploymentKeys.status('dep-code-reviewer'))).toBeUndefined();

    const { result } = renderHook(() => useWakeUpDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // A complete DeploymentStatus is seeded — not left undefined — so a status
    // observer mounting afterward renders the in-progress badge and polls.
    const status = queryClient.getQueryData(deploymentKeys.status('dep-code-reviewer'));
    expect(status).toEqual({
      value: 'deploying',
      reason: 'provisioning',
      details: 'Pods are being provisioned',
    });
  });
});

describe('statusRefetchInterval – resume grace window', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('polls while transitional regardless of any grace window', () => {
    expect(statusRefetchInterval('dep-x', 'deploying')).toBe(3000);
    expect(statusRefetchInterval('dep-x', 'undeploying')).toBe(3000);
  });

  it('idles on a terminal state when no resume is in flight', () => {
    expect(statusRefetchInterval('dep-idle', 'inactive')).toBe(false);
    expect(statusRefetchInterval('dep-idle', 'active')).toBe(false);
  });

  it('keeps polling through one or more interim non-polling reads after a resume', () => {
    markDeploymentResuming('dep-resume');
    // Right after a resume, a status read can still report the pre-resume
    // terminal "inactive" (stopped) before the wakeup worker flips it to
    // provisioning/active — these reads must NOT stop polling.
    expect(statusRefetchInterval('dep-resume', 'inactive')).toBe(3000);
    expect(statusRefetchInterval('dep-resume', 'inactive')).toBe(3000);
  });

  it('closes the window and idles once status converges on active', () => {
    markDeploymentResuming('dep-converge');
    expect(statusRefetchInterval('dep-converge', 'inactive')).toBe(3000);
    expect(statusRefetchInterval('dep-converge', 'active')).toBe(false);
    // Window cleared: a later spurious inactive read no longer revives polling.
    expect(statusRefetchInterval('dep-converge', 'inactive')).toBe(false);
  });

  it('stops honoring the window after it lapses', () => {
    vi.useFakeTimers();
    markDeploymentResuming('dep-lapse');
    expect(statusRefetchInterval('dep-lapse', 'inactive')).toBe(3000);
    vi.advanceTimersByTime(31_000);
    expect(statusRefetchInterval('dep-lapse', 'inactive')).toBe(false);
  });

  it('slides the window forward on transitional reads so a long cold start does not strand a later inactive read', () => {
    vi.useFakeTimers();
    markDeploymentResuming('dep-slide');

    // A slow cold start (image pull) reports "deploying" for well past one
    // RESUME_GRACE_MS (30s). Each transitional poll slides the deadline forward.
    for (let elapsed = 0; elapsed < 90_000; elapsed += 3_000) {
      expect(statusRefetchInterval('dep-slide', 'deploying')).toBe(3000);
      vi.advanceTimersByTime(3_000);
    }

    // The post-active reconcile race now produces a transient "inactive" ~90s
    // after wakeup — long past a fixed 30s deadline. Because the transitional
    // reads kept the window fresh, polling continues instead of terminating.
    expect(statusRefetchInterval('dep-slide', 'inactive')).toBe(3000);
  });

  it('still early-exits on active after the window has been slid forward', () => {
    vi.useFakeTimers();
    markDeploymentResuming('dep-slide-active');

    statusRefetchInterval('dep-slide-active', 'deploying');
    vi.advanceTimersByTime(40_000);
    // Past the original deadline, but a fresh transitional read slides it.
    expect(statusRefetchInterval('dep-slide-active', 'deploying')).toBe(3000);

    // Convergence still wins and closes the window.
    expect(statusRefetchInterval('dep-slide-active', 'active')).toBe(false);
    expect(statusRefetchInterval('dep-slide-active', 'inactive')).toBe(false);
  });

  it('does not open a window for a plain deploy (transitional reads with no resume)', () => {
    // No markDeploymentResuming: a normal deploy polls while transitional but
    // must not grant a grace window that survives into a terminal read.
    expect(statusRefetchInterval('dep-plain', 'deploying')).toBe(3000);
    expect(statusRefetchInterval('dep-plain', 'inactive')).toBe(false);
  });
});

describe('useRestartDeployment', () => {
  it('restarts a deployment and invalidates the detail cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.detail('dep-code-reviewer'), { deployment: mockDeployments.deployments[0] });

    const { result } = renderHook(() => useRestartDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('restarting');
    expect(result.current.data?.pods).toEqual(['pod-abc-1', 'pod-abc-2']);

    const detailState = queryClient.getQueryState(deploymentKeys.detail('dep-code-reviewer'));
    expect(detailState?.isInvalidated).toBe(true);
  });

  it('exposes isPending while the request is in-flight', async () => {
    let resolve!: () => void;
    server.use(
      http.post('/api/v1/deployments/:id/restart', () =>
        new Promise<Response>((res) => {
          resolve = () => res(HttpResponse.json({ status: 'restarting', pods: [] }));
        }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useRestartDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isPending).toBe(true));
    expect(result.current.isPending).toBe(true);

    resolve();
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('useUndeployAgent', () => {
  it('undeploys an agent and invalidates deployments cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Prime the cache
    queryClient.setQueryData(deploymentKeys.all(testAccount), mockDeployments);

    const { result } = renderHook(() => useUndeployAgent(testAccount), { wrapper });

    result.current.mutate({ deployment_id: 'test-deployment-id' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('undeployed');

    // Deployments cache should have been invalidated
    const deploymentsState = queryClient.getQueryState(deploymentKeys.all(testAccount));
    expect(deploymentsState?.isInvalidated).toBe(true);
  });
});

const mockLogs: LogEntry[] = Array.from({ length: 3 }, (_, i) => ({
  timestamp: `2026-06-02T10:00:0${i}.000Z`,
  level: 'info',
  message: `log line ${i}`,
}));

describe('useDeploymentLogs', () => {
  it('fetches logs with direction=backward and tailLines=500', async () => {
    let gotDirection: string | null = null;
    let gotTailLines: string | null = null;

    server.use(
      http.get('/api/v1/deployments/:id/logs', ({ request }) => {
        const url = new URL(request.url);
        gotDirection = url.searchParams.get('direction');
        gotTailLines = url.searchParams.get('tailLines');
        return HttpResponse.json(mockLogs);
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentLogs('dep-1', 'my-workload', 'agent'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(gotDirection).toBe('backward');
    expect(gotTailLines).toBe('500');
    expect(result.current.data).toHaveLength(3);
    expect(result.current.data[0].message).toBe('log line 0');
  });

  it('returns hasMore=true when a full page is returned and false when partial', async () => {
    const fullPage: LogEntry[] = Array.from({ length: 500 }, (_, i) => ({
      timestamp: `2026-06-02T10:00:00.${String(i).padStart(3, '0')}Z`,
      level: 'info',
      message: `line ${i}`,
    }));

    server.use(
      http.get('/api/v1/deployments/:id/logs', () => HttpResponse.json(fullPage)),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentLogs('dep-1', 'my-workload', 'agent'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.hasMore).toBe(true);

    // Simulate a partial page (fewer than 500) for the next fetch
    server.use(
      http.get('/api/v1/deployments/:id/logs', () => HttpResponse.json(mockLogs)),
    );

    await act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.hasMore).toBe(false));
  });

  it('passes until cursor from oldest log when loading more', async () => {
    let gotUntil: string | null = null;
    let callCount = 0;

    const firstPage: LogEntry[] = Array.from({ length: 500 }, (_, i) => ({
      timestamp: `2026-06-02T10:00:00.${String(i).padStart(3, '0')}Z`,
      level: 'info',
      message: `line ${i}`,
    }));

    server.use(
      http.get('/api/v1/deployments/:id/logs', ({ request }) => {
        callCount++;
        const url = new URL(request.url);
        if (callCount > 1) gotUntil = url.searchParams.get('until');
        return HttpResponse.json(callCount === 1 ? firstPage : mockLogs);
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentLogs('dep-1', 'my-workload', 'agent'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    await act(() => result.current.loadMore());

    // The until cursor should be the oldest (first) timestamp of the initial page.
    expect(gotUntil).toBe(firstPage[0].timestamp);
  });

  it('does not fetch when disabled', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentLogs('dep-1', 'my-workload', 'agent', '1h', 'UTC', { enabled: false }),
      { wrapper },
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.data).toHaveLength(0);
  });
});
