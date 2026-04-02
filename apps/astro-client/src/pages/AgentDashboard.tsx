import React, { useMemo, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import {
  BookOpenIcon,
  UsersIcon,
  Cog6ToothIcon,
} from "@heroicons/react/24/outline";
import { Bot } from "lucide-react";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { Button } from "@/components/ui/button";
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

function DashboardLabel({ icon: Icon, to, children }: { icon: React.ElementType; to?: string; children: React.ReactNode }) {
  const className = "inline-flex items-center gap-1.5 font-mono text-mono-sm" + (to ? " hover:text-teal-700 transition-colors" : "");
  const content = <><Icon className="size-3.5" strokeWidth={1.5} />{children}</>;
  return to ? <Link to={to} className={className}>{content}</Link> : <span className={className}>{content}</span>;
}

function AgentDashboardContent() {
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
  const userAccount = searchParams.get("account") || personalAccount?.name || "";

  const revealState = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarUrl?: string } | null;
  const revealDeploymentId = revealState?.revealDeploymentId ?? null;
  const revealAgentName = revealState?.revealAgentName ?? null;
  const revealDisplayName = revealState?.revealDisplayName ?? null;
  const revealAvatarUrl = revealState?.revealAvatarUrl ?? null;
  const [showReveal, setShowReveal] = useState(!!revealDeploymentId);

  const setActiveAccount = (account: string) => {
    setSearchParams(account === personalAccount?.name ? {} : { account });
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
    <div className="min-h-full bg-muted">
      <PageBreadcrumb
        items={[
          { label: "Dashboard", to: "/dashboard" },
          {
            label: (
              <OrgSwitcher
                activeAccount={userAccount}
                onChange={setActiveAccount}
              />
            ),
          },
        ]}
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              asChild
            >
              <Link to={blueprintsPaths.discover}>Browse blueprints</Link>
            </Button>
            {activeAccountType !== "organization" && (
              <Button
                variant="default"
                size="sm"
                asChild
              >
                <Link to="/settings"><Cog6ToothIcon className="size-3.5 text-white" />Settings</Link>
              </Button>
            )}
          </div>
        }
        mobileActions={
          <div className="flex items-center gap-2">
            <Button variant="outline" size="icon" asChild>
              <Link to={blueprintsPaths.discover} aria-label="Browse blueprints">
                <BookOpenIcon className="size-3.5" />
              </Link>
            </Button>
            {activeAccountType !== "organization" && (
              <Button variant="outline" size="icon" asChild>
                <Link to="/settings" aria-label="Settings">
                  <Cog6ToothIcon className="size-3.5" />
                </Link>
              </Button>
            )}
          </div>
        }
      />

      <div className="px-6 py-6">
        <div className="mb-6">
          <h1 className="text-heading-1 text-foreground">
            {greeting}
            {displayName ? `, ${displayName}` : ""}
          </h1>
          <div className="mt-3 flex flex-wrap items-center gap-4 text-body-sm text-muted-foreground">
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
            window.history.replaceState({}, "", location.pathname + location.search);
            navigate(targetPath, { state: { fromAgents: true } });
          }}
        />
      )}
    </div>
  );
}

export default function AgentDashboard() {
  return (
    <ProtectedRoute>
      <AgentDashboardContent />
    </ProtectedRoute>
  );
}
