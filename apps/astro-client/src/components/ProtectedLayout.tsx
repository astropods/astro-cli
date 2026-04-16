import { Navigate, Outlet, useLocation } from "react-router";
import { useAuth } from "../lib/auth";

/**
 * Layout route that requires authentication. Wrap protected routes with this
 * in routes.ts so the auth check runs before any child page mounts — hooks in
 * child components are guaranteed to have an authenticated user.
 */
export default function ProtectedLayout() {
  const { isLoading, isAuthenticated } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return null;
  }

  if (!isAuthenticated) {
    const returnPath = location.pathname + location.search;
    return <Navigate to={`/login?redirect=${encodeURIComponent(returnPath)}`} replace />;
  }

  return <Outlet />;
}
