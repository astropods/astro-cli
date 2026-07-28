import { useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useAllAccountsDeployments } from "@/api/queries/all-accounts";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAccountFilterParam } from "@/hooks/use-account-filter-param";
import { deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/ui/LiveRevealOverlay";
import type { AgentDeploymentSummary, AvatarColors } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Agents | Astro" }];

export default function AgentDashboard() {
  const { isAuthenticated } = useAuth();
  const { activeAccount: userAccount } = useActiveAccount();
  const location = useLocation();
  const navigate = useNavigate();

  const [revealDeployment] = useState<AgentDeploymentSummary | null>(() => {
    const rs = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarColors?: AvatarColors } | null;
    if (!rs?.revealDeploymentId || !rs.revealAgentName) return null;
    return {
      id: rs.revealDeploymentId,
      name: rs.revealAgentName,
      display_name: rs.revealDisplayName ?? rs.revealAgentName,
      avatar_colors: rs.revealAvatarColors,
      build_id: "",
      created_at: new Date().toISOString(),
    } satisfies AgentDeploymentSummary;
  });
  const [showReveal, setShowReveal] = useState(!!revealDeployment);
  const [accountFilters, setAccountFilters] = useAccountFilterParam("agents");

  const {
    deployments: allDeployments,
    isLoading,
    isError,
    failedAccounts,
    retryFailed,
  } = useAllAccountsDeployments(isAuthenticated, accountFilters);

  const clearRevealState = () => {
    navigate(location.pathname + location.search, { replace: true, state: {} });
  };

  return (
    <>
      <PageContainer
        outerClassName="bg-background"
      >
        <PageHeader
          title="Agents"
          description="Deployed agents running across your accounts."
        />

        <DeployedAgentsSection
          deployments={allDeployments}
          account={userAccount}
          isLoading={isLoading}
          isError={isError}
          failedAccounts={failedAccounts}
          onRetry={retryFailed}
          skeletonDeploymentId={showReveal ? revealDeployment?.id ?? null : null}
          accountFilters={accountFilters}
          onAccountFiltersChange={setAccountFilters}
        />
      </PageContainer>

      {showReveal && revealDeployment && (
        <LiveRevealOverlay
          deployment={revealDeployment}
          account={userAccount}
          onDismiss={() => {
            setShowReveal(false);
            clearRevealState();
          }}
          onViewDeployment={() => {
            const targetPath = deploymentPath(userAccount, revealDeployment.id);
            const backPath = location.pathname + location.search;
            window.history.replaceState({}, "", location.pathname + location.search);
            navigate(targetPath, { state: { fromAgents: true, backPath } });
          }}
        />
      )}
    </>
  );
}
