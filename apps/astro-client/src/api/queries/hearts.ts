import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { agentKeys } from './keys';

export function useToggleHeart(account: string, name: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.toggleHeart(account, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(account, name) });
      queryClient.invalidateQueries({ queryKey: agentKeys.all });
      queryClient.invalidateQueries({ queryKey: agentKeys.byAccount(account) });
    },
  });
}
