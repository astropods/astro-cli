import { useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { createServerApi } from "@/lib/api.server";
import { DashboardStats } from "@/components/dashboard/DashboardStats";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useDeployments } from "@/api/queries/deployments";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";
import type { AgentDeployment } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Agents | Astro" }];

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  try {
    const auth = await api.getCurrentUser();
    const account = auth.accounts?.find((a) => a.type === "personal");
    if (!account) return { count: 0 };
    const { count } = await api.countDeployments(account.name);
    return { count };
  } catch {
    return { count: 0 };
  }
}


function AgentDashboardInner({ skeletonCount }: { skeletonCount: number }) {
  const { isAuthenticated } = useAuth();
  const { activeAccount: userAccount } = useActiveAccount();
  const location = useLocation();
  const navigate = useNavigate();

  const revealState = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarUrl?: string } | null;
  const revealDeploymentId = revealState?.revealDeploymentId ?? null;
  const revealAgentName = revealState?.revealAgentName ?? null;
  const revealDisplayName = revealState?.revealDisplayName ?? null;
  const revealAvatarUrl = revealState?.revealAvatarUrl ?? null;
  const [showReveal, setShowReveal] = useState(!!revealDeploymentId);


  const { data, isLoading } = useDeployments(userAccount, isAuthenticated);
  const { data: accountBlueprints } = useAccountBlueprints(userAccount, {
    enabled: isAuthenticated,
  });

  const deployments = data?.deployments ?? [];
  const blueprintAgents = accountBlueprints?.agents ?? [];

  const revealDeployment = useMemo<AgentDeployment | null>(() => {
    if (!revealDeploymentId) return null;
    const existing = deployments.find((d) => d.id === revealDeploymentId);
    if (existing) {
      return {
        ...existing,
        ...(!existing.display_name && revealDisplayName ? { display_name: revealDisplayName } : {}),
        ...(!existing.avatar_url && revealAvatarUrl ? { avatar_url: revealAvatarUrl } : {}),
      };
    }
    if (!revealAgentName) return null;
    return {
      id: revealDeploymentId,
      name: revealAgentName,
      display_name: revealDisplayName ?? revealAgentName,
      build_id: "",
      namespace: "",
      status: "pending",
      replicas: 1,
      ready: 0,
      created_at: new Date().toISOString(),
      components: [],
    } satisfies AgentDeployment;
  }, [deployments, revealAgentName, revealAvatarUrl, revealDeploymentId, revealDisplayName]);

  const clearRevealState = () => {
    navigate(location.pathname + location.search, { replace: true, state: {} });
  };

  const isAgentsEmpty = !isLoading && deployments.length === 0;

  return (
    <>
      <PageContainer
        style={
          isAgentsEmpty
            ? {
                backgroundImage:
                  "radial-gradient(ellipse 100% 55% at 50% 0%, color-mix(in oklch, var(--muted) 65%, transparent) 0%, transparent 50%)",
              }
            : undefined
        }
      >
        <PageHeader
          title="Agents"
          description="Deployed agents running in your account."
          adornment={<PageScopeSwitcher />}
        />

        {!isAgentsEmpty && <DashboardStats account={userAccount} isLoading={isLoading} />}

        <DeployedAgentsSection
          deployments={deployments}
          account={userAccount}
          isLoading={isLoading}
          blueprintAgents={blueprintAgents}
          skeletonDeploymentId={showReveal ? revealDeploymentId : null}
          skeletonCount={skeletonCount}
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
  return <AgentDashboardInner skeletonCount={loaderData?.count ?? 0} />;
}
