import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type {
  BillingDataResponse,
  BillingInvoice,
  BillingSpend,
  SpendThresholdsInput,
} from '../../lib/api';
import { billingKeys } from './keys';
import {
  EXTERNAL_DELETED,
  EXTERNAL_INVALID_REQUEST,
  EXTERNAL_PAID,
  EXTERNAL_SKIPPED,
  EXTERNAL_UNCOLLECTIBLE,
  EXTERNAL_VOID,
  INVOICE_FINALIZED,
  normalizeStatus,
} from '../../lib/billing-provider';
import { downloadBlob } from '../../lib/download';

// `enabled` also waits on the period window (resolved by a second query);
// firing early bills a provider call for a window nothing renders.
export function useBillingUsage(
  account: string,
  params?: { from?: string; to?: string },
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: billingKeys.usage(account, params?.from, params?.to),
    queryFn: () => api.getBillingUsage(account, params),
    enabled: !!account && (options?.enabled ?? true),
    staleTime: 60_000,
  });
}

// Same enabled-gating as useBillingUsage: the period window comes from a
// second query, and firing early bills a provider call for a window
// nothing renders.
export function useBillingDailySpend(
  account: string,
  params?: { from?: string; to?: string },
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: billingKeys.dailySpend(account, params?.from, params?.to),
    queryFn: () => api.getBillingDailySpend(account, params),
    enabled: !!account && (options?.enabled ?? true),
    staleTime: 60_000,
  });
}

// An empty status is not here: that is where a finalized invoice waits.
const SETTLED_PAYMENT = new Set([
  EXTERNAL_PAID,
  EXTERNAL_VOID,
  EXTERNAL_DELETED,
  EXTERNAL_UNCOLLECTIBLE,
  EXTERNAL_SKIPPED,
  EXTERNAL_INVALID_REQUEST,
]);

// Without a provider the status stays empty forever.
function awaitingPayment(inv: BillingInvoice): boolean {
  if (normalizeStatus(inv.status) !== INVOICE_FINALIZED) return false;
  const external = inv.external_invoice;
  if (!external?.billing_provider_type) return false;
  return !SETTLED_PAYMENT.has(normalizeStatus(external.external_status));
}

function hasUnsettledPayment(data: BillingDataResponse<BillingInvoice[]> | undefined): boolean {
  return (data?.data ?? []).some(awaitingPayment);
}

const RECHECK_MS = 30_000;

// A declined card sits unfixed for weeks, and each recheck walks the provider's
// whole invoice list, which takes no date bound.
const RECHECK_WINDOW_MS = 5 * 60_000;

export function useBillingInvoices(account: string) {
  // Read inside refetchInterval, not held in state: no render carries the news.
  const watchingSince = useRef(Date.now());

  return useQuery({
    queryKey: billingKeys.invoices(account),
    queryFn: () => api.getBillingInvoices(account),
    enabled: !!account,
    staleTime: 60_000,
    refetchInterval: (query) =>
      Date.now() - watchingSince.current < RECHECK_WINDOW_MS &&
      hasUnsettledPayment(query.state.data)
        ? RECHECK_MS
        : false,
  });
}

/** Not a query: a one-shot action on click, like useDownloadDeploymentFile. */
export function useDownloadInvoicePdf(account: string) {
  return useMutation({
    mutationFn: async ({ invoiceId, filename }: { invoiceId: string; filename: string }) => {
      downloadBlob(await api.getInvoicePdf(account, invoiceId), filename);
    },
  });
}

/** Cached gating status — a plain DB read server-side, so a short staleTime is
 *  cheap and keeps the banner from lingering after a card is added. */
// Mounted once in the app shell, so without an interval a webhook-driven
// suspension would not surface until the window regained focus.
export function useBillingStatus(account: string) {
  return useQuery({
    queryKey: billingKeys.status(account),
    queryFn: () => api.getBillingStatus(account),
    enabled: !!account,
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

/** Refetches invoices when the gating status moves, which is the only trace of
 *  a payment landing that reaches this app. Reason is in the signature: a
 *  payment can clear dunning while another gate holds the status still. */
export function useWatchInvoicePayments(account: string) {
  const { data: status } = useBillingStatus(account);
  const qc = useQueryClient();
  const previous = useRef<string | undefined>(undefined);

  const signature = status
    ? `${status.status}:${status.reason ?? ''}:${status.has_payment_method}`
    : undefined;
  useEffect(() => {
    if (signature && previous.current && signature !== previous.current) {
      qc.invalidateQueries({ queryKey: billingKeys.invoices(account) });
    }
    previous.current = signature;
  }, [account, qc, signature]);
}

/** Current-period spend plus the account's own warning and limit. The provider
 *  is the only store for the thresholds, so this refetches after a write rather
 *  than patching a local copy that could disagree with what actually fires. */
export function useBillingSpend(account: string) {
  return useQuery({
    queryKey: billingKeys.spend(account),
    queryFn: () => api.getBillingSpend(account),
    enabled: !!account,
    staleTime: 60_000,
  });
}

/** A backend that does not hold these controls answers 200 with available:false.
 *  Reporting that as a save would show a success toast and a seeded number over
 *  a write that never happened. */
function assertWritten(result: BillingDataResponse<unknown>): void {
  if (!result.available) {
    throw new Error("Billing controls are not available for this account.");
  }
}

export function useSetBillingSpendThresholds(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (thresholds: SpendThresholdsInput) => {
      const result = await api.setBillingSpendThresholds(account, thresholds);
      assertWritten(result);
      return result;
    },
    onSuccess: (_result, thresholds) => {
      // Seed what was just written so the form keeps reading through the cache.
      // Holding the typed text locally instead would leave "50.999" on screen
      // against a stored threshold of $51.
      qc.setQueryData<BillingDataResponse<BillingSpend>>(
        billingKeys.spend(account),
        (prev) => (prev?.available && prev.data ? seedThresholds(prev, thresholds) : prev),
      );
    },
    // A refused write can still have landed one of the two controls, so the
    // read is invalidated either way. The provider is the only store for these.
    onSettled: () => {
      qc.invalidateQueries({ queryKey: billingKeys.spend(account) });
      // A limit change can lift or impose a suspension, so the banner refetches.
      qc.invalidateQueries({ queryKey: billingKeys.status(account) });
    },
  });
}

/** A cleared threshold is absent, not zero: zero is a cap at nothing. in_alarm is
 *  the provider's own evaluation, so a seeded value cannot claim it. */
function seedThresholds(
  prev: BillingDataResponse<BillingSpend>,
  thresholds: SpendThresholdsInput,
): BillingDataResponse<BillingSpend> {
  const seed = (amount: number | null) =>
    amount == null ? undefined : { amount, in_alarm: false };
  return {
    ...prev,
    data: {
      ...(prev.data as BillingSpend),
      warning: seed(thresholds.warning),
      limit: seed(thresholds.limit),
    },
  };
}

// Not a query: a SetupIntent is single-use, so this fires fresh per dialog
// open rather than being cached like a read.
export function useCreateSetupIntent(account: string) {
  return useMutation({
    mutationFn: () => api.createSetupIntent(account),
  });
}

export function usePaymentMethod(account: string) {
  return useQuery({
    queryKey: billingKeys.paymentMethod(account),
    queryFn: () => api.getPaymentMethod(account),
    enabled: !!account,
    staleTime: 60_000,
  });
}

// useConfirmPaymentMethod saves a card after Stripe.js confirms the SetupIntent.
export function useConfirmPaymentMethod(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (setupIntentId: string) =>
      api.confirmPaymentMethod(account, setupIntentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: billingKeys.paymentMethod(account) });
      // A card change flips credits_exhausted gating, so the banner must refetch.
      qc.invalidateQueries({ queryKey: billingKeys.status(account) });
      // Charging what the account owes changes the invoices' payment status.
      qc.invalidateQueries({ queryKey: billingKeys.invoices(account) });
    },
  });
}

export function useDeletePaymentMethod(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.deletePaymentMethod(account),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: billingKeys.paymentMethod(account) });
      // A card change flips credits_exhausted gating, so the banner must refetch.
      qc.invalidateQueries({ queryKey: billingKeys.status(account) });
    },
  });
}
