// Standalone, client-only build of the chat experience for the astro CLI.
//
// This entry reuses the exact same chat page and components as the deployed
// astro-client app (no fork) but mounts them without SSR, without WorkOS auth,
// and against a single synthetic "local" deployment. The astro CLI embeds the
// produced bundle (see apps/astro-cli/internal/chatui) and serves it during
// `ast dev` / `ast project`, proxying the deployment-scoped chat/messaging API
// to the local messaging sidecar — the same contract astro-server exposes in
// production, so the React code runs unchanged here.
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createBrowserRouter,
  Navigate,
  Outlet,
  RouterProvider,
} from "react-router";
import { Toaster } from "sonner";
import "../index.css";
import { AuthContext, type AuthContextType } from "@/lib/auth-context";
import { ActiveAccountProvider } from "@/hooks/use-active-account";
import { queryClientConfig } from "@/lib/queryClient";
import { DEFAULT_THEME, ServerThemeContext } from "@/lib/theme";
import ChatPage from "@/pages/chat/Chat";

// The CLI synthesizes a single account + deployment under this name; keep it in
// sync with apps/astro-cli/internal/chatui (LocalAccount / LocalDeploymentID).
const LOCAL_ACCOUNT = "local";

// Auth is bypassed locally: there is no WorkOS session in `ast dev`. We provide
// an already-authenticated context with one personal account so the shared chat
// hooks (useAuth, useActiveAccount, useChatAgents) run exactly as they do in the
// deployed app. Mutating actions (login/logout/switchOrg) are inert no-ops.
const localAuth: AuthContextType = {
  user: {
    id: "local",
    email: "local@astropods.dev",
    first_name: "Local",
    last_name: "Developer",
    email_verified: true,
    created_at: new Date(0).toISOString(),
    updated_at: new Date(0).toISOString(),
  },
  sessionId: "local",
  organizationId: null,
  role: null,
  permissions: [],
  expiresAt: null,
  isLoading: false,
  isAuthenticated: true,
  error: null,
  accounts: [{ id: LOCAL_ACCOUNT, name: LOCAL_ACCOUNT, type: "personal" }],
  needsOnboarding: false,
  refreshVersion: 0,
  login: () => {},
  logout: () => {},
  refresh: async () => {},
  refreshUserData: async () => {},
  patchAccount: () => {},
  checkAuth: async () => {},
  switchOrg: async () => {},
  hydrateAuth: () => {},
};

const queryClient = new QueryClient(queryClientConfig);

// ActiveAccountProvider calls useRevalidator()/useRouteLoaderData("root"), which
// require a data router. A root route with id "root" and a loader satisfies both.
function RootLayout() {
  return (
    <ActiveAccountProvider>
      <Outlet />
    </ActiveAccountProvider>
  );
}

const router = createBrowserRouter([
  {
    id: "root",
    loader: () => ({ activeAccount: LOCAL_ACCOUNT }),
    Component: RootLayout,
    children: [
      { index: true, Component: () => <Navigate to="/chat" replace /> },
      { path: "chat", id: "chat-index", Component: ChatPage },
      { path: "chat/:deploymentId", id: "chat-deployment", Component: ChatPage },
      { path: "*", Component: () => <Navigate to="/chat" replace /> },
    ],
  },
]);

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("chat-embed: #root element not found");

createRoot(rootElement).render(
  <StrictMode>
    <ServerThemeContext.Provider value={DEFAULT_THEME}>
      <AuthContext.Provider value={localAuth}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
          <Toaster position="bottom-right" />
        </QueryClientProvider>
      </AuthContext.Provider>
    </ServerThemeContext.Provider>
  </StrictMode>,
);
