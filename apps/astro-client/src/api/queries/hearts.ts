import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Blueprint, type BlueprintsListResponse } from '../../lib/api';
import { blueprintKeys } from './keys';

function patchBlueprint(blueprint: Blueprint, hearted: boolean, heartCount: number): Blueprint {
  return { ...blueprint, hearted, heart_count: heartCount };
}

type Snapshot = {
  detail: Blueprint | undefined;
  lists: { key: readonly unknown[]; data: BlueprintsListResponse | undefined }[];
};

export function useToggleHeart(account: string, name: string) {
  const queryClient = useQueryClient();

  const listKeys = [blueprintKeys.all, blueprintKeys.byAccount(account)];

  function applyPatch(hearted: boolean, heartCount: number) {
    queryClient.setQueryData<Blueprint>(blueprintKeys.detail(account, name), (old) =>
      old ? patchBlueprint(old, hearted, heartCount) : old
    );

    for (const key of listKeys) {
      queryClient.setQueryData<BlueprintsListResponse>(key, (old) => {
        if (!old) return old;
        return {
          ...old,
          blueprints: old.blueprints.map((a) =>
            a.account === account && a.name === name
              ? patchBlueprint(a, hearted, heartCount)
              : a
          ),
        };
      });
    }
  }

  return useMutation({
    mutationFn: () => api.toggleHeart(account, name),
    onMutate: async (): Promise<Snapshot> => {
      await queryClient.cancelQueries({ queryKey: blueprintKeys.detail(account, name) });
      for (const key of listKeys) {
        await queryClient.cancelQueries({ queryKey: key });
      }

      const detail = queryClient.getQueryData<Blueprint>(blueprintKeys.detail(account, name));
      const lists = listKeys.map((key) => ({
        key,
        data: queryClient.getQueryData<BlueprintsListResponse>(key),
      }));

      const wasHearted = detail?.hearted ?? false;
      const prevCount = detail?.heart_count ?? 0;
      applyPatch(!wasHearted, wasHearted ? prevCount - 1 : prevCount + 1);

      return { detail, lists };
    },
    onError: (_err, _vars, snapshot) => {
      if (!snapshot) return;
      queryClient.setQueryData(blueprintKeys.detail(account, name), snapshot.detail);
      for (const { key, data } of snapshot.lists) {
        queryClient.setQueryData(key, data);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, name) });
      for (const key of listKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
}
