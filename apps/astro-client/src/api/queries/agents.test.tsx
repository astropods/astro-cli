import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useAgents, useAgent, useDeployAgent } from './agents';
import { createHookWrapper } from '@/test/test-utils';
import { mockAgents } from '@/test/msw/handlers';
import { deploymentKeys } from './keys';

describe('useAgents', () => {
  it('fetches the agent list', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgents(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.agents).toHaveLength(mockAgents.length);
    expect(result.current.data?.agents[0].name).toBe('code-reviewer');
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/agents', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgents(), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'internal_error' });
  });
});

describe('useAgent', () => {
  it('fetches a single agent by name', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent('code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.name).toBe('code-reviewer');
    expect(result.current.data?.versions).toHaveLength(2);
  });

  it('does not fetch when name is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent(''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('returns an error for a non-existent agent', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent('no-such-agent'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'not_found' });
  });
});

describe('useDeployAgent', () => {
  it('deploys an agent and invalidates deployments cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Prime the deployments cache so we can verify invalidation
    queryClient.setQueryData(deploymentKeys.all, { deployments: [], count: 0, namespace: 'test' });

    const { result } = renderHook(() => useDeployAgent(), { wrapper });

    result.current.mutate({ name: 'code-reviewer', version: '1.0.0' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('deployed');
    expect(result.current.data?.name).toBe('code-reviewer');

    // Deployments cache should have been invalidated
    const deploymentsState = queryClient.getQueryState(deploymentKeys.all);
    expect(deploymentsState?.isInvalidated).toBe(true);
  });
});
