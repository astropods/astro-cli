import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useRecheckKnowledgeStore } from './knowledge';
import { createHookWrapper } from '@/test/test-utils';
import { knowledgeKeys } from './keys';

// ── useRecheckKnowledgeStore ─────────────────────────────────────────────────

describe('useRecheckKnowledgeStore', () => {
  beforeEach(() => {
    server.use(
      http.post('/api/v1/accounts/acme/knowledge/pg-ext/recheck', () =>
        HttpResponse.json({
          id: 'ks-1',
          arn: 'arn:knowledge:acme:pg-ext',
          name: 'pg-ext',
          provider: 'postgres',
          mode: 'external',
          status: 'ready',
          storage: '',
          public: false,
          endpoint: {
            cloud_provider: 'aws',
            endpoint_service: 'com.amazonaws.vpce.us-east-1.vpce-svc-0def',
            region: 'us-east-1',
            endpoint_id: 'vpce-0abc',
            endpoint_dns: 'vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com',
            status: 'ready',
          },
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        }),
      ),
    );
  });

  it('invalidates both the list and the affected store detail on success', async () => {
    const { wrapper, queryClient } = createHookWrapper();

    // Seed both caches so we can verify invalidation.
    queryClient.setQueryData(knowledgeKeys.all('acme'), []);
    queryClient.setQueryData(knowledgeKeys.detail('acme', 'pg-ext'), { name: 'pg-ext' });

    const { result } = renderHook(() => useRecheckKnowledgeStore('acme'), { wrapper });

    result.current.mutate({ name: 'pg-ext' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // The resolved endpoint DNS is surfaced back to the caller.
    expect(result.current.data?.endpoint?.endpoint_dns).toBe(
      'vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com',
    );
    expect(queryClient.getQueryState(knowledgeKeys.all('acme'))?.isInvalidated).toBe(true);
    expect(
      queryClient.getQueryState(knowledgeKeys.detail('acme', 'pg-ext'))?.isInvalidated,
    ).toBe(true);
  });

  it('surfaces server errors', async () => {
    server.use(
      http.post('/api/v1/accounts/acme/knowledge/pg-ext/recheck', () =>
        HttpResponse.json({ error: 'endpoint not provisioned' }, { status: 409 }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useRecheckKnowledgeStore('acme'), { wrapper });

    result.current.mutate({ name: 'pg-ext' });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
