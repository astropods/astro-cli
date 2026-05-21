import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useDeployments, useDeployment, useDeploymentEvents, useUndeployAgent, useStopDeployment, useRestartDeployment } from './deployments';
import { createHookWrapper } from '@/test/test-utils';
import { mockDeployments, mockDeploymentEvents } from '@/test/msw/handlers';
import { deploymentKeys } from './keys';
import type { DeploymentsListResponse } from '@/lib/api';

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
              replicas: 1,
              ready: 1,
              created_at: '2025-04-02T00:00:00Z',
              components: ['deployment', 'service'],
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
  it('stops a deployment and invalidates the deployments list and detail cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.all(testAccount), mockDeployments);
    queryClient.setQueryData(deploymentKeys.detail('dep-code-reviewer'), { deployment: mockDeployments.deployments[0] });

    const { result } = renderHook(() => useStopDeployment(testAccount), { wrapper });

    result.current.mutate({ deploymentId: 'dep-code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('stopped');

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
