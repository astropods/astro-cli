import React, { useMemo, useState } from "react";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { createServerApi } from "@/lib/api.server";
import {
  BookOpenIcon,
  UsersIcon,
} from "@heroicons/react/24/outline";
import { Bot } from "lucide-react";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { DashboardStats } from "@/components/dashboard/DashboardStats";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { OrgSwitcher } from "@/components/OrgSwitcher";
import { useDeployments } from "@/api/queries/deployments";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useAccountMembers } from "@/api/queries/accounts";
import { useAuth } from "@/lib/auth";
import { blueprintsPaths, deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";
import type { AgentDeployment } from "@/lib/api";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  try {
    const auth = await api.getCurrentUser();
    const url = new URL(request.url);
    const accountParam = url.searchParams.get("account");
    const account = auth.accounts?.find((a) => a.name === accountParam)
      ?? auth.accounts?.find((a) => a.type === "personal");
    if (!account) return { count: 0 };
    const { count } = await api.countDeployments(account.name);
    return { count };
  } catch {
    return { count: 0 };
  }
}

function DashboardLabel({ icon: Icon, to, children }: { icon: React.ElementType; to?: string; children: React.ReactNode }) {
  const className = "inline-flex items-center gap-1.5 font-mono text-mono-sm" + (to ? " hover:text-teal-700 transition-colors" : "");
  const content = <><Icon className="size-3.5" strokeWidth={1.5} />{children}</>;
  return to ? <Link to={to} className={className}>{content}</Link> : <span className={className}>{content}</span>;
}

function AgentDashboardContent({ skeletonCount }: { skeletonCount: number }) {
  const greeting = useMemo(() => {
    const hour = new Date().getHours();
    if (hour < 12) return "Good morning";
    if (hour < 18) return "Good afternoon";
    return "Good evening";
  }, []);

  const { personalAccount, accounts, isAuthenticated } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();

  const { defaultAccount, validStoredDefault, handleSetDefault } = useDefaultAccount();
  const userAccount = searchParams.get("account") || validStoredDefault || personalAccount?.name || "";

  const revealState = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarUrl?: string } | null;
  const revealDeploymentId = revealState?.revealDeploymentId ?? null;
  const revealAgentName = revealState?.revealAgentName ?? null;
  const revealDisplayName = revealState?.revealDisplayName ?? null;
  const revealAvatarUrl = revealState?.revealAvatarUrl ?? null;
  const [showReveal, setShowReveal] = useState(!!revealDeploymentId);

  const setActiveAccount = (account: string) => {
    setSearchParams({ account });
  };
  const displayName =
    personalAccount?.display_name || personalAccount?.name || "";

  const { data, isLoading } = useDeployments(userAccount, isAuthenticated);
  const { data: accountBlueprints } = useAccountBlueprints(userAccount, {
    enabled: isAuthenticated,
  });
  const { data: membersData } = useAccountMembers(userAccount);

  const blueprintCount = accountBlueprints?.agents.length ?? 0;
  const deployments = data?.deployments ?? [];
  const blueprintAgents = accountBlueprints?.agents ?? [];
  const memberCount = membersData?.members.length ?? 0;
  const activeAccountType = accounts.find((a) => a.name === userAccount)?.type;

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

  return (
    <div className="flex-1 bg-muted">
      <div className="px-6 py-6">
        <div className="mb-6 flex flex-col gap-3">
          <div className="flex flex-col-reverse gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <h1 className="min-w-0 text-heading-1 text-foreground">
              {greeting}
              {displayName ? `, ${displayName}` : ""}
            </h1>
            <OrgSwitcher
              activeAccount={userAccount}
              defaultAccount={defaultAccount}
              onChange={setActiveAccount}
              onSetDefault={handleSetDefault}
            />
          </div>
          <div className="flex flex-wrap items-center gap-4 text-body-sm text-muted-foreground">
            {activeAccountType === "organization" && (
              <DashboardLabel icon={UsersIcon}>
                {memberCount} member{memberCount !== 1 ? "s" : ""}
              </DashboardLabel>
            )}
            <DashboardLabel icon={Bot}>
              {deployments.length} agent{deployments.length !== 1 ? "s" : ""}
            </DashboardLabel>
            <DashboardLabel icon={BookOpenIcon} to={activeAccountType === "organization" ? blueprintsPaths.account(userAccount) : blueprintsPaths.personal}>
              {blueprintCount} blueprint{blueprintCount !== 1 ? "s" : ""}
            </DashboardLabel>
          </div>
        </div>

        <DashboardStats
          account={userAccount}
          isLoading={isLoading}
        />
        <div>
          <h2 className="text-heading-2 text-foreground mb-4">
            Deployed agents
          </h2>
          <DeployedAgentsSection
            deployments={deployments}
            account={userAccount}
            isLoading={isLoading}
            blueprintAgents={blueprintAgents}
            skeletonDeploymentId={showReveal ? revealDeploymentId : null}
            skeletonCount={skeletonCount}
          />
        </div>
      </div>
      {showReveal && revealDeployment && (
        <LiveRevealOverlay
          deployment={revealDeployment}
          account={userAccount}
          fallbackAvatarUrl={revealAvatarUrl ?? undefined}
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
    </div>
  );
}

export default function AgentDashboard({ loaderData }: Route.ComponentProps) {
  return (
    <ProtectedRoute>
      <AgentDashboardContent skeletonCount={loaderData?.count ?? 0} />
    </ProtectedRoute>
  );
}
