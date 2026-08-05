import {
  hasBlueprintListFilters,
  type BlueprintListParams,
} from '@/lib/blueprint-list-params';
import { useUserResourceSearch } from '@/hooks/use-user-resource-search';

/** Settled search term for the /blueprints page. Typing is debounced by the
 *  search box itself (see DebouncedFilterInput). */
export function useBlueprintSearch() {
  const { search, setSearch, params: searchParams } = useUserResourceSearch();
  const params: BlueprintListParams = searchParams;
  const hasActiveFilters = hasBlueprintListFilters(params);

  return { search, setSearch, params, hasActiveFilters };
}
