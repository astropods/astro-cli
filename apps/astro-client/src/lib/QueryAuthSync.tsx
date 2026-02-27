import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { ApiError } from './api';
import { useAuth } from './use-auth';

function isAuthError(error: unknown): boolean {
  const apiError = error as ApiError | undefined;
  if (apiError?.status === 401) return true;
  const code = apiError?.error;
  return code === 'unauthorized' || code === 'session_invalid' || code === 'session_expired';
}

/**
 * Bridges TanStack Query and the auth layer:
 * 1. Detects 401 / auth errors from queries and triggers re-authentication.
 * 2. After a successful token refresh (refreshVersion bump), invalidates all
 *    queries so they re-fetch with the fresh token.
 */
export function QueryAuthSync() {
  const queryClient = useQueryClient();
  const { checkAuth, isAuthenticated, refreshVersion } = useAuth();
  const checkingRef = useRef(false);
  const prevRefreshVersion = useRef(refreshVersion);

  // Subscribe to query cache errors — trigger checkAuth on 401s
  useEffect(() => {
    const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== 'updated' || event.action.type !== 'error') return;
      if (!isAuthError(event.action.error)) return;
      if (checkingRef.current) return;

      checkingRef.current = true;
      checkAuth().finally(() => {
        checkingRef.current = false;
      });
    });

    return unsubscribe;
  }, [queryClient, checkAuth]);

  // When refreshVersion increments and user is authenticated, invalidate all
  // queries so they refetch with the fresh credentials.
  useEffect(() => {
    if (refreshVersion > prevRefreshVersion.current && isAuthenticated) {
      queryClient.invalidateQueries();
    }
    prevRefreshVersion.current = refreshVersion;
  }, [refreshVersion, isAuthenticated, queryClient]);

  return null;
}
