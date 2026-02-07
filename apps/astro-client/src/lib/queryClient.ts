import { QueryClient } from '@tanstack/react-query';
import type { ApiError } from './api';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60, // 1 minute before data is considered stale
      gcTime: 1000 * 60 * 5, // 5 minutes before inactive cache is garbage collected
      retry: (failureCount, error) => {
        // Don't retry on 4xx errors (auth failures, not found, validation, etc.)
        const apiError = error as unknown as ApiError;
        if (apiError?.code?.startsWith('4')) return false;
        return failureCount < 2;
      },
      refetchOnWindowFocus: true,
    },
    mutations: {
      retry: false,
    },
  },
});
