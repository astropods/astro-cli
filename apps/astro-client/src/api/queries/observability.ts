import { useCallback } from 'react';
import {
  keepPreviousData,
  queryOptions,
  useInfiniteQuery,
  useQueries,
  useQuery,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type DeploymentSummariesResponse,
  type InsightsQueryParams,
} from '@/lib/api';
import { useExperiments } from '@/lib/experiments';
import { observabilityKeys } from './keys';

const ACTIVITY_QUERY_OPTS = {
  staleTime: 1000 * 60 * 5,
  // Background poll every 5 min so a long-running Insights session sees fresh
  // data without depending on user interaction or tab focus. Window focus is
  // intentionally off (refresh-on-return is jarring); the timer is the safety
  // net for the "user stares at the dashboard for 30 minutes" path.
  refetchInterval: 1000 * 60 * 5,
  refetchIntervalInBackground: false,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

const INSIGHTS_QUERY_OPTS = {
  ...ACTIVITY_QUERY_OPTS,
  gcTime: 1000 * 60 * 5,
} as const;

const LIVE_QUERY_OPTS = {
  staleTime: 0,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

const VISIBLE_DEPLOYMENT_SUMMARY_BATCH_SIZE = 100;

// The insightsRollups experiment swaps the read path between the v1
// (Langfuse + Redis) and v2 (Postgres rollups) endpoints. Both are wire
// compatible, so nothing downstream branches — only the URL and the cache key
// change, and the key must change or a toggle would re-render the other path's
// cached response.
export function useAccountInsights(
  account: string,
  params?: InsightsQueryParams,
  opts?: { enabled?: boolean },
) {
  const { experiments } = useExperiments();
  const version = experiments.insightsRollups ? 'v2' : 'v1';
  return useQuery({
    queryKey: observabilityKeys.insights(account, params, version),
    queryFn: () => api.getAccountInsights(account, params, version),
    enabled: (opts?.enabled ?? true) && !!account,
    ...INSIGHTS_QUERY_OPTS,
  });
}

export function useAccountObservabilitySummary(
  account: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.accountSummary(account, opts?.window),
    queryFn: () => api.getAccountObservabilitySummary(account, params),
    enabled: (opts?.enabled ?? true) && !!account,
    staleTime: 1000 * 60 * 5,
    retry: false,
    // Without keepPreviousData, DashboardStats flashes 0s on org switch
    // because the loading prop on MetricCard was removed.
    placeholderData: keepPreviousData,
  });
}

export function useObservabilityMetrics(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.metrics(deploymentId, opts?.window),
    queryFn: () => api.getObservabilityMetrics(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}

export function useObservabilitySummaries(account: string) {
  return useQuery({
    queryKey: observabilityKeys.deploymentSummaries(account),
    queryFn: () => api.getDeploymentObservabilitySummaries(account),
    enabled: !!account,
    ...ACTIVITY_QUERY_OPTS,
  });
}

export function useVisibleDeploymentSummaries(deploymentIDs: string[]) {
  const ids = [...new Set(deploymentIDs)].sort();
  const batches = Array.from(
    { length: Math.ceil(ids.length / VISIBLE_DEPLOYMENT_SUMMARY_BATCH_SIZE) },
    (_, index) => ids.slice(
      index * VISIBLE_DEPLOYMENT_SUMMARY_BATCH_SIZE,
      (index + 1) * VISIBLE_DEPLOYMENT_SUMMARY_BATCH_SIZE,
    ),
  );

  // Each batch has its own cache key. Growing or revisiting a visible page can
  // reuse settled batches, and one rejected batch does not erase healthy data.
  return useQueries({
    queries: batches.map((batch) => queryOptions({
      queryKey: observabilityKeys.visibleDeploymentSummaries(batch),
      queryFn: async () => {
        try {
          return await api.getUserDeploymentSummaries(batch);
        } catch (error) {
          console.warn('Failed to load a deployment summary batch', {
            deploymentCount: batch.length,
            error,
          });
          throw error;
        }
      },
      ...ACTIVITY_QUERY_OPTS,
    })),
    combine: useCallback((results: UseQueryResult<DeploymentSummariesResponse>[]) => {
      const summaries: DeploymentSummariesResponse['summaries'] = {};
      for (const result of results) {
        if (result.data?.summaries) Object.assign(summaries, result.data.summaries);
      }
      const isPending = results.some((result) => result.isPending);
      return {
        data: { summaries },
        isPending,
        isFetching: results.some((result) => result.isFetching),
        isError: results.length > 0 && !isPending && results.every((result) => result.isError),
        isSuccess: results.length > 0 && !isPending && results.some((result) => result.isSuccess),
      };
    }, []),
  });
}

export function useObservabilitySummary(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.summary(deploymentId, opts?.window),
    queryFn: () => api.getObservabilitySummary(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}

export function useObservabilityTraces(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.traces(deploymentId, opts?.window, params),
    queryFn: () => api.getObservabilityTraces(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}

// TRACES_PAGE_SIZE matches the server's max traces limit; larger windows are
// assembled by paging through offsets rather than one oversized request.
export const TRACES_PAGE_SIZE = 100;

// useObservabilityTracesInfinite pages the traces endpoint by offset so callers
// can assemble a window wider than a single request allows. Flatten
// `data.pages` and call `fetchNextPage` until `hasNextPage` is false (or the
// desired count is reached).
export function useObservabilityTracesInfinite(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useInfiniteQuery({
    queryKey: observabilityKeys.tracesPaged(deploymentId, opts?.window, params),
    queryFn: ({ pageParam }) =>
      api.getObservabilityTraces(deploymentId, {
        ...params,
        limit: String(TRACES_PAGE_SIZE),
        offset: String(pageParam),
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage) => {
      const next = lastPage.offset + lastPage.limit;
      return next < lastPage.total ? next : undefined;
    },
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}

export function useObservabilityTraceUsers(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.traceUsers(deploymentId, opts?.window),
    queryFn: () => api.getObservabilityTraceUsers(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...ACTIVITY_QUERY_OPTS,
  });
}

export function useObservabilityObservationDetail(
  deploymentId: string,
  observationId: string | null | undefined,
) {
  return useQuery({
    queryKey: observabilityKeys.observationDetail(deploymentId, observationId ?? ''),
    queryFn: () => api.getObservabilityObservationDetail(deploymentId, observationId!),
    enabled: !!deploymentId && !!observationId,
    refetchOnWindowFocus: false,
    staleTime: Infinity,
    gcTime: 1000 * 60 * 30,
    retry: false,
  });
}

export function useObservabilityTraceDetail(
  deploymentId: string,
  traceId: string | null | undefined,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.traceDetail(deploymentId, traceId ?? ''),
    queryFn: () => api.getObservabilityTraceDetail(deploymentId, traceId!),
    enabled: (opts?.enabled ?? true) && !!deploymentId && !!traceId,
    ...ACTIVITY_QUERY_OPTS,
    // Trace records are immutable once written; longer staleTime keeps the panel snappy when navigating between traces.
    // placeholderData disabled: showing stale data from a different trace would be confusing.
    placeholderData: undefined,
  });
}
