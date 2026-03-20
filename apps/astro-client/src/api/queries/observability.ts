import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { observabilityKeys } from './keys';

export function useObservabilityMetrics(
  account: string,
  name: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.metrics(account, name, params),
    queryFn: () => api.getObservabilityMetrics(account, name, params),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    staleTime: 1000 * 60 * 5,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}

export function useObservabilitySummary(
  account: string,
  name: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.summary(account, name, params),
    queryFn: () => api.getObservabilitySummary(account, name, params),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    staleTime: 1000 * 60 * 5,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}

export function useObservabilityTraces(
  account: string,
  name: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: observabilityKeys.traces(account, name, params),
    queryFn: () => api.getObservabilityTraces(account, name, params),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    staleTime: 1000 * 60 * 5,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}
