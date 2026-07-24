import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Blueprint, type BlueprintsListResponse } from '../../lib/api';
import { allAccountKeys, blueprintKeys, heartKeys } from './keys';

function patchBlueprint(blueprint: Blueprint, hearted: boolean, heartCount: number): Blueprint {
  return { ...blueprint, hearted, heart_count: heartCount };
}

function patchBlueprintListResponse(
  data: BlueprintsListResponse,
  account: string,
  name: string,
  hearted: boolean,
  heartCount: number,
): BlueprintsListResponse {
  return {
    ...data,
    agents: data.agents.map((a) =>
      a.account === account && a.name === name
        ? patchBlueprint(a, hearted, heartCount)
        : a,
    ),
  };
}

function isBlueprintListResponse(data: unknown): data is BlueprintsListResponse {
  return !!data && typeof data === 'object' && 'agents' in data && Array.isArray((data as BlueprintsListResponse).agents);
}

type Snapshot = {
  detail: Blueprint | undefined;
  lists: { key: readonly unknown[]; data: BlueprintsListResponse | undefined }[];
  paginatedLists: { key: readonly unknown[]; data: BlueprintsListResponse | undefined }[];
};

export function useHeartToggleInList() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, name }: { account: string; name: string }) =>
      api.toggleHeart(account, name),
    onSettled: (_data, _err, { account, name }) => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, name) });
    },
  });
}

export function useHeartedBlueprints(account: string, cursor?: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: [...heartKeys.byAccount(account), cursor ?? ''],
    queryFn: () => api.listHearted(account, cursor),
    enabled: (opts?.enabled ?? true) && !!account,
    placeholderData: keepPreviousData,
  });
}

export function useToggleHeart(account: string, name: string) {
  const queryClient = useQueryClient();

  const listKeys = [blueprintKeys.all, blueprintKeys.byAccount(account)];

  function applyPatch(hearted: boolean, heartCount: number) {
    queryClient.setQueryData<Blueprint>(blueprintKeys.detail(account, name), (old) =>
      old ? patchBlueprint(old, hearted, heartCount) : old,
    );

    for (const key of listKeys) {
      queryClient.setQueryData<BlueprintsListResponse>(key, (old) => {
        if (!old) return old;
        return patchBlueprintListResponse(old, account, name, hearted, heartCount);
      });
    }

    queryClient.setQueriesData(
      { queryKey: blueprintKeys.byAccount(account) },
      (old) => {
        if (!isBlueprintListResponse(old)) return old;
        return patchBlueprintListResponse(old, account, name, hearted, heartCount);
      },
    );
  }

  return useMutation({
    mutationFn: () => api.toggleHeart(account, name),
    onMutate: async (): Promise<Snapshot> => {
      await queryClient.cancelQueries({ queryKey: blueprintKeys.detail(account, name) });
      for (const key of listKeys) {
        await queryClient.cancelQueries({ queryKey: key });
      }
      await queryClient.cancelQueries({ queryKey: blueprintKeys.byAccount(account) });

      const detail = queryClient.getQueryData<Blueprint>(blueprintKeys.detail(account, name));
      const lists = listKeys.map((key) => ({
        key,
        data: queryClient.getQueryData<BlueprintsListResponse>(key),
      }));
      const paginatedLists = queryClient
        .getQueriesData<BlueprintsListResponse>({ queryKey: blueprintKeys.byAccount(account) })
        .flatMap(([key, data]) => (isBlueprintListResponse(data) ? [{ key, data }] : []));

      const wasHearted = detail?.hearted ?? false;
      const prevCount = detail?.heart_count ?? 0;
      applyPatch(!wasHearted, wasHearted ? prevCount - 1 : prevCount + 1);

      return { detail, lists, paginatedLists };
    },
    onError: (_err, _vars, snapshot) => {
      if (!snapshot) return;
      queryClient.setQueryData(blueprintKeys.detail(account, name), snapshot.detail);
      for (const { key, data } of snapshot.lists) {
        queryClient.setQueryData(key, data);
      }
      for (const { key, data } of snapshot.paginatedLists) {
        queryClient.setQueryData(key, data);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, name) });
      for (const key of listKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: allAccountKeys.resource('blueprints') });
      queryClient.invalidateQueries({ queryKey: heartKeys.all });
    },
  });
}
