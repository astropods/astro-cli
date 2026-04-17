import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useBlueprints, useBlueprint, useDeployAgent, usePrefilledDeploymentTemplate, useArchiveBlueprint, useCreateBlueprint } from './blueprints';
import { createHookWrapper } from '@/test/test-utils';
import { mockBlueprints } from '@/test/msw/handlers';
import { blueprintKeys, deploymentKeys } from './keys';
import type { Blueprint } from '@/lib/api';

const testAccount = 'testuser';

describe('useBlueprints', () => {
  it('fetches the blueprint list', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprints(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.agents).toHaveLength(mockBlueprints.length);
    expect(result.current.data?.agents[0].name).toBe('code-reviewer');
  });

  it('returns an error when the server fails', async () => {
    server.use(
      http.get('/api/v1/agents', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprints(), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'internal_error' });
  });
});

describe('useBlueprint', () => {
  it('fetches a single blueprint by account and name', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprint(testAccount, 'code-reviewer'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.name).toBe('code-reviewer');
    expect(result.current.data?.versions).toHaveLength(2);
  });

  it('does not fetch when name is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprint(testAccount, ''), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('does not fetch when account is empty', () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprint('', 'code-reviewer'), { wrapper });

    expect(result.current.fetchStatus).toBe('idle');
  });

  it('returns an error for a non-existent blueprint', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprint(testAccount, 'no-such-agent'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'not_found' });
  });

  // Regression: if ListDeployments returns a dotted name like
  // "otheraccount.data-analyst", the configure page passes it to useBlueprint,
  // which calls GET /api/v1/agents/testuser/otheraccount.data-analyst — a
  // blueprint that doesn't exist under that account. Together with the passthrough
  // test in deployments.test.tsx, this proves the full failure chain.
  it('returns 404 for account-qualified name (regression: dotted name from buggy ListDeployments)', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBlueprint(testAccount, 'otheraccount.data-analyst'), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toMatchObject({ error: 'not_found' });
  });
});

// The configure page calls usePrefilledDeploymentTemplate(account, deployment.name, deploymentId).
// The server resolves the template by matching all three params. If deployment.name is
// account-qualified (the bug), the server can't find the template and returns 404.
describe('usePrefilledDeploymentTemplate', () => {
  // Happy path: plain name resolves to the correct prefilled template.
  it('fetches prefilled template for a cross-account deployment using plain agent name', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => usePrefilledDeploymentTemplate(testAccount, 'data-analyst', 'dep-cross-account'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.source.name).toBe('data-analyst');
    expect(result.current.data?.variables?.OPENAI_API_KEY?.value).toBe('sk-cross-value');
  });

  // Negative: dotted name doesn't match any template route, proving the failure mode.
  it('returns 404 when account-qualified name is used instead of plain name', async () => {
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => usePrefilledDeploymentTemplate(testAccount, 'otheruser.data-analyst', 'dep-cross-account'),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toMatchObject({ error: 'not_found' });
  });
});

describe('useDeployAgent', () => {
  const deployPayload = {
    spec: 'deployment/v1',
    source: { account: testAccount, name: 'code-reviewer', build: 'a1b2c3d4e5f6', registry: 'registry.example.com' },
    target: { runtime: 'kubernetes', namespace: 'prod' },
    agent: { image: 'registry.example.com/testuser/code-reviewer:a1b2c3d4e5f6', endpoints: { http: { port: 8080 } } },
  } as unknown as Parameters<ReturnType<typeof useDeployAgent>['mutate']>[0];

  // Redeploys optimistically patch build_id via setQueryData rather than
  // invalidating, avoiding a refetch that could return a stale build_id while
  // the server is still reconciling. The write only runs inside onSuccess, so
  // a failed mutation never touches the cache.
  it('redeploy optimistically patches deployment build_id in cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.all(testAccount), {
      deployments: [{ name: 'code-reviewer', build_id: 'old-build' }],
      count: 1,
    });

    const { result } = renderHook(() => useDeployAgent(testAccount, 'code-reviewer'), { wrapper });
    result.current.mutate(deployPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.status).toBe('deployed');

    const cached = queryClient.getQueryData<{ deployments: { name: string; build_id: string }[] }>(
      deploymentKeys.all(testAccount),
    );
    expect(cached?.deployments[0].build_id).toBe(result.current.data?.build_id);
  });

  // Fresh installs have no existing entry to patch, so the deployments cache
  // is invalidated instead to pick up the new deployment from the server.
  it('fresh install invalidates deployments cache', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.all(testAccount), {
      deployments: [],
      count: 0,
    });

    const { result } = renderHook(() => useDeployAgent(testAccount, 'code-reviewer'), { wrapper });
    result.current.mutate(deployPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const deploymentsState = queryClient.getQueryState(deploymentKeys.all(testAccount));
    expect(deploymentsState?.isInvalidated).toBe(true);
  });

  // If the cached DeploymentsListResponse has deployments: null (e.g. the list
  // query hasn't resolved yet), .some() must not throw. The cache should be
  // invalidated so the server resolves the new state.
  it('does not throw when cached deployments is null', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(deploymentKeys.all(testAccount), {
      deployments: null,
      count: 0,
    });

    const { result } = renderHook(() => useDeployAgent(testAccount, 'code-reviewer'), { wrapper });
    result.current.mutate(deployPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.isError).toBe(false);
    const deploymentsState = queryClient.getQueryState(deploymentKeys.all(testAccount));
    expect(deploymentsState?.isInvalidated).toBe(true);
  });

  // If there is no cache entry at all, .some() must not throw either.
  it('does not throw when deployments cache is empty', async () => {
    const { wrapper } = createHookWrapper();

    const { result } = renderHook(() => useDeployAgent(testAccount, 'code-reviewer'), { wrapper });
    result.current.mutate(deployPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.isError).toBe(false);
  });
});

describe('useArchiveBlueprint', () => {
  // Successful archive invalidates the account blueprint list and the global list
  // so the profile page card disappears without a manual refresh.
  it('invalidates account and global blueprint caches on success', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(blueprintKeys.byAccount(testAccount), {
      agents: [{ name: 'code-reviewer' }],
      count: 1,
    });
    queryClient.setQueryData(blueprintKeys.all, {
      agents: [{ name: 'code-reviewer' }],
      count: 1,
    });

    const { result } = renderHook(() => useArchiveBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'code-reviewer' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(queryClient.getQueryState(blueprintKeys.byAccount(testAccount))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(blueprintKeys.all)?.isInvalidated).toBe(true);
  });

  it('surfaces error when server returns non-2xx', async () => {
    server.use(
      http.post('/api/v1/agents/:account/:name/archive', () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useArchiveBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'code-reviewer' });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useCreateBlueprint', () => {
  // Default success handler — returns the created blueprint shell.
  function useCreateBlueprintHandler(overrides?: { status?: number; body?: unknown }) {
    return http.post('/api/v1/agents/:account', async ({ request }) => {
      const body = (await request.json()) as { name: string; visibility?: string };
      if (overrides?.status && overrides.status >= 400) {
        return HttpResponse.json(overrides.body ?? { error: 'error' }, { status: overrides.status });
      }
      return HttpResponse.json({ account: testAccount, name: body.name }, { status: 201 });
    });
  }

  it('returns account and name on success', async () => {
    server.use(useCreateBlueprintHandler());

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.name).toBe('my-new-agent');
    expect(result.current.data?.account).toBe(testAccount);
  });

  it('sends visibility in the request body', async () => {
    // Use an object so TS doesn't narrow the property to `never` via closure analysis.
    const captured: { body: { name: string; visibility?: string } | null } = { body: null };
    server.use(
      http.post('/api/v1/agents/:account', async ({ request }) => {
        captured.body = (await request.json()) as { name: string; visibility?: string };
        return HttpResponse.json({ account: testAccount, name: captured.body.name }, { status: 201 });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent', visibility: 'public' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(captured.body?.visibility).toBe('public');
  });

  it('omits visibility when not provided', async () => {
    const captured: { body: { name: string; visibility?: string } | null } = { body: null };
    server.use(
      http.post('/api/v1/agents/:account', async ({ request }) => {
        captured.body = (await request.json()) as { name: string; visibility?: string };
        return HttpResponse.json({ account: testAccount, name: captured.body.name }, { status: 201 });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(captured.body?.visibility).toBeUndefined();
  });

  it('invalidates account and global blueprint caches on success', async () => {
    server.use(useCreateBlueprintHandler());

    const { wrapper, queryClient } = createHookWrapper();

    queryClient.setQueryData(blueprintKeys.byAccount(testAccount), { agents: [], count: 0 });
    queryClient.setQueryData(blueprintKeys.all, { agents: [], count: 0 });

    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(queryClient.getQueryState(blueprintKeys.byAccount(testAccount))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(blueprintKeys.all)?.isInvalidated).toBe(true);
  });

  it('surfaces conflict error when name is already taken (409)', async () => {
    server.use(useCreateBlueprintHandler({ status: 409, body: { error: 'agent "my-new-agent" already exists' } }));

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as unknown as { status: number })?.status).toBe(409);
  });

  it('surfaces server error on 500', async () => {
    server.use(useCreateBlueprintHandler({ status: 500, body: { error: 'internal_error' } }));

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useCreateBlueprint(testAccount), { wrapper });
    result.current.mutate({ name: 'my-new-agent' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toMatchObject({ error: 'internal_error' });
  });
});

// ── nameIsTaken derivation ────────────────────────────────────────────────────
//
// Mirrors the exact logic in NewBlueprint.tsx:
//   const nameIsTaken = !!existingBlueprint && (!existingBlueprint.archived_at || !!existingBlueprint.name_reserved);
//
// A blueprint name can be reclaimed only when the blueprint is archived AND name_reserved is false.
// name_reserved is set to true when: (a) the blueprint is ever made public, or (b) it is first deployed.
describe('blueprint name availability (nameIsTaken)', () => {
  function isNameTaken(blueprint: Blueprint | undefined): boolean {
    return !!blueprint && (!blueprint.archived_at || !!blueprint.name_reserved);
  }

  const base: Blueprint = { name: 'my-agent', account: 'testuser', registry: '', versions: [] };

  it('is available when no blueprint exists with that name', () => {
    expect(isNameTaken(undefined)).toBe(false);
  });

  it('is taken when an active (non-archived) blueprint exists, name_reserved = false', () => {
    // Active blueprints always block regardless of name_reserved.
    expect(isNameTaken({ ...base, name_reserved: false })).toBe(true);
  });

  it('is taken when an active blueprint exists, name_reserved = true', () => {
    expect(isNameTaken({ ...base, name_reserved: true })).toBe(true);
  });

  it('is available when the blueprint is archived and name_reserved = false (name can be reclaimed)', () => {
    // Never deployed, never public → safe to reuse. Create() will unarchive and reset it.
    expect(isNameTaken({ ...base, archived_at: '2025-01-01T00:00:00Z', name_reserved: false })).toBe(false);
  });

  it('is taken when the blueprint is archived but name_reserved = true (was ever public or deployed)', () => {
    // The name is permanently held to prevent impersonating the old blueprint's history.
    expect(isNameTaken({ ...base, archived_at: '2025-01-01T00:00:00Z', name_reserved: true })).toBe(true);
  });

  it('is available when blueprint is archived and name_reserved is undefined (legacy agent, treated as unreserved)', () => {
    // Agents created before the name_reserved column was added default to false via DB DEFAULT.
    // The Blueprint type has name_reserved?: boolean, so undefined behaves the same as false.
    expect(isNameTaken({ ...base, archived_at: '2025-01-01T00:00:00Z' })).toBe(false);
  });
});
