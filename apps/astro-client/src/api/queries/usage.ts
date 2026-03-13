import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { usageKeys } from './keys';

export function useAccountUsage(account: string) {
  return useQuery({
    queryKey: usageKeys.byAccount(account),
    queryFn: () => api.getAccountUsage(account),
    enabled: !!account,
    staleTime: 60_000,
  });
}
