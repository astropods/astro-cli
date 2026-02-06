import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useDeployments, useUndeployAgent } from './deployments';
import { createHookWrapper } from '@/test/test-utils';
import { mockDeployments } from '@/test/msw/handlers';
import { deploymentKeys } from './keys';

describe('useDeployments', () => {
  it('fetches the deployment list', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.deployments).toHaveLength(mockDeployments.deployments.length);
    expect(result.current.data?.deployments[0].name).toBe('code-reviewer');
    expect(result.current.data?.namespace).toBe('user-abc123');
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/deployments', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useDeployments(), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'internal_error' });
  });
});

describe('useUndeployAgent', () => {
  it('undeploys an agent and invalidates deployments cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Prime the cache
    queryClient.setQueryData(deploymentKeys.all, mockDeployments);

    const { result } = renderHook(() => useUndeployAgent(), { wrapper });

    result.current.mutate({ name: 'code-reviewer', version: '1.0.0' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('undeployed');

    // Deployments cache should have been invalidated
    const deploymentsState = queryClient.getQueryState(deploymentKeys.all);
    expect(deploymentsState?.isInvalidated).toBe(true);
  });
});
