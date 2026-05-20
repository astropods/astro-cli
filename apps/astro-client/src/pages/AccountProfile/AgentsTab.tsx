import type { AgentDeployment } from "@/lib/api";
import { DeployedAgentCard } from "@/components/DeployedAgentCard";
import { mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";
import { deploymentPath } from "@/lib/routes";
import { TabSearchInput, TabFilterDropdown } from "./TabToolbar";
import { EyeOff } from "lucide-react";

export type AgentSort = "modified" | "name";

const SORT_OPTIONS: { value: AgentSort; label: string }[] = [
  { value: "modified", label: "Last modified" },
  { value: "name", label: "Name A–Z" },
];

interface AgentsTabProps {
  deployments: AgentDeployment[];
  accountName: string;
  search: string;
  onSearchChange: (v: string) => void;
  sort: AgentSort;
  onSortChange: (v: AgentSort) => void;
  isLoading?: boolean;
}

export function AgentsTab({ deployments, accountName, search, onSearchChange, sort, onSortChange, isLoading }: AgentsTabProps) {
  const hasFilters = search.trim() !== "" || sort !== "modified";
  const sortLabel = SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Last modified";

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-3">
        <TabSearchInput value={search} onChange={onSearchChange} placeholder="Search agents…" />

        <TabFilterDropdown
          value={sort}
          onChange={onSortChange}
          options={SORT_OPTIONS}
          triggerLabel={sortLabel}
        />

        <span className="ml-auto flex items-center gap-1.5 text-body-sm text-faint-foreground shrink-0">
          <EyeOff className="size-3.5" />
          Only visible to you
        </span>
      </div>

      {isLoading && deployments.length === 0 && !hasFilters ? null : deployments.length === 0 ? (
        <p className="text-body text-muted-foreground">
          {hasFilters ? "No agents match your search." : "No agents deployed yet."}
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {deployments.map((d) => (
            <DeployedAgentCard
              key={d.id}
              name={d.name}
              displayName={d.display_name}
              deploymentId={d.id}
              account={accountName}
              href={deploymentPath(accountName, d.id)}
              status={mapDeploymentStatus(d)}
              requests={0}
              lastActive="—"
              installedAt={formatDate(d.created_at)}
              updatedAt={formatDate(d.updated_at || d.created_at)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
