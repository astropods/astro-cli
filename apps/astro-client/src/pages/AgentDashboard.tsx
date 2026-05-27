import { useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { getActiveAccount } from "@/lib/api.server";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useDeployments } from "@/api/queries/deployments";
import { deploymentKeys } from "@/api/queries/keys";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/ui/LiveRevealOverlay";
import type { AgentDeployment, AvatarColors } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Agents | Astro" }];

// Inline (not loadAccountScoped) to prime the deployments cache before render.
export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getActiveAccount(request);
  if (!ctx) {
    return { account: null, deployments: null };
  }
  const deployments = await ctx.api.listDeployments(ctx.accountName).catch(() => null);
  return { account: ctx.accountName, deployments };
}


function AgentDashboardInner() {
  const { isAuthenticated } = useAuth();
  const { activeAccount: userAccount } = useActiveAccount();
  const location = useLocation();
  const navigate = useNavigate();

  const [revealDeployment] = useState<AgentDeployment | null>(() => {
    const rs = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarUrl?: string; revealAvatarColors?: AvatarColors } | null;
    if (!rs?.revealDeploymentId || !rs.revealAgentName) return null;
    return {
      id: rs.revealDeploymentId,
      name: rs.revealAgentName,
      display_name: rs.revealDisplayName ?? rs.revealAgentName,
      avatar_url: rs.revealAvatarUrl ?? undefined,
      avatar_colors: rs.revealAvatarColors,
      build_id: "",
      namespace: "",
      status: "pending",
      replicas: 1,
      ready: 0,
      created_at: new Date().toISOString(),
      components: [],
    } satisfies AgentDeployment;
  });
  const [showReveal, setShowReveal] = useState(!!revealDeployment);


  const { data, isLoading } = useDeployments(userAccount, isAuthenticated);

  const deployments = data?.deployments ?? [];


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
          description="Deployed agents running in your account."
          action={<PageScopeSwitcher />}
        />

        <DeployedAgentsSection
          deployments={deployments}
          account={userAccount}
          isLoading={isLoading}
          skeletonDeploymentId={showReveal ? revealDeployment?.id ?? null : null}
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

export default function AgentDashboard({ loaderData }: Route.ComponentProps) {
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account) return;
    if (ld.deployments) qc.setQueryData(deploymentKeys.all(ld.account), ld.deployments);
  });

  return <AgentDashboardInner />;
}
