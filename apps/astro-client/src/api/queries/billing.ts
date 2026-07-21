import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
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
    },
  });
}

export function useDeletePaymentMethod(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.deletePaymentMethod(account),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: billingKeys.paymentMethod(account) });
    },
  });
}
