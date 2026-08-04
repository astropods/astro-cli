import { useEffect, useState } from "react";
import type { UserResourceListParams } from "@/lib/user-resource-list-params";

const DEFAULT_DEBOUNCE_MS = 300;

/** Shared debounced search state for server-paginated user resource lists. */
export function useUserResourceSearch(debounceMs = DEFAULT_DEBOUNCE_MS) {
  const [search, setSearch] = useState("");
  const [params, setParams] = useState<UserResourceListParams>({});

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const q = search.trim();
      setParams((previous) => {
        if (previous.q === (q || undefined)) return previous;
        return q ? { q } : {};
      });
    }, debounceMs);
    return () => window.clearTimeout(timer);
  }, [debounceMs, search]);

  return {
    search,
    setSearch,
    params,
    hasActiveSearch: !!params.q,
  };
}
