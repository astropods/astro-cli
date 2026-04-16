import { useEffect } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useAuth } from "../lib/auth";
import { api } from "../lib/api";

/**
 * Thin redirect to WorkOS login. Reads an optional `redirect` query param
 * so that ProtectedLayout can deep-link back to the original page after auth.
 * If the user is already authenticated, navigates to the redirect target (or /).
 */
export default function LoginRedirect() {
  const { isAuthenticated, isLoading } = useAuth();
  const [searchParams] = useSearchParams();
  const redirect = searchParams.get("redirect") || undefined;

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      window.location.replace(api.getLoginUrl(redirect));
    }
  }, [isLoading, isAuthenticated, redirect]);

  if (!isLoading && isAuthenticated) {
    return <Navigate to={redirect || "/"} replace />;
  }

  return null;
}
