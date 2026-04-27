import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { DeployedAgentCard } from "@/components/DeployedAgentCard";
import { DashboardAgentsEmptyState } from "./DashboardAgentsEmptyState";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useObservabilitySummaries, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { deploymentPath } from "@/lib/routes";
import { mapDeploymentStatus, formatRelativeTime } from "@/lib/deployment-utils";
import { blueprintKeys } from "@/api/queries/keys";
import { api, type BlueprintsListResponse } from "@/lib/api";
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
  requests: requestsProp,
}: {
  deployment: AgentDeployment;
  account: string;
  hasNewBuildAvailable: boolean;
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

  return (
    <DeployedAgentCard
      name={deployment.name}
      displayName={deployment.display_name}
      deploymentId={deployment.id}
      account={account}
      href={`${deploymentPath(account, deployment.id)}?tab=${status === "active" ? "monitor" : "deployments"}`}
      status={status}
      requests={requests}
      lastActive={lastActive}
      installedAt={deployment.created_at}
      updatedAt={deployment.updated_at || deployment.created_at}
      hasNewBuildAvailable={hasNewBuildAvailable}
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

  // Cross-account deploys (deployment.source_account != viewer account)
  // need their upgrade signal computed against the source account's
  // blueprint, not the viewer's. We fan out blueprint queries for every
  // unique source account in the visible deployments, usually 1 (the
  // viewer's), occasionally 2-3 when an org's blueprint was deployed
  // into the personal account. useQueries keeps each query independently
  // cached and shared with detail-view fetches.
  const sourceAccounts = useMemo(() => {
    const seen = new Set<string>();
    for (const d of deployments) seen.add(d.source_account || account);
    return Array.from(seen);
  }, [deployments, account]);

  const blueprintQueries = useQueries({
    queries: sourceAccounts.map((acct) => ({
      queryKey: blueprintKeys.byAccount(acct),
      queryFn: () => api.listAccountBlueprints(acct),
      enabled: !!acct,
    })),
  });

  const blueprintsByAccount = useMemo(() => {
    const byAccount = new Map<string, Map<string, BlueprintsListResponse["agents"][number]>>();
    blueprintQueries.forEach((result, i) => {
      const acct = sourceAccounts[i];
      if (!acct || !result.data?.agents) return;
      const byName = new Map<string, BlueprintsListResponse["agents"][number]>();
      for (const agent of result.data.agents) byName.set(agent.name, agent);
      byAccount.set(acct, byName);
    });
    return byAccount;
  }, [blueprintQueries, sourceAccounts]);

  const deploymentsWithNewBuild = useMemo(() => {
    return new Set(
      deployments.flatMap((deployment) => {
        const lineageAccount = deployment.source_account || account;
        const agent = blueprintsByAccount.get(lineageAccount)?.get(deployment.name);
        if (!agent?.versions?.length) return [];
        if (lineageAccount !== account && agent.visibility === "private") return [];
        const latest = agent.versions.reduce((a, b) =>
          new Date(b.published_at) > new Date(a.published_at) ? b : a,
        );
        return latest.build_id && deployment.build_id !== latest.build_id ? [deployment.id] : [];
      }),
    );
  }, [blueprintsByAccount, deployments, account]);

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
                requests={requestCounts.get(deployment.id)}
              />
            );
          })}
        </div>
      )}
    </>
  );
}
