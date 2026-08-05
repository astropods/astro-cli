import { useState } from "react";
import type { UserResourceListParams } from "@/lib/user-resource-list-params";

/**
 * Settled search term for server-paginated user resource lists.
 *
 * The in-flight text belongs to the search box itself (see
 * DebouncedFilterInput), which reports the term only once the user stops
 * typing. Pages therefore re-render when the term settles rather than on every
 * keystroke, which keeps their result grids off the typing path.
 */
export function useUserResourceSearch() {
  const [search, setSearch] = useState("");
  const q = search.trim();
  const params: UserResourceListParams = q ? { q } : {};

  return {
    search,
    setSearch,
    params,
    hasActiveSearch: !!q,
  };
}
