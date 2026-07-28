import {
  useEffect,
  useState,
  useCallback,
  useRef,
  type ReactNode,
} from 'react';
import { api, type AuthResponse, ApiRequestError } from './api';
import { AuthContext, initialAuthState, type AuthState } from './auth-context';
import { clearPageFilterStorage } from './persistent-storage';

interface AuthProviderProps {
  children: ReactNode;
  serverAuth?: AuthResponse | null;
}

const PAGE_FILTER_IDENTITY_KEY = 'astro:page-filter-owner';

function establishPageFilterIdentity(
  response: AuthResponse,
  pageFiltersCleared = false,
) {
  if (typeof window === 'undefined') return;

  try {
    const identity = JSON.stringify([
      response.user.id,
      response.organization_id ?? null,
    ]);
    const previousIdentity = localStorage.getItem(PAGE_FILTER_IDENTITY_KEY);
    if (
      !pageFiltersCleared &&
      previousIdentity !== null &&
      previousIdentity !== identity
    ) {
      clearPageFilterStorage();
    }
    localStorage.setItem(PAGE_FILTER_IDENTITY_KEY, identity);
  } catch {
    // Keep authentication usable when browser storage is unavailable.
  }
}

function deriveAuthState(response: AuthResponse): AuthState {
  const accounts = response.accounts || [];
  return {
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
    refreshVersion: 0,
  };
}

export function AuthProvider({ children, serverAuth }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>(() =>
    serverAuth ? deriveAuthState(serverAuth) : initialAuthState,
  );
  const hydratedRef = useRef(!!serverAuth);

  // Note: the durable "returning user" marker (astro_returning) is set
  // server-side alongside the session cookie in astro-server (handlers/auth.go),
  // scoped to the same domain but non-HttpOnly so the marketing nav can read it.

  const updateFromResponse = useCallback(
    (
      response: AuthResponse,
      { isRefresh = false, pageFiltersCleared = false } = {},
    ) => {
      const accounts = response.accounts || [];
      establishPageFilterIdentity(response, pageFiltersCleared);

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

  // Deduplicate concurrent checkAuth calls — when a tab regains focus,
  // both the visibility handler and QueryAuthSync (reacting to 401s from
  // stale TanStack Query refetches) can call checkAuth simultaneously.
  // With WorkOS refresh token rotation, concurrent /me requests would race
  // on the same refresh token, causing one to fail and log the user out.
  const checkAuthPromiseRef = useRef<Promise<void> | null>(null);

  const checkAuth = useCallback(async () => {
    if (checkAuthPromiseRef.current) {
      return checkAuthPromiseRef.current;
    }

    const promise = (async () => {
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
        const error = err as ApiRequestError;
        // Not authenticated is not an error state, just means user needs to log in
        if (
          error.code === 'unauthorized' ||
          error.code === 'session_invalid' ||
          error.code === 'session_expired'
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
              error.message ||
              error.code ||
              'Failed to check authentication',
          });
        }
      } finally {
        checkAuthPromiseRef.current = null;
      }
    })();

    checkAuthPromiseRef.current = promise;
    return promise;
  }, [updateFromResponse]);

  const doRefreshSession = useCallback(async (opts: { isRefresh: boolean }) => {
    try {
      const response = await api.refreshSession();
      updateFromResponse(response, opts);
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

  const refresh = useCallback(() => doRefreshSession({ isRefresh: true }), [doRefreshSession]);

  // Refreshes user/account data (display name, avatar, etc.) without bumping
  // refreshVersion, so QueryAuthSync does not invalidate all queries.
  // Use this after profile edits instead of refresh().
  const refreshUserData = useCallback(() => doRefreshSession({ isRefresh: false }), [doRefreshSession]);

  const patchAccount = useCallback(
    (accountName: string, patch: Partial<AuthState['accounts'][number]>) => {
      setState((prev) => ({
        ...prev,
        accounts: prev.accounts.map((account) =>
          account.name === accountName ? { ...account, ...patch } : account,
        ),
      }));
    },
    [],
  );

  const switchOrg = useCallback(async (organizationId: string) => {
    try {
      const response = await api.switchOrg(organizationId);
      clearPageFilterStorage();
      updateFromResponse(response, {
        isRefresh: true,
        pageFiltersCleared: true,
      });
    } catch (err) {
      if (err instanceof ApiRequestError && err.status === 401 && err.code === 'session_expired') {
        clearPageFilterStorage();
        window.location.replace(api.getLoginUrl(window.location.pathname + window.location.search));
        return;
      }
      throw err;
    }
  }, [updateFromResponse]);

  const login = useCallback(() => {
    // Redirect to the server's login endpoint which will redirect to WorkOS
    window.location.replace(api.getLoginUrl());
  }, []);

  const logout = useCallback(() => {
    clearPageFilterStorage();
    // Redirect to the server's logout endpoint
    window.location.href = api.getLogoutUrl();
  }, []);

  const hydrateAuth = useCallback((response: AuthResponse) => {
    if (hydratedRef.current) return;
    hydratedRef.current = true;
    updateFromResponse(response);
  }, [updateFromResponse]);

  useEffect(() => {
    if (serverAuth) establishPageFilterIdentity(serverAuth);
  }, [serverAuth]);

  // Check authentication on mount — skip if already hydrated from server.
  useEffect(() => {
    if (!hydratedRef.current) {
      checkAuth();
    }
  }, [checkAuth]);

  // Re-validate session when tab becomes visible or window gains focus.
  // The server refreshes tokens transparently via /me when needed.
  useEffect(() => {
    if (!state.isAuthenticated) return;

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        checkAuth();
      }
    };

    const handleFocus = () => {
      checkAuth();
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);
    window.addEventListener('focus', handleFocus);

    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('focus', handleFocus);
    };
  }, [state.isAuthenticated, checkAuth]);

  const value = {
    ...state,
    login,
    logout,
    refresh,
    refreshUserData,
    patchAccount,
    checkAuth,
    switchOrg,
    hydrateAuth,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
