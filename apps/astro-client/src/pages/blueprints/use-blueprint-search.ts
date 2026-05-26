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
      const next: BlueprintListParams = q ? { q } : {};
      // Preserve the previous reference when the structural value is unchanged.
      // Otherwise the initial debounce settles into a fresh `{}` on mount, which
      // ripples out to consumers (e.g. the page-reset effect on /blueprints) and
      // can race with concurrent user interactions like clicking "Page 2".
      setParams((prev) => (prev.q === next.q ? prev : next));
    }, debounceMs);
    return () => window.clearTimeout(timer);
  }, [search, debounceMs]);

  const hasActiveFilters = hasBlueprintListFilters(params);

  return { search, setSearch, params, hasActiveFilters };
}
