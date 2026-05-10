import { keepPreviousData, useQueries, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { observabilityKeys } from './keys';

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
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}

export function useObservabilitySummaries(deploymentIds: string[]) {
  return useQueries({
    queries: deploymentIds.map((id) => ({
      queryKey: observabilityKeys.summary(id),
      queryFn: () => api.getObservabilitySummary(id),
      enabled: !!id,
      staleTime: 0,
      gcTime: 1000 * 60 * 30,
      placeholderData: keepPreviousData,
      retry: false,
      refetchOnWindowFocus: false,
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
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
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
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
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
    // Trace details are immutable once written, so a longer staleTime keeps the
    // panel snappy when navigating between traces.
    staleTime: 1000 * 60 * 5,
    gcTime: 1000 * 60 * 30,
    retry: false,
    refetchOnWindowFocus: false,
  });
}
