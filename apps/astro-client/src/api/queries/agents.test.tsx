import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useAgents, useAgent, useDeployAgent } from './agents';
import { createHookWrapper } from '@/test/test-utils';
import { mockAgents } from '@/test/msw/handlers';
import { deploymentKeys } from './keys';

const testAccount = 'testuser';

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
  it('fetches a single agent by account and name', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent(testAccount, 'code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.name).toBe('code-reviewer');
    expect(result.current.data?.versions).toHaveLength(2);
  });

  it('does not fetch when name is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent(testAccount, ''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when account is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent('', 'code-reviewer'), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('returns an error for a non-existent agent', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useAgent(testAccount, 'no-such-agent'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'not_found' });
  });
});

describe('useDeployAgent', () => {
  it('deploys an agent and invalidates deployments cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Prime the deployments cache so we can verify invalidation
    queryClient.setQueryData(deploymentKeys.all(testAccount), { deployments: [], count: 0, namespace: 'test' });

    const { result } = renderHook(() => useDeployAgent(testAccount), { wrapper });

    result.current.mutate({
      spec: 'deployment/v1',
      source: { account: testAccount, name: 'code-reviewer', build: 'a1b2c3d4e5f6', registry: 'registry.example.com' },
      target: { runtime: 'kubernetes', namespace: 'prod' },
      agent: { image: 'registry.example.com/testuser/code-reviewer:a1b2c3d4e5f6', endpoints: { http: { port: 8080 } } },
    } as any);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('deployed');
    expect(result.current.data?.name).toBe('code-reviewer');

    // Deployments cache should have been invalidated
    const deploymentsState = queryClient.getQueryState(deploymentKeys.all(testAccount));
    expect(deploymentsState?.isInvalidated).toBe(true);
  });
});
