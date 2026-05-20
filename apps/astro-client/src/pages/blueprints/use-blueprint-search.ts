import { useEffect, useState } from 'react';
import {
  hasBlueprintListFilters,
  type BlueprintListParams,
} from '@/lib/blueprint-list-params';

const DEFAULT_DEBOUNCE_MS = 300;

/** Debounced search input for the /blueprints page. */
export function useBlueprintSearch(debounceMs = DEFAULT_DEBOUNCE_MS) {
  const [search, setSearch] = useState('');
  const [params, setParams] = useState<BlueprintListParams>({});

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const q = search.trim();
      setParams(q ? { q } : {});
    }, debounceMs);
    return () => window.clearTimeout(timer);
  }, [search, debounceMs]);

  const hasActiveFilters = hasBlueprintListFilters(params);

  return { search, setSearch, params, hasActiveFilters };
}
