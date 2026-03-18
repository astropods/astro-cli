import { useQuery } from '@tanstack/react-query';
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
    retry: false,
  });
}
