import { useMemo } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { AgentAvatar } from "@/components/AgentAvatar";
import { DeployedAgentCard } from "@/components/DeployedAgentCard";
import { DashboardToolbar } from "./DashboardToolbar";
import { useAgentFilters } from "./useAgentFilters";
import { useObservabilitySummaries, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { blueprintsPaths, deploymentPath } from "@/lib/routes";
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
      href={deploymentPath(account, deployment.id)}
      status={status}
      requests={requests}
      lastActive={lastActive}
      installedAt={deployment.created_at}
      updatedAt={deployment.updated_at || deployment.created_at}
      avatarUrl={deployment.avatar_url}
      hasNewBuildAvailable={hasNewBuildAvailable}
    />
  );
}

interface DeployedAgentsSectionProps {
  deployments: AgentDeployment[];
  blueprintAgents: { name: string; versions?: { build_id: string; published_at: string }[] }[];
  account: string;
  isLoading: boolean;
}

export function DeployedAgentsSection({
  deployments,
  blueprintAgents,
  account,
  isLoading,
}: DeployedAgentsSectionProps) {
  const summaryResults = useObservabilitySummaries(deployments.map((d) => d.id));

  const requestCounts = useMemo(() => {
    const map = new Map<string, number>();
    deployments.forEach((d, i) => {
      const count = summaryResults[i]?.data?.total_traces;
      if (count !== undefined) map.set(d.id, count);
    });
    return map;
  }, [deployments, summaryResults]);

  const { filtered, toolbarProps } = useAgentFilters(deployments, requestCounts);
  const isEmpty = !isLoading && deployments.length === 0;

  const deploymentsWithNewBuild = useMemo(() => {
    const result = new Set<string>();
    for (const agent of blueprintAgents) {
      if (!agent.versions?.length) continue;
      const latest = agent.versions.reduce((a, b) =>
        new Date(b.published_at) > new Date(a.published_at) ? b : a,
      );
      if (!latest?.build_id) continue;
      const deployment = deployments.find((d) => d.name === agent.name);
      if (deployment && deployment.build_id !== latest.build_id) {
        result.add(deployment.id);
      }
    }
    return result;
  }, [blueprintAgents, deployments]);

  if (isLoading) {
    return (
      <>
        <div className="mb-4">
          <DashboardToolbar {...toolbarProps} disabled />
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <AgentCardSkeleton key={i} />
          ))}
        </div>
      </>
    );
  }

  if (isEmpty) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border py-14 px-12 gap-4">
        <div className="flex items-center">
          <AgentAvatar name="agent-avatar-1" size={52} className="z-10 relative" />
          <AgentAvatar name="agent-avatar-3" size={52} className="-ml-3 z-20 relative" />
          <AgentAvatar name="agent-avatar-2" size={52} className="-ml-3 z-10 relative" />
        </div>
        <div className="flex flex-col items-center gap-1.5">
          <p className="text-heading-4 font-semibold text-foreground">No agents deployed yet</p>
          <p className="text-body-sm text-muted-foreground text-center max-w-[300px]">
            Deploy an agent from a blueprint or create your own from the CLI.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="default" size="sm" asChild>
            <Link to={blueprintsPaths.discover}>Browse blueprints</Link>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href="https://docs.astropods.com/quickstart" target="_blank" rel="noopener noreferrer">
              Read docs
            </a>
          </Button>
        </div>
      </div>
    );
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
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
          {filtered.map((deployment) => (
            <AgentCard
              key={deployment.id}
              deployment={deployment}
              account={account}
              hasNewBuildAvailable={deploymentsWithNewBuild.has(deployment.id)}
              requests={requestCounts.get(deployment.id)}
            />
          ))}
        </div>
      )}
    </>
  );
}
