import { keepPreviousData, useQueries, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { observabilityKeys } from './keys';

const ACTIVITY_QUERY_OPTS = {
  staleTime: 1000 * 60 * 5,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

const LIVE_QUERY_OPTS = {
  staleTime: 0,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

function buildDateParams(from?: string, to?: string): Record<string, string> {
  const p: Record<string, string> = {};
  if (from) p.from = from;
  if (to) p.to = to;
  return p;
}

export function useAccountActivitySummary(
  account: string,
  from?: string,
  to?: string,
  opts?: { groupBy?: 'user'; enabled?: boolean },
) {
  const groupBy = opts?.groupBy;
  return useQuery({
    queryKey: observabilityKeys.activitySummary(account, from, to, groupBy),
    queryFn: () => {
      const p = buildDateParams(from, to);
      if (groupBy) p.group_by = groupBy;
      return api.getAccountObservabilitySummary(account, p);
    },
    enabled: (opts?.enabled ?? true) && !!account,
    ...ACTIVITY_QUERY_OPTS,
  });
}

export function useBlueprintsSummary(
  account: string,
  from?: string,
  to?: string,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.blueprintsSummary(account, from, to),
    queryFn: () => api.getAccountBlueprintsSummary(account, buildDateParams(from, to)),
    enabled: (opts?.enabled ?? true) && !!account,
    ...ACTIVITY_QUERY_OPTS,
  });
}

export function useUsersSummary(
  account: string,
  from?: string,
  to?: string,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.usersSummary(account, from, to),
    queryFn: () => api.getAccountUsersSummary(account, buildDateParams(from, to)),
    enabled: (opts?.enabled ?? true) && !!account,
    ...ACTIVITY_QUERY_OPTS,
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
    staleTime: 0,
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

export function useObservabilitySummaries(deploymentIds: string[]) {
  return useQueries({
    queries: deploymentIds.map((id) => ({
      queryKey: observabilityKeys.summary(id),
      queryFn: () => api.getObservabilitySummary(id),
      enabled: !!id,
      ...LIVE_QUERY_OPTS,
    })),
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
    queryKey: observabilityKeys.traces(deploymentId, opts?.window),
    queryFn: () => api.getObservabilityTraces(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
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
