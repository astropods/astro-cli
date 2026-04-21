import { useQuery, useInfiniteQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
import type { AuditLogQueryParams, AuditLogListResponse } from '../../lib/api';
import { auditLogKeys } from './keys';

export function useAuditLog(account: string, filters?: Omit<AuditLogQueryParams, 'before'>) {
  return useInfiniteQuery<AuditLogListResponse>({
    queryKey: [...auditLogKeys.byAccount(account), filters],
    queryFn: ({ pageParam }) =>
      api.listAuditLog(account, { ...filters, before: pageParam as string | undefined }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_before : undefined,
    enabled: !!account,
  });
}

export function useAuditLogFilters(account: string) {
  return useQuery({
    queryKey: auditLogKeys.filters(account),
    queryFn: () => api.listAuditLogFilters(account),
    enabled: !!account,
    staleTime: 60_000,
  });
}
