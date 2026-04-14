import { useState } from "react";
import {
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  Navigate,
  useLocation,
  isRouteErrorResponse,
} from "react-router";
import type { Route } from "./+types/root";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, useAuth } from "./lib/auth";
import { AmplitudeProvider } from "./lib/AmplitudeProvider";
import { queryClientConfig } from "./lib/queryClient";
import { QueryAuthSync } from "./lib/QueryAuthSync";

export const meta: Route.MetaFunction = () => [
  { title: "Astro" },
];

export const links: Route.LinksFunction = () => [
  { rel: "apple-touch-icon", sizes: "180x180", href: "/apple-touch-icon.png" },
  { rel: "icon", type: "image/png", sizes: "32x32", href: "/favicon-32x32.png" },
  { rel: "icon", type: "image/png", sizes: "16x16", href: "/favicon-16x16.png" },
  { rel: "manifest", href: "/site.webmanifest" },
];

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <Meta />
        <Links />
      </head>
      <body>
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

function OnboardingGuard({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, needsOnboarding } = useAuth();
  const location = useLocation();

  if (isLoading) return <>{children}</>;

  if (isAuthenticated && needsOnboarding && location.pathname !== "/onboarding") {
    return <Navigate to="/onboarding" replace />;
  }

  if (isAuthenticated && !needsOnboarding && location.pathname === "/onboarding") {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

export default function Root() {
  // Create a new QueryClient per server request (prevents data leakage between
  // users in SSR). useState ensures the client persists across re-renders.
  const [queryClient] = useState(() => new QueryClient(queryClientConfig));

  return (
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <QueryAuthSync />
        <AmplitudeProvider>
          <OnboardingGuard>
            <Outlet />
          </OnboardingGuard>
        </AmplitudeProvider>
      </QueryClientProvider>
    </AuthProvider>
  );
}

export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
  let message = "Oops!";
  let details = "An unexpected error occurred.";
  let showReload = true;

  if (isRouteErrorResponse(error)) {
    message = error.status === 404 ? "404" : `${error.status}`;
    details =
      error.status === 404
        ? "The requested page could not be found."
        : error.statusText || details;
    showReload = error.status !== 404;
  } else if (error instanceof Error) {
    details = friendlyErrorMessage(error.message) ?? error.message;
  }

  return (
    <main className="flex items-center justify-center min-h-screen">
      <div className="flex flex-col items-center gap-4 text-center">
        <h1 className="text-7xl font-extrabold">{message}</h1>
        <p className="max-w-md text-stone-600 text-sm">{details}</p>
        {showReload && (
          <button
            type="button"
            onClick={() => window.location.reload()}
            className="mt-2 rounded-md bg-stone-900 px-4 py-2 text-sm font-medium text-white hover:bg-stone-800 transition-colors"
          >
            Reload page
          </button>
        )}
      </div>
    </main>
  );
}

/** Map minified React error codes to human-readable messages. */
function friendlyErrorMessage(msg: string): string | null {
  if (msg.includes("Minified React error #31") || msg.includes("Objects are not valid as a React child")) {
    return "A component tried to render a data object instead of text. This is a bug — please report it.";
  }
  if (msg.includes("Minified React error #130") || msg.includes("Element type is invalid")) {
    return "A component failed to load correctly. Try reloading the page.";
  }
  if (msg.includes("Minified React error #185") || msg.includes("Maximum update depth exceeded")) {
    return "The page got stuck in a loop. Try reloading.";
  }
  return null;
}
