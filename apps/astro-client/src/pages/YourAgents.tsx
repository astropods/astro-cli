import { useState, useMemo } from "react";
import { ProtectedRoute } from "../components/ProtectedRoute";
import {
  MyAgentsHeader,
  type ViewMode,
} from "../components/MyAgentsHeader";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";
import { useDeployments } from "../api/queries/deployments";
import { useAuth } from "../lib/auth";
import type { AgentDeployment } from "../lib/api";

function mapDeploymentStatus(deployment: AgentDeployment): DeployedAgentStatus {
  if (deployment.status === "error" || (deployment.ready === 0 && deployment.replicas > 0)) {
    return "error";
  }
  if (deployment.status === "pending" || deployment.ready < deployment.replicas) {
    return "pending";
  }
  if (deployment.replicas === 0) {
    return "inactive";
  }
  return "active";
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function YourAgentsContent() {
  const [filter, setFilter] = useState("");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  const { accounts, isAuthenticated } = useAuth();
  const userAccount = accounts[0]?.name ?? "";
  const { data } = useDeployments(userAccount, isAuthenticated);
  const deployments = data?.deployments ?? [];

  const filtered = useMemo(() => {
    if (!filter) return deployments;
    const lower = filter.toLowerCase();
    return deployments.filter((d) => d.name.toLowerCase().includes(lower));
  }, [deployments, filter]);

  return (
    <div className="p-6 md:p-8">
      <MyAgentsHeader
        filter={filter}
        onFilterChange={setFilter}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />

      <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {filtered.map((deployment) => (
          <DeployedAgentCard
            key={deployment.name}
            name={deployment.name}
            account={userAccount}
            href={`/${userAccount}/${deployment.name}`}
            status={mapDeploymentStatus(deployment)}
            requests={0}
            lastActive="—"
            installedAt={formatDate(deployment.created_at)}
            updatedAt={formatDate(deployment.created_at)}
          />
        ))}
      </div>
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
