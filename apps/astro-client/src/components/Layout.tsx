import { useEffect, useState } from "react";
import { Outlet, useSearchParams } from "react-router";
import { AppHeader } from "./AppHeader";
import { useAuth } from "../lib/auth";
import { api } from "../lib/api";
import { ActiveAccountProvider } from "@/hooks/use-active-account";

const AUTH_RETRY_KEY = "auth_invalid_state_retry";

export default function Layout() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { error: authError, isAuthenticated } = useAuth();
  const [callbackError, setCallbackError] = useState<string | null>(null);

  // Clear retry counter on successful authentication
  useEffect(() => {
    if (isAuthenticated) {
      sessionStorage.removeItem(AUTH_RETRY_KEY);
    }
  }, [isAuthenticated]);

  // Handle error from OAuth callback
  useEffect(() => {
    const errorParam = searchParams.get("error");
    const errorDesc = searchParams.get("error_description");

    if (errorParam) {
      // Clear the error params from URL
      const newParams = new URLSearchParams(searchParams);
      newParams.delete("error");
      newParams.delete("error_description");
      setSearchParams(newParams, { replace: true });

      // Auto-retry login on stale CSRF state (e.g. user sat on login page too long)
      if (errorParam === "invalid_state") {
        const retryCount = parseInt(
          sessionStorage.getItem(AUTH_RETRY_KEY) || "0",
          10,
        );
        if (retryCount < 1) {
          sessionStorage.setItem(AUTH_RETRY_KEY, String(retryCount + 1));
          window.location.replace(api.getLoginUrl());
          return;
        }
        sessionStorage.removeItem(AUTH_RETRY_KEY);
      }

      setCallbackError(errorDesc || errorParam);
    }
  }, [searchParams, setSearchParams]);

  // Auto-dismiss callback error after 8 seconds
  useEffect(() => {
    if (!callbackError) return;
    const timer = setTimeout(() => setCallbackError(null), 8000);
    return () => clearTimeout(timer);
  }, [callbackError]);

  const displayError = authError || callbackError;

  return (
    <ActiveAccountProvider>
      <div className="flex min-h-dvh flex-col bg-muted">
        <AppHeader />
        {displayError && (
          <div
            className="m-6 mb-0 md:m-8 md:mb-0 p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded cursor-pointer"
            onClick={() => setCallbackError(null)}
          >
            {displayError}
          </div>
        )}
        <Outlet />
      </div>
    </ActiveAccountProvider>
  );
}
