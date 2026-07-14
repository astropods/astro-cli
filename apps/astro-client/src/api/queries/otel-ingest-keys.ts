import { keepPreviousData, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { otelIngestKeyKeys } from './keys';

export function useOtelIngestKeys(account: string) {
  return useQuery({
    queryKey: otelIngestKeyKeys.byAccount(account),
    queryFn: () => api.listOtelIngestKeys(account),
    enabled: !!account,
    placeholderData: keepPreviousData,
  });
}

export function useCreateOtelIngestKey(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.createOtelIngestKey(account, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: otelIngestKeyKeys.byAccount(account) });
    },
  });
}

export function useRevokeOtelIngestKey(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (keyId: string) => api.revokeOtelIngestKey(account, keyId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: otelIngestKeyKeys.byAccount(account) });
    },
  });
}
