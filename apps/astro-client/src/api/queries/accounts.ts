import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { AccountPublic } from '../../lib/api';
import { accountKeys } from './keys';

export function useProfile() {
  return useQuery({
    queryKey: accountKeys.profile,
    queryFn: () => api.getProfile(),
  });
}

export function useAccount(name: string, opts?: { initialData?: AccountPublic }) {
  return useQuery({
    queryKey: accountKeys.detail(name),
    queryFn: () => api.getAccount(name),
    enabled: !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? Date.now() : undefined,
  });
}

export function useCheckAccountName(name: string) {
  return useQuery({
    queryKey: accountKeys.checkName(name),
    queryFn: () => api.checkAccountName(name),
    enabled: name.length >= 2,
  });
}

export function useCreateAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: { name: string; type: string }) => api.createAccount(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.profile });
    },
  });
}
