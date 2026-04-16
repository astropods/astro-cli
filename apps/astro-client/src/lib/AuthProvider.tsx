import {
  useEffect,
  useState,
  useCallback,
  useRef,
  type ReactNode,
} from 'react';
import { api, type AuthResponse, type ApiError } from './api';
import { AuthContext, initialAuthState, type AuthState } from './auth-context';

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>(initialAuthState);

  const updateFromResponse = useCallback(
    (response: AuthResponse, { isRefresh = false } = {}) => {
      const accounts = response.accounts || [];

      setState((prev) => ({
        user: response.user,
        sessionId: response.session_id,
        organizationId: response.organization_id || null,
        role: response.role || null,
        permissions: response.permissions || [],
        expiresAt: response.expires_at ? new Date(response.expires_at) : null,
        isLoading: false,
        isAuthenticated: true,
        error: null,
        accounts,
        needsOnboarding: !accounts.some((a) => a.type === 'personal'),
        refreshVersion: isRefresh ? prev.refreshVersion + 1 : prev.refreshVersion,
      }));
    },
    [],
  );

  const checkAuth = useCallback(async () => {
    try {
      // Only show loading state on initial check (not yet authenticated).
      // When re-checking for an already-authenticated user (e.g. after a 401
      // from a stale token), skip the loading flag so the page stays visible.
      setState((prev) => ({
        ...prev,
        isLoading: prev.isAuthenticated ? prev.isLoading : true,
        error: null,
      }));
      const response = await api.getCurrentUser();
      updateFromResponse(response);
    } catch (err) {
      const error = err as ApiError;
      // Not authenticated is not an error state, just means user needs to log in
      if (
        error.error === 'unauthorized' ||
        error.error === 'session_invalid' ||
        error.error === 'session_expired'
      ) {
        setState({
          ...initialAuthState,
          isLoading: false,
        });
      } else {
        setState({
          ...initialAuthState,
          isLoading: false,
          error:
            error.error_description ||
            error.error ||
            'Failed to check authentication',
        });
      }
    }
  }, [updateFromResponse]);

  const refresh = useCallback(async () => {
    try {
      const response = await api.refreshSession();
      updateFromResponse(response, { isRefresh: true });
    } catch (err) {
      // Network failures (e.g. offline) — don't log the user out, the next
      // visibility/focus event will retry.
      if (err instanceof TypeError) return;

      // Server-confirmed errors — mark as unauthenticated so ProtectedLayout
      // redirects to login without flashing an error banner.
      setState({
        ...initialAuthState,
        isLoading: false,
      });
    }
  }, [updateFromResponse]);

  const switchOrg = useCallback(async (organizationId: string) => {
    const response = await api.switchOrg(organizationId);
    updateFromResponse(response, { isRefresh: true });
  }, [updateFromResponse]);

  const login = useCallback(() => {
    // Redirect to the server's login endpoint which will redirect to WorkOS
    window.location.replace(api.getLoginUrl());
  }, []);

  const logout = useCallback(() => {
    // Redirect to the server's logout endpoint
    window.location.href = api.getLogoutUrl();
  }, []);

  // Check authentication on mount
  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  // Check if token needs refresh (expiring within 5 minutes)
  const isTokenExpiringSoon = useCallback(() => {
    if (!state.expiresAt) return false;
    const fiveMinutesFromNow = Date.now() + 5 * 60 * 1000;
    return state.expiresAt.getTime() < fiveMinutesFromNow;
  }, [state.expiresAt]);

  // Track if a refresh is in progress to avoid concurrent refreshes
  const isRefreshing = useRef(false);

  // Refresh token if expiring soon
  const refreshIfNeeded = useCallback(async () => {
    if (!state.isAuthenticated || isRefreshing.current) return;
    if (isTokenExpiringSoon()) {
      isRefreshing.current = true;
      try {
        await refresh();
      } finally {
        isRefreshing.current = false;
      }
    }
  }, [state.isAuthenticated, isTokenExpiringSoon, refresh]);

  // Check token freshness when tab becomes visible or window gains focus
  // This handles the common case of users returning after being away
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        refreshIfNeeded();
      }
    };

    const handleFocus = () => {
      refreshIfNeeded();
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);
    window.addEventListener('focus', handleFocus);

    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('focus', handleFocus);
    };
  }, [refreshIfNeeded]);

  const value = {
    ...state,
    login,
    logout,
    refresh,
    checkAuth,
    switchOrg,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
