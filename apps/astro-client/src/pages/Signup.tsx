import { useEffect } from "react";
import { Navigate, useSearchParams } from "react-router";
import { useAuth } from "../lib/auth";
import { api } from "../lib/api";

/**
 * Thin redirect to WorkOS signup. Passes `screen_hint=sign-up` so WorkOS
 * AuthKit opens directly on the sign-up screen instead of sign-in.
 * If the user is already authenticated, navigates to the redirect target (or /).
 */
export default function SignupRedirect() {
  const { isAuthenticated, isLoading } = useAuth();
  const [searchParams] = useSearchParams();
  const redirect = searchParams.get("redirect") || undefined;

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      window.location.replace(api.getLoginUrl(redirect, "sign-up"));
    }
  }, [isLoading, isAuthenticated, redirect]);

  if (!isLoading && isAuthenticated) {
    return <Navigate to={redirect || "/"} replace />;
  }

  return null;
}
