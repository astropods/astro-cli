import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
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
