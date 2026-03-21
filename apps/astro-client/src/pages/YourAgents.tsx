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
import { useAuth } from "../lib/auth";
import { mapDeploymentStatus, formatDate } from "../lib/deployment-utils";
import { deploymentPath } from "../lib/routes";

function YourAgentsContent() {
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  const { personalAccount, isAuthenticated } = useAuth();
  const userAccount = personalAccount?.name ?? "";
  const { data } = useDeployments(userAccount, isAuthenticated);
  const { data: accountAgents } = useAccountAgents(userAccount, isAuthenticated);

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
          description="Browse available agents and add one to get started."
          actionLabel="Browse agents"
          actionTo="/browse"
        />
      ) : (
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {filtered.map((deployment) => {
            const latestBuildId = latestBuildByName.get(deployment.name);
            const hasNewBuildAvailable = !!latestBuildId && latestBuildId !== deployment.build_id;
            const status = mapDeploymentStatus(deployment);
            const clickable = status === "active" || status === "error" || status === "pending" || status === "undeploying";
            return (
            <DeployedAgentCard
              key={deployment.id}
              name={deployment.name}
              displayName={deployment.display_name}
              deploymentId={deployment.id}
              account={userAccount}
              href={clickable ? deploymentPath(userAccount, deployment.id) : undefined}
              status={status}
              requests={0}
              lastActive="—"
              installedAt={formatDate(deployment.created_at)}
              updatedAt={formatDate(deployment.created_at)}
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
