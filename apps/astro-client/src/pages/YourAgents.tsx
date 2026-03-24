import { useState, useMemo } from "react";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { EmptyState } from "../components/EmptyState";
import {
  MyAgentsHeader,
  type ViewMode,
} from "../components/MyAgentsHeader";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { useDeployments } from "../api/queries/deployments";
import { useAccountAgents } from "../api/queries/agents";
import { useObservabilitySummary, useObservabilityTraces } from "../api/queries/observability";
import { useAuth } from "../lib/auth";
import { mapDeploymentStatus, formatDate } from "../lib/deployment-utils";
import { deploymentPath } from "../lib/routes";
import type { AgentDeployment } from "../lib/api";

function formatRelativeTime(isoString: string): string {
  const diffMs = new Date(isoString).getTime() - Date.now();
  const diffSecs = Math.round(diffMs / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return rtf.format(diffSecs, "second");
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

  return (
    <DeployedAgentCard
      name={deployment.name}
      displayName={deployment.display_name}
      deploymentId={deployment.id}
      account={userAccount}
      href={clickable ? deploymentPath(userAccount, deployment.id) : undefined}
      status={status}
      requests={requests}
      lastActive={lastActive}
      installedAt={formatDate(deployment.created_at)}
      updatedAt={formatDate(deployment.created_at)}
      hasNewBuildAvailable={hasNewBuildAvailable}
    />
  );
}

function YourAgentsContent() {
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  const { personalAccount, isAuthenticated } = useAuth();
  const userAccount = personalAccount?.name ?? "";
  const { data } = useDeployments(userAccount, isAuthenticated);
  const { data: accountAgents } = useAccountAgents(userAccount, { enabled: isAuthenticated });

  const latestBuildByName = useMemo(() => {
    const result = new Map<string, string>();
    const agents = accountAgents?.agents ?? [];
    for (const agent of agents) {
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
  }, [accountAgents?.agents]);

  const filtered = useMemo(() => {
    const list = data?.deployments ?? [];
    if (!filter) return list;
    const lower = filter.toLowerCase();
    return list.filter((d) =>
      d.name.toLowerCase().includes(lower) ||
      d.display_name?.toLowerCase().includes(lower),
    );
  }, [data?.deployments, filter]);

  return (
    <div className="flex flex-1 flex-col p-6 md:p-8">
      <MyAgentsHeader
        filter={filter}
        onFilterChange={setFilter}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />

      {filtered.length === 0 ? (
        <EmptyState
          title="No agents yet"
          description="Browse available agent blueprints and deploy one to get started."
          actionLabel="Browse blueprints"
          actionTo="/blueprints"
        />
      ) : (
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {filtered.map((deployment) => {
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
    </div>
  );
}

export default function YourAgents() {
  return (
    <ProtectedRoute>
      <YourAgentsContent />
    </ProtectedRoute>
  );
}
