import { useState, useMemo } from "react";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { EmptyState } from "../components/EmptyState";
import {
  MyAgentsHeader,
  type ViewMode,
} from "../components/MyAgentsHeader";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { useDeployments } from "../api/queries/deployments";
import { useAuth } from "../lib/auth";
import { mapDeploymentStatus, formatDate } from "../lib/deployment-utils";

function YourAgentsContent() {
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  const { personalAccount, isAuthenticated } = useAuth();
  const userAccount = personalAccount?.name ?? "";
  const { data } = useDeployments(userAccount, isAuthenticated);

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
          {filtered.map((deployment) => (
            <DeployedAgentCard
              key={deployment.name}
              name={deployment.name}
              displayName={deployment.display_name}
              account={userAccount}
              href={`/${userAccount}/agents/${deployment.name}`}
              status={mapDeploymentStatus(deployment)}
              requests={0}
              lastActive="—"
              installedAt={formatDate(deployment.created_at)}
              updatedAt={formatDate(deployment.created_at)}
            />
          ))}
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
