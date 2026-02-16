import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { accountKeys } from './keys';

export function useProfile() {
  return useQuery({
    queryKey: accountKeys.profile,
    queryFn: () => api.getProfile(),
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
