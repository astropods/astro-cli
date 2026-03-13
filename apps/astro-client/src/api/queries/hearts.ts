import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Agent, type AgentsListResponse } from '../../lib/api';
import { agentKeys } from './keys';

function patchAgent(agent: Agent, hearted: boolean, heartCount: number): Agent {
  return { ...agent, hearted, heart_count: heartCount };
}

export function useToggleHeart(account: string, name: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.toggleHeart(account, name),
    onSuccess: (data) => {
      queryClient.setQueryData<Agent>(agentKeys.detail(account, name), (old) =>
        old ? patchAgent(old, data.hearted, data.heart_count) : old
      );

      for (const key of [agentKeys.all, agentKeys.byAccount(account)]) {
        queryClient.setQueryData<AgentsListResponse>(key, (old) => {
          if (!old) return old;
          return {
            ...old,
            agents: old.agents.map((a) =>
              a.account === account && a.name === name
                ? patchAgent(a, data.hearted, data.heart_count)
                : a
            ),
          };
        });
      }
    },
  });
}
