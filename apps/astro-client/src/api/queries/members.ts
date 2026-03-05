import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { InviteEntry } from '../../lib/api';
import { memberKeys, invitationKeys } from './keys';

export function useAccountMembers(account: string) {
  return useQuery({
    queryKey: memberKeys.list(account),
    queryFn: () => api.getAccountMembers(account),
    enabled: !!account,
  });
}

export function useAccountInvitations(account: string) {
  return useQuery({
    queryKey: invitationKeys.list(account),
    queryFn: () => api.getAccountInvitations(account),
    enabled: !!account,
  });
}

export function useCreateInvitations(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (invitations: InviteEntry[]) => api.createInvitations(account, invitations),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: invitationKeys.list(account) });
    },
  });
}
