import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { BillingDataResponse, BillingSpend, SpendThresholdsInput, UsageThresholds, UsageThresholdsInput } from '../../lib/api';
import { billingKeys } from './keys';

export function useBillingUsage(account: string, params?: { from?: string; to?: string }) {
  return useQuery({
    queryKey: billingKeys.usage(account, params?.from, params?.to),
    queryFn: () => api.getBillingUsage(account, params),
    enabled: !!account,
    staleTime: 60_000,
  });
}

export function useBillingInvoices(account: string) {
  return useQuery({
    queryKey: billingKeys.invoices(account),
    queryFn: () => api.getBillingInvoices(account),
    enabled: !!account,
    staleTime: 60_000,
  });
}

export function useInvoicePdf(account: string, invoiceId: string, enabled: boolean) {
  return useQuery({
    queryKey: billingKeys.invoicePdf(account, invoiceId),
    queryFn: () => api.getInvoicePdf(account, invoiceId),
    enabled: enabled && !!account && !!invoiceId,
    staleTime: 5 * 60_000,
    gcTime: 5 * 60_000,
  });
}

export function useBillingBalances(account: string) {
  return useQuery({
    queryKey: billingKeys.balances(account),
    queryFn: () => api.getBillingBalances(account),
    enabled: !!account,
    staleTime: 60_000,
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

export function useSetBillingUsageThresholds(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (thresholds: UsageThresholdsInput) => {
      const result = await api.setBillingUsageThresholds(account, thresholds);
      assertWritten(result);
      return result;
    },
    onSuccess: (_result, thresholds) => {
      qc.setQueryData<BillingDataResponse<BillingSpend>>(
        billingKeys.spend(account),
        (prev) => (prev?.available && prev.data ? seedUsageThresholds(prev, thresholds) : prev),
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: billingKeys.spend(account) });
      qc.invalidateQueries({ queryKey: billingKeys.status(account) });
    },
  });
}

/** The saved cap, seeded per metric so the row keeps its number while the
 *  refetch is in flight. The spend row does the same; a row that only
 *  invalidates shows the previous value until the read lands. */
function seedUsageThresholds(
  prev: BillingDataResponse<BillingSpend>,
  thresholds: UsageThresholdsInput,
): BillingDataResponse<BillingSpend> {
  const data = prev.data as BillingSpend;
  const seed = (amount: number | null) =>
    amount == null ? undefined : { amount, in_alarm: false };
  const held = data.usage?.[thresholds.metric];
  const seeded: UsageThresholds = {
    unit: held?.unit ?? "",
    warning: seed(thresholds.warning),
    limit: seed(thresholds.limit),
  };
  return {
    ...prev,
    data: { ...data, usage: { ...data.usage, [thresholds.metric]: seeded } },
  };
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
