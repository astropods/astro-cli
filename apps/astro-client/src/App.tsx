import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, useAuth } from "./lib/auth";
import { queryClient } from "./lib/queryClient";
import { Layout } from "./components/Layout";
import { Home } from "./pages/Home";
import { Hire } from "./pages/Hire";
import { AgentDetail } from "./pages/AgentDetail";
import { RequestAgent } from "./pages/RequestAgent";
import { YourAgents } from "./pages/YourAgents";
import { OperatorOverview } from "./pages/OperatorOverview";
import { AgentPage } from "./pages/AgentPage";
import { DeployPage } from "./pages/DeployPage";
import { Docs } from "./pages/Docs";
import { Onboarding } from "./pages/Onboarding";
import { NotFound } from "./pages/NotFound";

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

function App() {
  return (
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <OnboardingGuard>
            <Routes>
              <Route path="/" element={<Layout />}>
                <Route index element={<Home />} />
                <Route path="hire" element={<Hire />} />
                <Route path="hire/:account/:agentSlug" element={<AgentDetail />} />
                {/* Keep old route for backwards compatibility during transition */}
                <Route path="hire/:agentSlug" element={<AgentDetail />} />
                <Route path="request-agent" element={<RequestAgent />} />
                <Route path="agents" element={<YourAgents />} />
                <Route path="operator" element={<OperatorOverview />} />
                <Route path="operator/deploy/:account/:name" element={<DeployPage />} />
                <Route path="u/:account/:agent" element={<AgentPage />} />
                <Route path="onboarding" element={<Onboarding />} />
                {/* /docs is public — getting started & CLI install */}
                <Route path="docs" element={<Docs />} />
                {/* /dev redirects to /docs */}
                <Route path="dev" element={<Navigate to="/docs" replace />} />
                <Route path="*" element={<NotFound />} />
              </Route>
            </Routes>
          </OnboardingGuard>
        </BrowserRouter>
      </QueryClientProvider>
    </AuthProvider>
  );
}

export default App;
