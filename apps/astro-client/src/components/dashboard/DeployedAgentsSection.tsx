import { DeploymentAgentCard } from "@/components/DeploymentAgentCard";
import { DashboardAgentsEmptyState } from "./DashboardAgentsEmptyState";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useDeploymentSummaryMaps } from "./useDeploymentSummaryMaps";
import type { AgentDeploymentSummary } from "@/lib/api";

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
  deployments: AgentDeploymentSummary[];
  account: string;
  isLoading: boolean;
  skeletonDeploymentId?: string | null;
}

export function DeployedAgentsSection({
  deployments,
  account,
  isLoading,
  skeletonDeploymentId,
}: DeployedAgentsSectionProps) {
  const { requestCounts, requestSeries, tokenSeries } = useDeploymentSummaryMaps(
    account,
    deployments,
  );

  const { filtered, toolbarProps } = useAgentFilters(deployments, requestCounts);
  const isEmpty = !isLoading && deployments.length === 0;

  if (isEmpty) {
    return <DashboardAgentsEmptyState />;
  }

  return (
    <>
      <div className="mb-4">
        <DashboardToolbar {...toolbarProps} />
      </div>

      {filtered.length === 0 && (
        <div className="flex items-center justify-center py-16">
          <p className="text-body-sm text-muted-foreground">
            No agents match your search.
          </p>
        </div>
      )}
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
                account={account}
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
