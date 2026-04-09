import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { AccountPublic, CreateAccountData } from '../../lib/api';
import { accountKeys } from './keys';

export function useAccountMembers(account: string) {
  return useQuery({
    queryKey: accountKeys.members(account),
    queryFn: () => api.getAccountMembers(account),
    enabled: !!account,
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

// No query invalidation here — callers refresh the session via useAuth().refresh()
// which updates the session cookie with fresh user data from WorkOS.
export function useRenameAccount() {
  return useMutation({
    mutationFn: ({ account, newName }: { account: string; newName: string }) =>
      api.renameAccount(account, newName),
  });
}

// No query invalidation here — callers refresh the session via useAuth().refresh()
// which updates the session cookie with fresh user data from WorkOS.
export function useUpdateProfile() {
  return useMutation({
    mutationFn: (data: { display_name: string }) =>
      api.updateProfile(data),
  });
}

// Avatar mutations — callers refresh the session via useAuth().refresh()
export function useUploadAvatar() {
  return useMutation({
    mutationFn: ({ account, file }: { account: string; file: Blob }) =>
      api.uploadAvatar(account, file),
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

// No query invalidation — callers refresh the session via useAuth().refresh()
export function useUpdateAccountDisplayName() {
  return useMutation({
    mutationFn: ({ account, displayName }: { account: string; displayName: string }) =>
      api.updateAccountDisplayName(account, displayName),
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
      queryClient.invalidateQueries({ queryKey: accountKeys.invitations(variables.account) });
    },
  });
}

export function useInvitations(account: string) {
  return useQuery({
    queryKey: accountKeys.invitations(account),
    queryFn: () => api.listInvitations(account),
    enabled: !!account,
  });
}

export function useCreateInvitations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, invitations }: { account: string; invitations: { value: string; kind: 'email' | 'account'; role: string }[] }) =>
      api.createInvitations(account, invitations),
    onSuccess: (_data, variables) => {
      // Invalidate both: WorkOS may auto-accept for existing users, adding them as members directly
      queryClient.invalidateQueries({ queryKey: accountKeys.invitations(variables.account) });
      queryClient.invalidateQueries({ queryKey: accountKeys.members(variables.account) });
    },
  });
}

export function useRevokeInvitation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, invitationId }: { account: string; invitationId: string }) =>
      api.revokeInvitation(account, invitationId),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: accountKeys.invitations(variables.account) });
      queryClient.invalidateQueries({ queryKey: accountKeys.members(variables.account) });
    },
  });
}
