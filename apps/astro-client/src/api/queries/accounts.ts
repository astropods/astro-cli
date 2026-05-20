import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { AccountPublic, CreateAccountData, AccountMembersResponse, AccountOrgsResponse } from '../../lib/api';
import { accountKeys } from './keys';

export function useAccountMembers(account: string, opts?: { includePending?: boolean; enabled?: boolean }) {
  return useQuery({
    queryKey: opts?.includePending ? accountKeys.pendingMembers(account) : accountKeys.members(account),
    queryFn: () => api.getAccountMembers(account, opts),
    enabled: (opts?.enabled ?? true) && !!account,
    placeholderData: keepPreviousData,
  });
}

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
    placeholderData: keepPreviousData,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useCheckAccountName(name: string) {
  return useQuery({
    queryKey: accountKeys.checkName(name),
    queryFn: () => api.checkAccountName(name),
    enabled: name.length >= 2,
  });
}

export function useSearchAccounts(query: string, opts?: { type?: 'personal' | 'organization' }) {
  return useQuery({
    queryKey: accountKeys.search(query, opts?.type),
    queryFn: () => api.searchAccounts(query, { type: opts?.type }),
    enabled: query.length >= 3,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  });
}

export function useCreateAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateAccountData) => api.createAccount(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.profile });
    },
  });
}

export function useDeleteAccount() {
  return useMutation({
    mutationFn: (account: string) => api.deleteAccount(account),
  });
}

export function useRenameAccount() {
  return useMutation({
    mutationFn: ({ account, newName }: { account: string; newName: string }) =>
      api.renameAccount(account, newName),
  });
}

export function useUpdateProfile(account?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { display_name: string }) =>
      api.updateProfile(data),
    onSuccess: () => {
      if (account) {
        queryClient.invalidateQueries({ queryKey: accountKeys.detail(account) });
      }
    },
  });
}

export function useUploadAvatar() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, file }: { account: string; file: Blob }) =>
      api.uploadAvatar(account, file),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.detail(variables.account) });
    },
  });
}

export function useSetAvatarPreset() {
  return useMutation({
    mutationFn: ({ account, index }: { account: string; index: number }) =>
      api.setAvatarPreset(account, index),
  });
}

export function useResetAvatar() {
  return useMutation({
    mutationFn: (account: string) => api.resetAvatar(account),
  });
}

export function useUpdateAccountProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, ...data }: { account: string; bio?: string; location?: string; email?: string; local_timezone?: string; pronouns?: string; website?: string; social_links?: string[]; blueprint_order?: string[] }) =>
      api.updateAccountProfile(account, data),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.detail(variables.account) });
    },
  });
}

export function useAccountOrgs(account: string, opts?: { enabled?: boolean }) {
  return useQuery<AccountOrgsResponse>({
    queryKey: accountKeys.orgs(account),
    queryFn: () => api.getAccountOrgs(account),
    enabled: (opts?.enabled ?? true) && !!account,
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  });
}

export function useUpdateAccountDisplayName() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, displayName }: { account: string; displayName: string }) =>
      api.updateAccountDisplayName(account, displayName),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.detail(variables.account) });
    },
  });
}

export function useUpdateMemberRole() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, userId, role }: { account: string; userId: string; role: string }) =>
      api.updateMemberRole(account, userId, role),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.members(variables.account) });
    },
  });
}

export function useRemoveAccountMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, userId }: { account: string; userId: string }) =>
      api.removeAccountMember(account, userId),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.members(variables.account) });
    },
  });
}

export function useCreateInvitations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, invitations }: { account: string; invitations: { value: string; kind: 'email' | 'account'; role: string }[] }) =>
      api.createInvitations(account, invitations),
    onMutate: async (variables) => {
      const key = accountKeys.pendingMembers(variables.account);
      await queryClient.cancelQueries({ queryKey: key });
      const previous = queryClient.getQueryData<AccountMembersResponse>(key);

      const now = new Date().toISOString();
      const optimistic = variables.invitations.map((inv, i) => ({
        account_id: '',
        user_id: `optimistic-${Date.now()}-${i}`,
        role: inv.role || 'member',
        status: 'pending',
        username: inv.kind === 'account' ? inv.value : '',
        display_name: inv.value,
        created_at: now,
        slack_workspaces: [],
      }));

      queryClient.setQueryData<AccountMembersResponse>(
        key,
        (old) => ({ members: [...(old?.members ?? []), ...optimistic] }),
      );

      return { previous };
    },
    onError: (_err, variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(accountKeys.pendingMembers(variables.account), context.previous);
      }
    },
    onSettled: (_data, _err, variables) => {
      // TanStack Query uses prefix matching: invalidating ['accounts', x, 'members']
      // also invalidates ['accounts', x, 'members', 'include-pending'] (pendingMembers key).
      queryClient.invalidateQueries({ queryKey: accountKeys.members(variables.account) });
    },
  });
}

