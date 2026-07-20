import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
} from '@tanstack/react-query';
import { api, type InsightsQueryParams } from '@/lib/api';
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

export function useAccountInsights(
  account: string,
  params?: InsightsQueryParams,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.insights(account, params),
    queryFn: () => api.getAccountInsights(account, params),
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
