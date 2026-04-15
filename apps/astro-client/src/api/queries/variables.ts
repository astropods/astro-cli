import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { CreateAccountVariableInput, UpdateAccountVariableInput } from '@/lib/api';
import { variableKeys } from './keys';

export function useAccountVariables(account: string) {
  return useQuery({
    queryKey: variableKeys.byAccount(account),
    queryFn: () => api.listAccountVariables(account),
    enabled: !!account,
  });
}

export function useCreateAccountVariables(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (variables: CreateAccountVariableInput[]) =>
      api.createAccountVariables(account, variables),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: variableKeys.byAccount(account) });
    },
  });
}

export function useUpdateAccountVariable(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: UpdateAccountVariableInput }) =>
      api.updateAccountVariable(account, name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: variableKeys.byAccount(account) });
    },
  });
}

export function useDeleteAccountVariable(account: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.deleteAccountVariable(account, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: variableKeys.byAccount(account) });
    },
  });
}
