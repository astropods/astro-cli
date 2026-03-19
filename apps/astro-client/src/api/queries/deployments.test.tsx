import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useDeployments, useUndeployAgent } from './deployments';
import { createHookWrapper } from '@/test/test-utils';
import { mockDeployments } from '@/test/msw/handlers';
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

    expect(result.current.error).toMatchObject({ error: 'internal_error' });
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
