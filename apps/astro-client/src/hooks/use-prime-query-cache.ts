import { useMemo } from "react";
import { useQueryClient, type InfiniteData, type QueryClient } from "@tanstack/react-query";

/**
 * Synchronously primes the React Query cache from SSR loader data so the
 * page's useQuery hooks find real entries on first mount instead of firing a
 * client fetch. Uses useMemo (not useEffect) because useEffect would run
 * AFTER the queries below have already started fetching.
 */
export function usePrimeQueryCache<T>(
  loaderData: T,
  setup: (queryClient: QueryClient, data: T) => void,
) {
  const queryClient = useQueryClient();
  // setup is intentionally excluded — re-running on every render would
  // defeat the loaderData-keyed memoization.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useMemo(() => setup(queryClient, loaderData), [queryClient, loaderData]);
}

export function firstInfinitePage<T>(page: T): InfiniteData<T, string | undefined> {
  return { pages: [page], pageParams: [undefined] };
}
