import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Agent, type AgentsListResponse } from '../../lib/api';
import { agentKeys } from './keys';

function patchAgent(agent: Agent, hearted: boolean, heartCount: number): Agent {
  return { ...agent, hearted, heart_count: heartCount };
}

type Snapshot = {
  detail: Agent | undefined;
  lists: { key: readonly unknown[]; data: AgentsListResponse | undefined }[];
};

export function useToggleHeart(account: string, name: string) {
  const queryClient = useQueryClient();

  const listKeys = [agentKeys.all, agentKeys.byAccount(account)];

  function applyPatch(hearted: boolean, heartCount: number) {
    queryClient.setQueryData<Agent>(agentKeys.detail(account, name), (old) =>
      old ? patchAgent(old, hearted, heartCount) : old
    );

    for (const key of listKeys) {
      queryClient.setQueryData<AgentsListResponse>(key, (old) => {
        if (!old) return old;
        return {
          ...old,
          agents: old.agents.map((a) =>
            a.account === account && a.name === name
              ? patchAgent(a, hearted, heartCount)
              : a
          ),
        };
      });
    }
  }

  return useMutation({
    mutationFn: () => api.toggleHeart(account, name),
    onMutate: async (): Promise<Snapshot> => {
      await queryClient.cancelQueries({ queryKey: agentKeys.detail(account, name) });
      for (const key of listKeys) {
        await queryClient.cancelQueries({ queryKey: key });
      }

      const detail = queryClient.getQueryData<Agent>(agentKeys.detail(account, name));
      const lists = listKeys.map((key) => ({
        key,
        data: queryClient.getQueryData<AgentsListResponse>(key),
      }));

      const wasHearted = detail?.hearted ?? false;
      const prevCount = detail?.heart_count ?? 0;
      applyPatch(!wasHearted, wasHearted ? prevCount - 1 : prevCount + 1);

      return { detail, lists };
    },
    onError: (_err, _vars, snapshot) => {
      if (!snapshot) return;
      queryClient.setQueryData(agentKeys.detail(account, name), snapshot.detail);
      for (const { key, data } of snapshot.lists) {
        queryClient.setQueryData(key, data);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(account, name) });
      for (const key of listKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
}
