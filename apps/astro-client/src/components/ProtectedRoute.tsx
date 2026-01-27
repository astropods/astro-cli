import { useEffect, type ReactNode } from "react";
import { useAuth } from "../lib/auth";

interface ProtectedRouteProps {
  children: ReactNode;
}

/**
 * ProtectedRoute component that requires authentication.
 * Automatically redirects to login if user is not authenticated.
 * Renders nothing while checking authentication.
 */
export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isLoading, isAuthenticated, login } = useAuth();

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

  return <>{children}</>;
}

