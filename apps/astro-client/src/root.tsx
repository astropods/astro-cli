import { useState, useEffect } from "react";
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
  { charSet: "utf-8" },
  { title: "Astro" },
  { name: "viewport", content: "width=device-width, initial-scale=1.0" },
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

// Temporary: blocks all unauthenticated access while the waitlist is active.
// When the waitlist is removed, delete this component and its usage in Root().
// Protected pages have their own auth via ProtectedRoute; public pages (browse,
// agent detail, account profiles) will become accessible without login.
const WAITLIST_URL = "https://blog.astropods.ai/waitlist";
const isProduction = typeof window !== "undefined" && window.location.hostname === "astropods.ai";

function WaitlistGuard({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, login } = useAuth();
  const location = useLocation();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      if (isProduction && location.pathname === "/") {
        window.location.href = WAITLIST_URL;
      } else {
        login();
      }
    }
  }, [isLoading, isAuthenticated, login, location.pathname]);

  if (isLoading || !isAuthenticated) return null;

  return <>{children}</>;
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
        <WaitlistGuard>
          <AmplitudeProvider>
            <OnboardingGuard>
              <Outlet />
            </OnboardingGuard>
          </AmplitudeProvider>
        </WaitlistGuard>
      </QueryClientProvider>
    </AuthProvider>
  );
}

export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
  let message = "Oops!";
  let details = "An unexpected error occurred.";

  if (isRouteErrorResponse(error)) {
    message = error.status === 404 ? "404" : "Error";
    details =
      error.status === 404
        ? "The requested page could not be found."
        : error.statusText || details;
  } else if (error instanceof Error) {
    details = error.message;
  }

  return (
    <main className="flex items-center justify-center min-h-screen">
      <div className="text-center">
        <h1 className="text-7xl font-extrabold mb-2">{message}</h1>
        <p className="text-stone-600 text-sm">{details}</p>
      </div>
    </main>
  );
}
