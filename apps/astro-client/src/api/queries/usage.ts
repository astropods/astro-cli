import { keepPreviousData, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { usageKeys } from './keys';
import type { QuotaIncreaseInput } from '../../lib/api';

export function useAccountUsage(account: string) {
  return useQuery({
    queryKey: usageKeys.byAccount(account),
    queryFn: () => api.getAccountUsage(account),
    enabled: !!account,
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  });
}

export function useQuotaIncreaseRequests(account: string) {
  return useQuery({
    queryKey: usageKeys.quotaRequests(account),
    queryFn: () => api.listQuotaIncreaseRequests(account),
    enabled: !!account,
    placeholderData: keepPreviousData,
  });
}

export function useRequestQuotaIncrease(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: QuotaIncreaseInput) =>
      api.requestQuotaIncrease(account, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: usageKeys.quotaRequests(account) });
    },
  });
}
