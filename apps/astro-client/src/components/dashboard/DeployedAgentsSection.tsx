import { useMemo } from "react";
import { DeployedAgentCard } from "@/components/DeployedAgentCard";
import { DashboardAgentsEmptyState } from "./DashboardAgentsEmptyState";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useObservabilitySummaries, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { deploymentPath } from "@/lib/routes";
import { mapDeploymentStatus, formatRelativeTime } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";

function AgentCardSkeleton() {
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
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="space-y-1">
            <div className="h-3 w-14 rounded bg-muted" />
            <div className="h-3 w-10 rounded bg-muted" />
          </div>
        ))}
      </div>
    </div>
  );
}

function AgentCard({
  deployment,
  account,
  hasNewBuildAvailable,
  latestBuildId,
  requests: requestsProp,
}: {
  deployment: AgentDeployment;
  account: string;
  hasNewBuildAvailable: boolean;
  latestBuildId?: string;
  requests?: number;
}) {
  const { data: summaryData } = useObservabilitySummary(deployment.id, undefined, {
    enabled: requestsProp === undefined,
  });
  const { data: tracesData } = useObservabilityTraces(deployment.id, { limit: "1" });
  const requests = requestsProp ?? summaryData?.total_traces ?? 0;
  const latestTrace = tracesData?.traces[0];
  const lastActive = latestTrace ? formatRelativeTime(latestTrace.timestamp) : "—";
  const status = mapDeploymentStatus(deployment);
  const detailHref = `${deploymentPath(account, deployment.id)}?tab=${status === "active" ? "monitor" : "deployments"}`;

  return (
    <DeployedAgentCard
      name={deployment.name}
      displayName={deployment.display_name}
      deploymentId={deployment.id}
      account={account}
      href={detailHref}
      deploymentDetailHref={detailHref}
      status={status}
      requests={requests}
      lastActive={lastActive}
      installedAt={deployment.created_at}
      updatedAt={deployment.updated_at || deployment.created_at}
      hasNewBuildAvailable={hasNewBuildAvailable}
      latestBuildId={latestBuildId}
      currentBuildId={deployment.build_id}
      errorMessage={deployment.error_message}
      avatarColors={deployment.avatar_colors}
    />
  );
}

interface DeployedAgentsSectionProps {
  deployments: AgentDeployment[];
  account: string;
  isLoading: boolean;
  skeletonDeploymentId?: string | null;
  skeletonCount?: number;
}

export function DeployedAgentsSection({
  deployments,
  account,
  isLoading,
  skeletonDeploymentId,
  skeletonCount = 4,
}: DeployedAgentsSectionProps) {
  const summaryResults = useObservabilitySummaries(deployments.map((d) => d.id));

  const requestCounts = useMemo(() =>
    new Map(
      deployments
        .map((d, i) => [d.id, summaryResults[i]?.data?.total_traces] as const)
        .filter((entry): entry is [string, number] => entry[1] !== undefined)
    ),
  [deployments, summaryResults]);

  const { filtered, toolbarProps } = useAgentFilters(deployments, requestCounts);
  const isEmpty = !isLoading && deployments.length === 0;

  // The list endpoint computes latest_build_id at request time via a single
  // batch query against agent_versions (see populateLatestBuildIDs in
  // apps/astro-server/handlers/deploy.go). The server is the single source of
  // truth for the upgrade signal — the dashboard never needs to fan out
  // per-account blueprint queries to derive it.
  const deploymentsWithNewBuild = useMemo(() => {
    return new Set(
      deployments.flatMap((d) =>
        d.latest_build_id && d.latest_build_id !== d.build_id ? [d.id] : [],
      ),
    );
  }, [deployments]);

  if (isLoading) {
    return (
      <>
        <div className="mb-4">
          <DashboardToolbar {...toolbarProps} disabled />
        </div>
        <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[800px]:grid-cols-3 @[1100px]:grid-cols-4">
          {Array.from({ length: skeletonCount }).map((_, i) => (
            <AgentCardSkeleton key={i} />
          ))}
        </div>
      </>
    );
  }

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
            No agents match your filters.
          </p>
        </div>
      )}
      {filtered.length > 0 && (
        <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[800px]:grid-cols-3 @[1100px]:grid-cols-4">
          {filtered.map((deployment) => {
            if (skeletonDeploymentId && deployment.id === skeletonDeploymentId) {
              return <AgentCardSkeleton key={deployment.id} />;
            }
            return (
              <AgentCard
                key={deployment.id}
                deployment={deployment}
                account={account}
                hasNewBuildAvailable={deploymentsWithNewBuild.has(deployment.id)}
                latestBuildId={deployment.latest_build_id}
                requests={requestCounts.get(deployment.id)}
              />
            );
          })}
        </div>
      )}
    </>
  );
}
