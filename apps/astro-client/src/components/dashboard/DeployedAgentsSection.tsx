import { DeploymentAgentCard } from "@/components/DeploymentAgentCard";
import { AccountLoadWarning } from "@/components/AccountLoadWarning";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { ActionPanel } from "@/components/ui/status-panel";
import { DashboardAgentsEmptyState } from "./DashboardAgentsEmptyState";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useMultiAccountDeploymentSummaryMaps } from "./useDeploymentSummaryMaps";
import type { DeploymentWithAccount } from "@/api/queries/all-accounts";

// Kept for the LiveReveal flow only: when a newly-deployed agent is revealing
// into its slot, we show this placeholder card in its spot until the real
// deployment row arrives. NOT used for generic loading — page data comes from
// the loader's cache-primed deployments, so there's nothing to skeleton.
export function AgentCardSkeleton() {
  return (
    <div className="flex flex-col items-center gap-4 rounded-md border border-border bg-card p-4 pt-8 animate-pulse">
      <div className="h-16 w-16 rounded-md bg-muted" />
      <div className="flex flex-col items-center gap-1.5">
        <div className="h-5 w-32 rounded bg-muted" />
        <div className="h-3 w-24 rounded bg-muted" />
      </div>
      <div className="h-12 w-full" />
      <div className="h-10 w-full rounded bg-muted" />
    </div>
  );
}

interface DeployedAgentsSectionProps {
  deployments: DeploymentWithAccount[];
  /** Fallback account used for the empty-state "create in…" CTA. */
  account: string;
  isLoading: boolean;
  isError: boolean;
  failedAccounts: string[];
  onRetry: () => void;
  skeletonDeploymentId?: string | null;
  /** Selected account names; empty means all accounts. */
  accountFilters: string[];
  onAccountFiltersChange: (accounts: string[]) => void;
}

export function DeployedAgentsSection({
  deployments,
  account,
  isLoading,
  isError,
  failedAccounts,
  onRetry,
  skeletonDeploymentId,
  accountFilters,
  onAccountFiltersChange,
}: DeployedAgentsSectionProps) {
  const { requestCounts, requestSeries, tokenSeries } =
    useMultiAccountDeploymentSummaryMaps(deployments);

  const { filtered, toolbarProps } = useAgentFilters(deployments, requestCounts);
  const hasTextFilter = toolbarProps.filter.trim().length > 0;
  const hasActiveFilters = hasTextFilter || accountFilters.length > 0;

  const clearFilters = () => {
    toolbarProps.onFilterChange("");
    onAccountFiltersChange([]);
  };

  const emptyState = hasActiveFilters ? (
    <FilteredEmptyState message="No agents match your filters." onClear={clearFilters} />
  ) : (
    <DashboardAgentsEmptyState account={account} />
  );
  const showEmptyState =
    !isLoading && filtered.length === 0 && (!isError || deployments.length > 0);

  if (isError && failedAccounts.length === 0 && deployments.length === 0) {
    return (
      <div role="alert">
        <ActionPanel
          tone="error"
          title="Couldn't load agents"
          primaryLabel="Retry"
          onPrimary={onRetry}
        >
          The agents list is temporarily unavailable.
        </ActionPanel>
      </div>
    );
  }

  if (showEmptyState && deployments.length === 0) {
    return emptyState;
  }

  return (
    <>
      {isError && (
        <div className="mb-4">
          <AccountLoadWarning failedAccounts={failedAccounts} onRetry={onRetry} />
        </div>
      )}

      {deployments.length > 0 && (
        <div className="mb-4">
          <DashboardToolbar
            {...toolbarProps}
            accountFilters={accountFilters}
            onAccountFiltersChange={onAccountFiltersChange}
          />
        </div>
      )}

      {showEmptyState && emptyState}
      {filtered.length > 0 && (
        <div className="grid grid-cols-1 gap-3 @[440px]:grid-cols-2 @[680px]:grid-cols-3 @[920px]:grid-cols-4 @[920px]:gap-4 @[1180px]:grid-cols-5 @[1180px]:gap-5">
          {filtered.map((deployment) => {
            if (skeletonDeploymentId && deployment.id === skeletonDeploymentId) {
              return <AgentCardSkeleton key={deployment.id} />;
            }
            return (
              <DeploymentAgentCard
                key={deployment.id}
                deployment={deployment}
                account={deployment.account}
                requestSeries={requestSeries.get(deployment.id)}
                tokenSeries={tokenSeries.get(deployment.id)}
              />
            );
          })}
        </div>
      )}
    </>
  );
}
