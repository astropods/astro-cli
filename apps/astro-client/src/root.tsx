import { useState } from "react";
import {
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  Navigate,
  useLocation,
  useMatches,
  isRouteErrorResponse,
} from "react-router";
import type { Route } from "./+types/root";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { getCurrentUserForRequest } from "./lib/api.server";
import { Toaster } from "./components/ui/toaster";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue } from "./lib/active-account";
import { AuthProvider, useAuth } from "./lib/auth";
import { AmplitudeProvider } from "./lib/AmplitudeProvider";
import { queryClientConfig } from "./lib/queryClient";
import { QueryAuthSync } from "./lib/QueryAuthSync";
import { Button } from "./components/ui/button";
import { parseCookieTheme, ServerThemeContext, DEFAULT_THEME } from "./lib/theme";

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
  const matches = useMatches();
  const rootData = matches[0]?.data as { serverTheme?: "light" | "dark" } | undefined;
  const serverTheme = rootData?.serverTheme ?? DEFAULT_THEME;

  return (
    <html lang="en" className={serverTheme === "dark" ? "dark" : undefined} suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){var t=localStorage.getItem("astro:theme")||"${DEFAULT_THEME}";if(t==="auto")t=window.matchMedia("(prefers-color-scheme:dark)").matches?"dark":"light";document.documentElement.classList.toggle("dark",t==="dark");document.cookie="astro-theme="+t+";path=/;max-age=31536000;SameSite=Lax"})()`,
          }}
        />
        <Meta />
        <Links />
      </head>
      <body>
        {children}
        <Toaster />
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

// Root revalidates only on programmatic revalidations (currentUrl === nextUrl).
// This keeps URL navigations cheap while allowing cookie migrations and other
// explicit refreshes to update the resolved active account.
export function shouldRevalidate({ currentUrl, nextUrl }: { currentUrl: URL; nextUrl: URL }) {
  return currentUrl.toString() === nextUrl.toString();
}

export async function loader({ request }: Route.LoaderArgs) {
  const cookieHeader = request.headers.get("cookie");
  const serverTheme = parseCookieTheme(cookieHeader);
  try {
    // readCookieValue can throw URIError on malformed percent-encoding —
    // keep it inside the try so the existing fallback catches it.
    const rawCookieAccount = readCookieValue(cookieHeader, ACTIVE_ACCOUNT_COOKIE);
    const serverAuth = await getCurrentUserForRequest(request);
    // Validate the cookie against the user's accounts list so a stale cookie
    // (e.g. account they no longer belong to) doesn't poison the initial UI.
    const matched =
      (rawCookieAccount && serverAuth.accounts?.find((a) => a.name === rawCookieAccount)) ||
      serverAuth.accounts?.find((a) => a.type === "personal") ||
      serverAuth.accounts?.[0];
    return { serverAuth, serverTheme, activeAccount: matched?.name ?? "" };
  } catch {
    return { serverAuth: null, serverTheme, activeAccount: "" };
  }
}

export default function Root({ loaderData }: Route.ComponentProps) {
  // Create a new QueryClient per server request (prevents data leakage between
  // users in SSR). useState ensures the client persists across re-renders.
  const [queryClient] = useState(() => new QueryClient(queryClientConfig));

  return (
    <ServerThemeContext.Provider value={loaderData?.serverTheme ?? DEFAULT_THEME}>
      <AuthProvider serverAuth={loaderData?.serverAuth}>
        <QueryClientProvider client={queryClient}>
          <QueryAuthSync />
          <AmplitudeProvider>
            <OnboardingGuard>
              <Outlet />
            </OnboardingGuard>
          </AmplitudeProvider>
        </QueryClientProvider>
      </AuthProvider>
    </ServerThemeContext.Provider>
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
        <p className="max-w-md text-muted-foreground text-sm">{details}</p>
        {showReload && (
          <Button onClick={() => window.location.reload()} className="mt-2">
            Reload page
          </Button>
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
