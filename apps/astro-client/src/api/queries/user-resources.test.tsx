import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { server } from '@/test/msw/server';
import { createHookWrapper } from '@/test/test-utils';
import { useUserBlueprints } from './blueprints';
import { useUserDeployments } from './deployments';
import { useUserKnowledgeStores } from './knowledge';
import { useVisibleDeploymentSummaries } from './observability';

const allScope = { all: true, accounts: ['alpha', 'beta'] };
const selectedScope = { all: false, accounts: ['alpha', 'beta'] };

describe('visible user-resource queries', () => {
  it('loads deployments globally and requests the next keyset cursor only on demand', async () => {
    const searches: string[] = [];
    server.use(
      http.get('/api/v1/me/deployments', ({ request }) => {
        const url = new URL(request.url);
        searches.push(url.search);
        const cursor = url.searchParams.get('cursor');
        return HttpResponse.json({
          deployments: [{
            id: cursor ? 'dep-2' : 'dep-1',
            name: cursor ? 'second' : 'first',
            build_id: 'build-1',
            created_at: '2026-08-03T00:00:00Z',
            account_name: cursor ? 'beta' : 'alpha',
          }],
          page: {
            limit: 50,
            has_more: !cursor,
            ...(cursor ? {} : { next_cursor: 'deployment-cursor' }),
          },
          scope: { all: true, accounts: ['alpha', 'beta'] },
        });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useUserDeployments(allScope, { q: ' reviewer ' }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(searches).toEqual(['?scope=all&q=reviewer&limit=50']);
    expect(result.current.data?.pages).toHaveLength(1);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));

    expect(searches).toEqual([
      '?scope=all&q=reviewer&limit=50',
      '?scope=all&q=reviewer&limit=50&cursor=deployment-cursor',
    ]);
  });

  it('keeps blueprint filtering and account selection on the server', async () => {
    let params: URLSearchParams | undefined;
    server.use(
      http.get('/api/v1/me/blueprints', ({ request }) => {
        params = new URL(request.url).searchParams;
        return HttpResponse.json({
          blueprints: [],
          page: { limit: 50, has_more: false },
          scope: { all: false, accounts: ['alpha', 'beta'] },
        });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useUserBlueprints(selectedScope, { q: 'review bot', sort: 'name' }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(params?.getAll('account')).toEqual(['alpha', 'beta']);
    expect(params?.get('q')).toBe('review bot');
    expect(params?.get('sort')).toBe('name');
    expect(params?.has('scope')).toBe(false);
  });

  it('uses the same selected-account contract for knowledge stores', async () => {
    let params: URLSearchParams | undefined;
    server.use(
      http.get('/api/v1/me/knowledge', ({ request }) => {
        params = new URL(request.url).searchParams;
        return HttpResponse.json({
          stores: [],
          page: { limit: 50, has_more: false },
          scope: { all: false, accounts: ['alpha', 'beta'] },
        });
      }),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useUserKnowledgeStores(selectedScope, { q: 'memory' }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(params?.getAll('account')).toEqual(['alpha', 'beta']);
    expect(params?.get('limit')).toBe('50');
    expect(params?.get('q')).toBe('memory');
  });

  it('deduplicates, sorts, chunks, and reuses settled deployment summary batches', async () => {
    const requested: string[][] = [];
    server.use(
      http.get('/api/v1/me/deployment-summaries', ({ request }) => {
        const ids = new URL(request.url).searchParams.getAll('deployment');
        requested.push(ids);
        return HttpResponse.json({
          summaries: Object.fromEntries(ids.map((id) => [
            id,
            { total_traces: Number(id.slice(4)), last_trace_at: '2026-08-03T00:00:00Z' },
          ])),
        });
      }),
    );

    const ids = Array.from(
      { length: 205 },
      (_, index) => `dep-${String(index).padStart(3, '0')}`,
    );
    const { wrapper } = createHookWrapper();
    const { result, rerender } = renderHook(
      ({ visible }) => useVisibleDeploymentSummaries(visible),
      {
        wrapper,
        initialProps: { visible: ids.slice(0, 100) },
      },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(requested).toHaveLength(1);

    rerender({ visible: [...ids].reverse().concat('dep-001') });
    await waitFor(() => expect(result.current.data.summaries['dep-204']).toBeDefined());

    expect(requested).toHaveLength(3);
    expect(requested.every((batch) => batch.length <= 100)).toBe(true);
    expect(requested.flat().sort()).toEqual(ids);
    expect(requested.filter((batch) => batch[0] === 'dep-000')).toHaveLength(1);
    expect(Object.keys(result.current.data.summaries).sort()).toEqual(ids);
    expect(result.current.data.summaries['dep-204'].total_traces).toBe(204);
  });

  it('keeps healthy deployment summary batches when one batch fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    server.use(
      http.get('/api/v1/me/deployment-summaries', ({ request }) => {
        const ids = new URL(request.url).searchParams.getAll('deployment');
        if (ids.includes('dep-100')) {
          return HttpResponse.json({ error: 'temporary failure' }, { status: 503 });
        }
        return HttpResponse.json({
          summaries: Object.fromEntries(ids.map((id) => [id, { total_traces: 1 }])),
        });
      }),
    );

    const ids = Array.from(
      { length: 205 },
      (_, index) => `dep-${String(index).padStart(3, '0')}`,
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useVisibleDeploymentSummaries(ids), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.isError).toBe(false);
    expect(result.current.data.summaries['dep-000']).toBeDefined();
    expect(result.current.data.summaries['dep-100']).toBeUndefined();
    expect(result.current.data.summaries['dep-204']).toBeDefined();
    expect(warn).toHaveBeenCalledWith(
      'Failed to load a deployment summary batch',
      expect.objectContaining({ deploymentCount: 100 }),
    );
    warn.mockRestore();
  });
});
