import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { useSetBillingUsageThresholds, useSetBillingSpendThresholds } from './billing';
import { createHookWrapper } from '@/test/test-utils';

// A backend that does not hold these controls answers 200 with available:false.
// Treating that as a save shows a success toast over a write that never
// happened, and seeds a number the next refetch silently takes away.
describe('billing threshold mutations', () => {
  it('fails a usage save the backend reports it did not make', async () => {
    server.use(
      http.put('/api/v1/accounts/acme/billing/usage/thresholds', () =>
        HttpResponse.json({ available: false }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSetBillingUsageThresholds('acme'), { wrapper });

    result.current.mutate({ metric: 'compute', warning: null, limit: 10 });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('fails a spend save the backend reports it did not make', async () => {
    server.use(
      http.put('/api/v1/accounts/acme/billing/spend/thresholds', () =>
        HttpResponse.json({ available: false }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSetBillingSpendThresholds('acme'), { wrapper });

    result.current.mutate({ warning: null, limit: 5000 });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });

  it('succeeds when the backend confirms the write', async () => {
    server.use(
      http.put('/api/v1/accounts/acme/billing/usage/thresholds', () =>
        HttpResponse.json({ available: true }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSetBillingUsageThresholds('acme'), { wrapper });

    result.current.mutate({ metric: 'compute', warning: null, limit: 10 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});
