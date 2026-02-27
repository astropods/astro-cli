import { useEffect, useState } from "react";
import { Outlet, useSearchParams } from "react-router";
import { AppHeader } from "./AppHeader";
import { useAuth } from "../lib/auth";

export default function Layout() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { error: authError } = useAuth();
  const [callbackError, setCallbackError] = useState<string | null>(null);

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
    <div className="flex min-h-screen flex-col">
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
  );
}
