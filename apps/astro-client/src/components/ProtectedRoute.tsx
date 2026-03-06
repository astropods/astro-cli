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
  const { isLoading, isAuthenticated, login, personalAccount, accounts } = useAuth();

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

  // Every authenticated user must have a personal account to use the app
  if (accounts.length > 0 && !personalAccount) {
    return (
      <div className="flex flex-col items-center justify-center flex-1 px-6 py-16">
        <h1 className="text-xl font-semibold mb-3">Account error</h1>
        <p className="text-stone-500 text-sm text-center max-w-md">
          No personal account found. This is an unexpected state &mdash; please
          contact support.
        </p>
      </div>
    );
  }

  return <>{children}</>;
}

