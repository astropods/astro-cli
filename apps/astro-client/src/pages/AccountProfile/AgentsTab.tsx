import type { AgentDeploymentSummary } from "@/lib/api";
import { DeploymentAgentCard } from "@/components/DeploymentAgentCard";
import { useDeploymentSummaryMaps } from "@/components/dashboard/useDeploymentSummaryMaps";
import { TabSearchInput, TabFilterDropdown } from "./TabToolbar";

export type AgentSort = "modified" | "name";

const SORT_OPTIONS: { value: AgentSort; label: string }[] = [
  { value: "modified", label: "Last modified" },
  { value: "name", label: "Name A–Z" },
];

interface AgentsTabProps {
  deployments: AgentDeploymentSummary[];
  accountName: string;
  search: string;
  onSearchChange: (v: string) => void;
  sort: AgentSort;
  onSortChange: (v: AgentSort) => void;
  isLoading?: boolean;
}

export function AgentsTab({ deployments, accountName, search, onSearchChange, sort, onSortChange, isLoading }: AgentsTabProps) {
  const { requestSeries, tokenSeries } = useDeploymentSummaryMaps(accountName, deployments);
  const hasFilters = search.trim() !== "" || sort !== "modified";
  const sortLabel = SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Last modified";

  return (
    <div className="flex flex-col gap-5">
      {(deployments.length > 0 || hasFilters) && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-3">
          <TabSearchInput value={search} onChange={onSearchChange} placeholder="Search agents" />
          <TabFilterDropdown
            value={sort}
            onChange={onSortChange}
            options={SORT_OPTIONS}
            triggerLabel={sortLabel}
          />
        </div>
      )}

      {isLoading && deployments.length === 0 && !hasFilters ? null : deployments.length === 0 ? (
        <p className="text-body text-muted-foreground">
          {hasFilters ? "No agents match your search." : "No agents deployed yet."}
        </p>
      ) : (
        <div className="@container grid grid-cols-1 gap-3 @[440px]:grid-cols-2 @[680px]:grid-cols-3 @[920px]:grid-cols-4 @[920px]:gap-4 @[1180px]:grid-cols-5 @[1180px]:gap-5">
          {deployments.map((d) => (
            <DeploymentAgentCard
              key={d.id}
              deployment={d}
              account={accountName}
              requestSeries={requestSeries.get(d.id)}
              tokenSeries={tokenSeries.get(d.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
