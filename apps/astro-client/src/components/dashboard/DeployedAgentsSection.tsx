import { useState } from "react";
import { DeploymentAgentCard } from "@/components/DeploymentAgentCard";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { ListPagination } from "@/components/ListPagination";
import { ListResultsTransition } from "@/components/ListResultsTransition";
import { ActionPanel } from "@/components/ui/status-panel";
import { DashboardAgentsEmptyState } from "./DashboardAgentsEmptyState";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useVisibleDeploymentSummaryMaps } from "./useDeploymentSummaryMaps";
import type { AgentDeploymentSummary } from "@/lib/api";

interface DeployedAgentsSectionProps {
  deployments: AgentDeploymentSummary[];
  account: string;
  isLoading: boolean;
  isError?: boolean;
  onRetry?: () => void;
  search?: string;
  onSearchChange?: (value: string) => void;
  currentPage?: number;
  totalPages?: number;
  isChangingPage?: boolean;
  onPageChange?: (page: number) => void;
  resultsTransitionKey?: string;
  accessProvisioningDeploymentId?: string | null;
  accessProvisioningDelayed?: boolean;
  accessProvisioningStalled?: boolean;
  onRetryAccessProvisioning?: () => void;
}

export function DeployedAgentsSection({
  deployments,
  account,
  isLoading,
  isError = false,
  onRetry = () => {},
  search = "",
  onSearchChange = () => {},
  currentPage = 1,
  totalPages = 0,
  isChangingPage = false,
  onPageChange = () => {},
  resultsTransitionKey = "agents",
  accessProvisioningDeploymentId,
  accessProvisioningDelayed = false,
  accessProvisioningStalled = false,
  onRetryAccessProvisioning,
}: DeployedAgentsSectionProps) {
  const { requestCounts, requestSeries, tokenSeries } = useVisibleDeploymentSummaryMaps(deployments);

  const { filtered, toolbarProps } = useAgentFilters(
    deployments,
    requestCounts,
    { filter: search, onFilterChange: onSearchChange },
  );
  const hasActiveFilters = toolbarProps.filter.trim().length > 0;
  const [filterResetKey, setFilterResetKey] = useState(0);
  const clearFilters = () => {
    toolbarProps.onFilterChange("");
    setFilterResetKey((key) => key + 1);
  };

  if (isError && deployments.length === 0) {
    return (
      <div role="alert">
        <ActionPanel tone="error" title="Couldn't load agents" primaryLabel="Retry" onPrimary={onRetry}>
          The agents list is temporarily unavailable.
        </ActionPanel>
      </div>
    );
  }

  const showToolbar = isLoading || deployments.length > 0 || hasActiveFilters;
  const showFilteredEmpty = !isLoading && filtered.length === 0 && hasActiveFilters;
  const showDashboardEmpty = !isLoading && deployments.length === 0 && !hasActiveFilters;

  if (showDashboardEmpty) {
    return <DashboardAgentsEmptyState account={account} />;
  }

  return (
    <>
      {showToolbar && (
        <div className="mb-4">
          <DashboardToolbar {...toolbarProps} filterResetKey={filterResetKey} />
        </div>
      )}

      <ListResultsTransition transitionKey={resultsTransitionKey}>
        {showFilteredEmpty && (
          <FilteredEmptyState message="No agents match your filters." onClear={clearFilters} />
        )}
        {filtered.length > 0 && (
          <div className="grid grid-cols-1 gap-3 @[440px]:grid-cols-2 @[680px]:grid-cols-3 @[920px]:grid-cols-4 @[920px]:gap-4 @[1180px]:grid-cols-5 @[1180px]:gap-5">
            {filtered.map((deployment) => {
              const ownerAccount = deployment.account_name || account;
              return (
                <DeploymentAgentCard
                  key={deployment.id}
                  deployment={deployment}
                  account={ownerAccount}
                  requestSeries={requestSeries.get(deployment.id)}
                  tokenSeries={tokenSeries.get(deployment.id)}
                  accessProvisioning={accessProvisioningDeploymentId === deployment.id}
                  accessProvisioningDelayed={accessProvisioningDelayed}
                  accessProvisioningStalled={
                    accessProvisioningDeploymentId === deployment.id && accessProvisioningStalled
                  }
                  onRetryAccess={onRetryAccessProvisioning}
                />
              );
            })}
          </div>
        )}
      </ListResultsTransition>
      <ListPagination
        currentPage={currentPage}
        totalPages={totalPages}
        onPageChange={onPageChange}
        disabled={isChangingPage}
        ariaLabel="Agent list pagination"
      />
    </>
  );
}
