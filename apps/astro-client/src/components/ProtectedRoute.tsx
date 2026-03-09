import { useEffect, type ReactNode } from "react";
import { Navigate } from "react-router";
import { useAuth } from "../lib/auth";

interface ProtectedRouteProps {
  children: ReactNode;
}

/**
 * ProtectedRoute component that requires authentication.
 * Automatically redirects to login if user is not authenticated.
 * Redirects to onboarding if user has no personal account.
 * Renders nothing while checking authentication.
 */
export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isLoading, isAuthenticated, login, personalAccount } = useAuth();

  // Redirect to login if not authenticated
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      login();
    }
  }, [isLoading, isAuthenticated, login]);

  // Render nothing while checking auth or redirecting
  if (isLoading || !isAuthenticated) {
    return null;
  }

  // User needs to claim their username before using the app
  if (!personalAccount) {
    return <Navigate to="/onboarding" replace />;
  }

  return <>{children}</>;
}

