import { describe, it, expect, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { focusManager } from '@tanstack/react-query';
import { server } from '@/test/msw/server';
import {
  useBillingDailySpend,
  useRefreshBilling,
  useBillingInvoices,
  useConfirmPaymentMethod,
  useSetBillingSpendThresholds,
  useWatchInvoicePayments,
} from './billing';
import { createHookWrapper } from '@/test/test-utils';

type InvoiceListBody = { available: boolean; data: Record<string, unknown>[] };

// A backend that does not hold these controls answers 200 with available:false.
// Treating that as a save shows a success toast over a write that never
// happened, and seeds a number the next refetch silently takes away.
describe('billing threshold mutations', () => {
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
      http.put('/api/v1/accounts/acme/billing/spend/thresholds', () =>
        HttpResponse.json({ available: true }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useSetBillingSpendThresholds('acme'), { wrapper });

    result.current.mutate({ warning: null, limit: 5000 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });
});

describe('card save and the invoices table', () => {
  it('refetches invoices after a card save, because collection changes their status', async () => {
    let invoiceHits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        invoiceHits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
      http.post('/api/v1/accounts/acme/billing/payment-method', () =>
        HttpResponse.json({ available: true }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => ({
        invoices: useBillingInvoices('acme'),
        confirm: useConfirmPaymentMethod('acme'),
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.invoices.isSuccess).toBe(true));
    expect(invoiceHits).toBe(1);

    result.current.confirm.mutate('seti_1');
    await waitFor(() => expect(result.current.confirm.isSuccess).toBe(true));

    await waitFor(() => expect(invoiceHits).toBeGreaterThan(1));
  });
});

describe('watching for a payment that lands later', () => {
  it('refetches invoices when the gating status changes under it', async () => {
    let invoiceHits = 0;
    let suspended = true;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        invoiceHits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
      http.get('/api/v1/accounts/acme/billing/status', () =>
        HttpResponse.json({
          status: suspended ? 'suspended' : 'active',
          credits_exhausted: false,
          has_payment_method: true,
        }),
      ),
    );
    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(
      () => {
        useWatchInvoicePayments('acme');
        return useBillingInvoices('acme');
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const before = invoiceHits;

    suspended = false;
    await queryClient.refetchQueries({ queryKey: ['billing', 'acme', 'status'] });

    await waitFor(() => expect(invoiceHits).toBeGreaterThan(before));
  });

  it('leaves invoices alone while the status holds steady', async () => {
    let invoiceHits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        invoiceHits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
      http.get('/api/v1/accounts/acme/billing/status', () =>
        HttpResponse.json({ status: 'active', credits_exhausted: false, has_payment_method: true }),
      ),
    );
    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(
      () => {
        useWatchInvoicePayments('acme');
        return useBillingInvoices('acme');
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const before = invoiceHits;

    await queryClient.refetchQueries({ queryKey: ['billing', 'acme', 'status'] });
    await new Promise((r) => setTimeout(r, 50));

    expect(invoiceHits).toBe(before);
  });
});

describe('an unsettled charge under a gate that never moves', () => {
  function invoice(externalStatus: string) {
    return {
      available: true,
      data: [
        {
          id: 'inv-1',
          status: 'FINALIZED',
          total: 1200,
          external_invoice: { billing_provider_type: 'stripe', external_status: externalStatus },
        },
      ],
    };
  }

  it('rechecks a failed charge without any status change to ride', async () => {
    vi.useFakeTimers();
    let hits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        hits += 1;
        return HttpResponse.json(invoice('PAYMENT_FAILED'));
      }),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBillingInvoices('acme'), { wrapper });

    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(hits).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(31_000);
    });
    await vi.waitFor(() => expect(hits).toBeGreaterThan(1));
    vi.useRealTimers();
  });

  it('stops rechecking once the charge settles', async () => {
    vi.useFakeTimers();
    let hits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        hits += 1;
        return HttpResponse.json(invoice('PAID'));
      }),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBillingInvoices('acme'), { wrapper });

    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    const settled = hits;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(120_000);
    });

    expect(hits).toBe(settled);
    vi.useRealTimers();
  });
});

describe('a finalized invoice awaiting its outcome', () => {
  function finalized(external: Record<string, string> | null) {
    return {
      available: true,
      data: [{ id: 'inv-1', status: 'FINALIZED', total: 1200, external_invoice: external }],
    };
  }

  async function pollsWithin(body: InvoiceListBody, ms: number) {
    let hits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        hits += 1;
        return HttpResponse.json(body);
      }),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBillingInvoices('acme'), { wrapper });
    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    const before = hits;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });
    return hits > before;
  }

  it('waits for the outcome when a provider will report one', async () => {
    vi.useFakeTimers();
    const polled = await pollsWithin(
      finalized({ billing_provider_type: 'stripe', external_status: '' }),
      31_000,
    );
    expect(polled).toBe(true);
    vi.useRealTimers();
  });

  it('waits on nothing when no provider is connected to report one', async () => {
    vi.useFakeTimers();
    // What an unconnected environment returns: a zero external_invoice.
    const polled = await pollsWithin(
      finalized({ billing_provider_type: '', external_status: '' }),
      120_000,
    );
    expect(polled).toBe(false);
    vi.useRealTimers();
  });

  it('stops once a write-off has been recorded', async () => {
    vi.useFakeTimers();
    const polled = await pollsWithin(
      finalized({ billing_provider_type: 'stripe', external_status: 'UNCOLLECTIBLE' }),
      120_000,
    );
    expect(polled).toBe(false);
    vi.useRealTimers();
  });
});

describe('provider states that never move', () => {
  function hitsFor(externalStatus: string) {
    return {
      available: true,
      data: [
        {
          id: 'inv-1',
          status: 'FINALIZED',
          total: 1200,
          external_invoice: { billing_provider_type: 'stripe', external_status: externalStatus },
        },
      ],
    };
  }

  async function pollsOver(body: InvoiceListBody, ms: number) {
    let hits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        hits += 1;
        return HttpResponse.json(body);
      }),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(() => useBillingInvoices('acme'), { wrapper });
    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    const before = hits;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });
    return hits - before;
  }

  it.each(['SKIPPED', 'INVALID_REQUEST_ERROR'])(
    'does not wait on %s, which the provider will never move off',
    async (status) => {
      vi.useFakeTimers();
      expect(await pollsOver(hitsFor(status), 120_000)).toBe(0);
      vi.useRealTimers();
    },
  );

  it('stops waiting on a declined card nobody fixes, rather than polling all day', async () => {
    vi.useFakeTimers();
    // Past the 5m window, which allows at most ten rechecks at 30s.
    const polls = await pollsOver(hitsFor('PAYMENT_FAILED'), 10 * 60_000);
    expect(polls).toBeGreaterThan(0);
    expect(polls).toBeLessThanOrEqual(10);
    vi.useRealTimers();
  });
});


describe('keeping the open period fresh without polling', () => {
  const period = { from: '2026-08-01T00:00:00Z', to: '2026-09-01T00:00:00Z' };

  function countDailySpendHits() {
    const state = { hits: 0 };
    server.use(
      http.get('/api/v1/accounts/acme/billing/usage/daily-spend', () => {
        state.hits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
    );
    return state;
  }

  it('re-reads the open period on mount even inside the stale window', async () => {
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();

    const first = renderHook(() => useBillingDailySpend('acme', period, { isCurrentPeriod: true }), {
      wrapper,
    });
    await waitFor(() => expect(first.result.current.isSuccess).toBe(true));
    expect(state.hits).toBe(1);
    first.unmount();

    const second = renderHook(() => useBillingDailySpend('acme', period, { isCurrentPeriod: true }), {
      wrapper,
    });
    await waitFor(() => expect(second.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(state.hits).toBe(2));
  });

  it('does not re-read a closed period on mount', async () => {
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();

    const first = renderHook(() => useBillingDailySpend('acme', period, { isCurrentPeriod: false }), {
      wrapper,
    });
    await waitFor(() => expect(first.result.current.isSuccess).toBe(true));
    const once = state.hits;
    first.unmount();

    const second = renderHook(() => useBillingDailySpend('acme', period, { isCurrentPeriod: false }), {
      wrapper,
    });
    await waitFor(() => expect(second.result.current.isSuccess).toBe(true));
    expect(state.hits).toBe(once);
  });

  it('does not poll the open period on a timer', async () => {
    vi.useFakeTimers();
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useBillingDailySpend('acme', period, { isCurrentPeriod: true }),
      { wrapper },
    );

    await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
    const mounted = state.hits;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(300_000);
    });

    expect(state.hits).toBe(mounted);
    vi.useRealTimers();
  });

  it('re-reads on tab focus inside the stale window', async () => {
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useBillingDailySpend('acme', period, { isCurrentPeriod: true }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(state.hits).toBe(1);

    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    await waitFor(() => expect(state.hits).toBe(2));
  });

  it('leaves a closed period alone on tab focus', async () => {
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useBillingDailySpend('acme', period, { isCurrentPeriod: false }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const once = state.hits;

    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    expect(state.hits).toBe(once);
  });

  it('refetch reaches the provider inside the stale window', async () => {
    const state = countDailySpendHits();
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useBillingDailySpend('acme', period, { isCurrentPeriod: true }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const mounted = state.hits;

    await act(async () => {
      await result.current.refetch();
    });

    expect(state.hits).toBe(mounted + 1);
  });
});

describe('the header refresh button', () => {
  const period = { from: '2026-08-01T00:00:00Z', to: '2026-09-01T00:00:00Z' };

  it('re-reads every mounted billing query, not just the one it was wired to', async () => {
    let dailyHits = 0;
    let invoiceHits = 0;
    server.use(
      http.get('/api/v1/accounts/acme/billing/usage/daily-spend', () => {
        dailyHits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
      http.get('/api/v1/accounts/acme/billing/invoices', () => {
        invoiceHits += 1;
        return HttpResponse.json({ available: true, data: [] });
      }),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => ({
        daily: useBillingDailySpend('acme', period, { isCurrentPeriod: true }),
        invoices: useBillingInvoices('acme'),
        refresher: useRefreshBilling('acme'),
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.daily.isSuccess).toBe(true));
    await waitFor(() => expect(result.current.invoices.isSuccess).toBe(true));
    expect(dailyHits).toBe(1);
    expect(invoiceHits).toBe(1);

    await act(async () => {
      result.current.refresher.refresh();
    });

    await waitFor(() => expect(dailyHits).toBe(2));
    await waitFor(() => expect(invoiceHits).toBe(2));
  });

  it('reports nothing in flight once the reads settle', async () => {
    server.use(
      http.get('/api/v1/accounts/acme/billing/usage/daily-spend', () =>
        HttpResponse.json({ available: true, data: [] }),
      ),
    );
    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => ({
        daily: useBillingDailySpend('acme', period, { isCurrentPeriod: true }),
        refresher: useRefreshBilling('acme'),
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.daily.isSuccess).toBe(true));
    await waitFor(() => expect(result.current.refresher.isRefreshing).toBe(false));
  });
});
