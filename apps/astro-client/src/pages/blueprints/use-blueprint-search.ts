import {
  hasBlueprintListFilters,
  type BlueprintListParams,
} from '@/lib/blueprint-list-params';
import { useUserResourceSearch } from '@/hooks/use-user-resource-search';

/** Debounced search input for the /blueprints page. */
export function useBlueprintSearch(debounceMs = 300) {
  const { search, setSearch, params: searchParams } = useUserResourceSearch(debounceMs);
  const params: BlueprintListParams = searchParams;
  const hasActiveFilters = hasBlueprintListFilters(params);

  return { search, setSearch, params, hasActiveFilters };
}
