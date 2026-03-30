import { useState, useMemo } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/YourAgents";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { EmptyState } from "../components/EmptyState";
import {
  MyAgentsHeader,
  type ViewMode,
} from "../components/MyAgentsHeader";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { useDeployments } from "../api/queries/deployments";
import { useAccountBlueprints } from "../api/queries/blueprints";
import { useObservabilitySummary, useObservabilityTraces } from "../api/queries/observability";
import { useAuth } from "../lib/auth";
import { mapDeploymentStatus } from "../lib/deployment-utils";
import { deploymentPath } from "../lib/routes";
import type { AgentDeployment } from "../lib/api";
import { createServerApi } from "../lib/api.server";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";

function formatRelativeTime(isoString: string): string {
  const diffMs = new Date(isoString).getTime() - Date.now();
  const diffSecs = Math.round(diffMs / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return "less than a minute ago";
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, "minute");
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");
  return rtf.format(diffDays, "day");
}

function AgentCardWithStats({
  deployment,
  userAccount,
  hasNewBuildAvailable,
}: {
  deployment: AgentDeployment;
  userAccount: string;
  hasNewBuildAvailable: boolean;
}) {
  const { data: summaryData } = useObservabilitySummary(deployment.id);
  const { data: tracesData } = useObservabilityTraces(deployment.id, { limit: "1" });

  const requests = summaryData?.total_traces ?? 0;
  const latestTrace = tracesData?.traces[0];
  const lastActive = latestTrace ? formatRelativeTime(latestTrace.timestamp) : "—";

  const status = mapDeploymentStatus(deployment);
  const clickable = status === "active" || status === "error" || status === "pending" || status === "undeploying";
  const href = (() => {
    if (!clickable) return undefined;
    const base = deploymentPath(userAccount, deployment.id);
    if (status === "active") return `${base}?tab=monitor`;
    return `${base}?tab=deployments`;
  })();

  return (
    <DeployedAgentCard
      name={deployment.name}
      displayName={deployment.display_name}
      deploymentId={deployment.id}
      account={userAccount}
      href={href}
      linkState={{ fromAgents: true }}
      status={status}
      requests={requests}
      lastActive={lastActive}
      installedAt={deployment.created_at}
      updatedAt={deployment.updated_at || deployment.created_at}
      avatarUrl={deployment.avatar_url}
      hasNewBuildAvailable={hasNewBuildAvailable}
    />
  );
}

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  try {
    const auth = await api.getCurrentUser();
    const personalAccount = auth.accounts?.find((a) => a.type === "personal");
    if (!personalAccount) return { count: 0 };
    const { count } = await api.countDeployments(personalAccount.name);
    return { count };
  } catch {
    return { count: 0 };
  }
}

function DeployedAgentCardSkeleton() {
  return (
    <div className="flex flex-col gap-3 rounded-md border border-stone-300 bg-background px-4 pb-[22px] pt-3 dark:border-border animate-pulse">
      <div className="flex items-center gap-3">
        <div className="h-9 w-9 shrink-0 rounded-sm bg-muted" />
        <div className="flex-1 space-y-2">
          <div className="h-4 w-28 rounded bg-muted" />
          <div className="h-3 w-16 rounded bg-muted" />
        </div>
      </div>
      <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-3">
        <div className="space-y-1">
          <div className="h-3 w-14 rounded bg-muted" />
          <div className="h-3 w-10 rounded bg-muted" />
        </div>
        <div className="space-y-1">
          <div className="h-3 w-14 rounded bg-muted" />
          <div className="h-3 w-10 rounded bg-muted" />
        </div>
        <div className="space-y-1">
          <div className="h-3 w-14 rounded bg-muted" />
          <div className="h-3 w-10 rounded bg-muted" />
        </div>
        <div className="space-y-1">
          <div className="h-3 w-14 rounded bg-muted" />
          <div className="h-3 w-10 rounded bg-muted" />
        </div>
      </div>
    </div>
  );
}

function YourAgentsContent({ skeletonCount }: { skeletonCount: number }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const { personalAccount, isAuthenticated } = useAuth();
  const userAccount = personalAccount?.name ?? "";
  const { data, isLoading } = useDeployments(userAccount, isAuthenticated);
  const { data: accountBlueprints } = useAccountBlueprints(userAccount, { enabled: isAuthenticated });
  const revealState = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealAvatarUrl?: string } | null;
  const revealDeploymentId = revealState?.revealDeploymentId ?? null;
  const revealAgentName = revealState?.revealAgentName ?? null;
  const revealAvatarUrl = revealState?.revealAvatarUrl ?? null;
  const [showReveal, setShowReveal] = useState(!!revealDeploymentId);

  const latestBuildByName = useMemo(() => {
    const result = new Map<string, string>();
    const blueprints = accountBlueprints?.agents ?? [];
    for (const agent of blueprints) {
      if (!agent.versions?.length) continue;
      const latestVersion = agent.versions.reduce((latest, current) =>
        new Date(current.published_at).getTime() > new Date(latest.published_at).getTime()
          ? current
          : latest,
      );
      if (latestVersion?.build_id) {
        result.set(agent.name, latestVersion.build_id);
      }
    }
    return result;
  }, [accountBlueprints?.agents]);

  const filtered = useMemo(() => {
    const list = data?.deployments ?? [];
    if (!filter) return list;
    const lower = filter.toLowerCase();
    return list.filter((d) =>
      d.name.toLowerCase().includes(lower) ||
      d.display_name?.toLowerCase().includes(lower),
    );
  }, [data?.deployments, filter]);

  const revealDeployment = useMemo(() => {
    if (!revealDeploymentId) return null;
    const existing = (data?.deployments ?? []).find((d) => d.id === revealDeploymentId);
    if (existing) return existing;
    if (!revealAgentName) return null;

    // Optimistic fallback so reveal can render immediately after redirect
    // before deployments polling returns the new deployment row.
    return {
      id: revealDeploymentId,
      name: revealAgentName,
      display_name: revealAgentName,
      build_id: "",
      namespace: "",
      status: "pending",
      replicas: 1,
      ready: 0,
      created_at: new Date().toISOString(),
      components: [],
    } satisfies AgentDeployment;
  }, [data?.deployments, revealAgentName, revealDeploymentId]);

  const clearRevealState = () => {
    navigate(location.pathname + location.search, { replace: true, state: {} });
  };

  return (
    <div className="flex flex-1 flex-col p-6 md:p-8">
      <MyAgentsHeader
        filter={filter}
        onFilterChange={setFilter}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />

      {isLoading ? (
        skeletonCount > 0 ? (
          <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
            {Array.from({ length: skeletonCount }, (_, i) => (
              <DeployedAgentCardSkeleton key={i} />
            ))}
          </div>
        ) : null
      ) : filtered.length === 0 ? (
        <EmptyState
          title="No agents yet"
          description="Browse available agent blueprints and deploy one to get started."
          actionLabel="Browse blueprints"
          actionTo="/blueprints"
        />
      ) : (
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {filtered.map((deployment) => {
            if (showReveal && (deployment.id === revealDeploymentId || deployment.name === revealAgentName)) {
              return <DeployedAgentCardSkeleton key={deployment.id} />;
            }
            const latestBuildId = latestBuildByName.get(deployment.name);
            const hasNewBuildAvailable = !!latestBuildId && latestBuildId !== deployment.build_id;
            return (
              <AgentCardWithStats
                key={deployment.id}
                deployment={deployment}
                userAccount={userAccount}
                hasNewBuildAvailable={hasNewBuildAvailable}
              />
            );
          })}
        </div>
      )}
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

export default function YourAgents({ loaderData }: Route.ComponentProps) {
  return (
    <ProtectedRoute>
      <YourAgentsContent skeletonCount={loaderData?.count ?? 0} />
    </ProtectedRoute>
  );
}
