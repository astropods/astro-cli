import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import {
  BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  blueprintListParamsKey,
  hasBlueprintListFilters,
  type BlueprintListParams,
} from '@/lib/blueprint-list-params';
import { blueprintKeys } from '@/api/queries/keys';

export function useAccountBlueprintsList(
  account: string,
  opts: {
    enabled?: boolean;
    params?: BlueprintListParams;
    page: number;
  },
) {
  const enabled = opts.enabled ?? true;
  const filterParams = blueprintListParamsKey(opts.params);
  const pageSize = filterParams.limit ?? BLUEPRINT_LIST_DEFAULT_PAGE_SIZE;
  const offset = Math.max(0, (opts.page - 1) * pageSize);
  const queryParams: BlueprintListParams = { ...filterParams, limit: pageSize, offset };

  return useQuery({
    queryKey: blueprintKeys.list(account, queryParams),
    queryFn: () => api.listAccountBlueprints(account, queryParams),
    enabled: !!account && enabled,
    placeholderData: hasBlueprintListFilters(filterParams) ? undefined : keepPreviousData,
  });
}
