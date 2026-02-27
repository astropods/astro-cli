import type { QueryClientConfig } from '@tanstack/react-query';
import type { ApiError } from './api';

export const queryClientConfig: QueryClientConfig = {
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60, // 1 minute before data is considered stale
      gcTime: 1000 * 60 * 5, // 5 minutes before inactive cache is garbage collected
      retry: (failureCount, error) => {
        // Don't retry on 4xx errors (auth failures, not found, validation, etc.)
        const apiError = error as unknown as ApiError;
        if (apiError?.status && apiError.status >= 400 && apiError.status < 500) return false;
        return failureCount < 2;
      },
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: false,
    },
  },
};
