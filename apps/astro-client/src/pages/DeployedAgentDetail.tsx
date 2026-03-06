import { useParams, Link } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { Badge } from "@/components/Badge";
import { AgentIdentity } from "@/components/AgentIdentity";

import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentName = params.agentName ?? "";

  const deploymentsData = await api.listDeployments(account).catch(() => ({ deployments: [], count: 0 }));
  const deployment = deploymentsData.deployments.find((d) => d.name === agentName) ?? null;

  return { deploymentsData, deployment, account, agentName };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const name = data?.deployment?.display_name || data?.agentName || "Agent";
  return [{ title: `${name} | Astro` }];
};

function DeployedAgentDetailSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      {/* Breadcrumb skeleton */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
      {/* Header skeleton */}
      <div className="flex items-center gap-4 px-6 py-6 border-b border-border">
        <Skeleton className="size-14 rounded-lg" />
        <div className="space-y-2">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
      </div>


    </div>
  );
}

const statusVariantMap = {
  active: "active",
  pending: "pending",
  error: "error",
  inactive: "inactive",
} as const;

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, agentName } = useParams<{ account: string; agentName: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated } = useAuth();

  const { data: deploymentsData } = useDeployments(account, isAuthenticated);

  const deployments = deploymentsData?.deployments ?? loaderData?.deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.name === agentName) ?? loaderData?.deployment ?? null;

  if (!deployment) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/agents">My Agents</Link>
        </Button>
      </div>
    );
  }

  const status = mapDeploymentStatus(deployment);
  const displayName = deployment.display_name || deployment.name;

  return (
    <div className="flex flex-1 flex-col">
      <PageBreadcrumb
        items={[
          { label: "My Agents", to: "/agents" },
          { label: displayName },
        ]}
      />

      {/* Header */}
      <div className="mx-auto w-full max-w-3xl">
        <div className="flex items-center gap-4 px-6 py-6">
          <AgentIdentity account={account} name={deployment.name} size={56} className="size-14 rounded-lg overflow-hidden" />
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-semibold truncate">{displayName}</h1>
              <Badge variant={statusVariantMap[status]} showDot>
                {status}
              </Badge>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              Deployed {formatDate(deployment.created_at)}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  if (!loaderData) {
    return <DeployedAgentDetailSkeleton />;
  }

  return (
    <ProtectedRoute>
      <DeployedAgentDetailContent loaderData={loaderData} />
    </ProtectedRoute>
  );
}
