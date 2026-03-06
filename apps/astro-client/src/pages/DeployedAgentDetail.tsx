import { useState, useRef, useMemo } from "react";
import { useParams, Link, useSearchParams } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { Badge } from "@/components/Badge";
import { AgentIdentity } from "@/components/AgentIdentity";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployments, useDeploymentLogs } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";
import { RefreshCw, Loader2, ArrowDown } from "lucide-react";
import type { PodDetail, ApiError } from "@/lib/api";
import { useWindowVirtualizer } from "@tanstack/react-virtual";

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
      <div className="flex items-center justify-between px-6 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
      <div className="mx-auto w-full max-w-3xl">
        <div className="flex items-center gap-4 px-6 py-6">
          <Skeleton className="size-14 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
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

function phaseColor(phase: string): string {
  switch (phase) {
    case "Running": return "text-green-600";
    case "Pending": return "text-yellow-600";
    case "Failed": return "text-red-600";
    case "Succeeded": return "text-blue-600";
    default: return "text-muted-foreground";
  }
}

// ---------------------------------------------------------------------------
// Pod log viewer (inline, not a modal)
// ---------------------------------------------------------------------------

function PodLogViewer({ account, namespace, pod }: { account: string; namespace: string; pod: PodDetail }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const containerParam = searchParams.get("container");
  const selectedContainer = containerParam && pod.containers.some((c) => c.name === containerParam)
    ? containerParam
    : pod.containers[0]?.name ?? "";
  const [tailLines, setTailLines] = useState(200);

  const { data: logs, isLoading, error: logsError, refetch } = useDeploymentLogs(
    account, namespace, pod.name, selectedContainer, tailLines,
  );

  const lines = useMemo(() => (logs ?? "").split("\n"), [logs]);
  const listRef = useRef<HTMLDivElement>(null);

  const virtualizer = useWindowVirtualizer({
    count: lines.length,
    estimateSize: () => 20,
    overscan: 60,
    scrollMargin: listRef.current?.offsetTop ?? 0,
  });

  const lineNumberWidth = String(lines.length).length;

  const scrollToBottom = () => {
    if (lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { align: "end" });
    }
  };

  const error = logsError
    ? (logsError as unknown as ApiError & { details?: string }).details
      ?? (logsError as unknown as ApiError).error_description
      ?? logsError.message
      ?? "Failed to fetch logs"
    : null;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        {pod.containers.length > 1 && (
          <div className="flex items-center gap-1.5 text-sm">
            <span className="text-muted-foreground">Container:</span>
            <Select
              value={selectedContainer}
              onValueChange={(value) => setSearchParams((prev) => {
                const next = new URLSearchParams(prev);
                next.set("container", value);
                return next;
              })}
            >
              <SelectTrigger className="h-7 w-auto min-w-[120px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pod.containers.map((c) => (
                  <SelectItem key={c.name} value={c.name}>{c.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <Select
          value={String(tailLines)}
          onValueChange={(value) => setTailLines(Number(value))}
        >
          <SelectTrigger className="h-7 w-auto min-w-[60px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[50, 100, 200, 500].map((n) => (
              <SelectItem key={n} value={String(n)}>{n}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isLoading}>
            <RefreshCw size={14} className={isLoading ? "animate-spin" : ""} />
            Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={scrollToBottom}>
            <ArrowDown size={14} />
            Bottom
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 size={24} className="animate-spin text-muted-foreground" />
          <span className="ml-2 text-muted-foreground">Loading logs...</span>
        </div>
      ) : error ? (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded">
          {error}
        </div>
      ) : (
        <div ref={listRef} className="bg-stone-900 rounded">
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => (
              <div
                key={virtualRow.index}
                data-index={virtualRow.index}
                ref={virtualizer.measureElement}
                className="absolute left-0 w-full flex text-xs font-mono leading-5 hover:bg-stone-800/60"
                style={{
                  top: virtualRow.start - (virtualizer.options.scrollMargin ?? 0),
                }}
              >
                <span
                  className="shrink-0 select-none text-right text-stone-500 px-3 border-r border-stone-700 py-px"
                  style={{ width: `${lineNumberWidth + 3}ch` }}
                >
                  {virtualRow.index + 1}
                </span>
                <span className="text-stone-100 px-3 whitespace-pre-wrap break-all min-w-0 py-px">
                  {lines[virtualRow.index]}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pod grid
// ---------------------------------------------------------------------------

function PodGrid({ pods, basePath }: { pods: PodDetail[]; basePath: string }) {
  if (pods.length === 0) {
    return <p className="text-sm text-muted-foreground">No pods</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <h2 className="text-base font-semibold text-foreground">Containers</h2>
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      {pods.map((pod) => {
        const readyCount = pod.containers.filter((c) => c.ready).length;
        return (
          <Link
            key={pod.name}
            to={`${basePath}?pod=${encodeURIComponent(pod.name)}`}
            className="border border-border rounded-sm p-3 bg-card hover:bg-card-hover transition-colors"
          >
            <p className="font-mono text-sm truncate" title={pod.name}>{pod.name}</p>
            <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
              <span className={`font-medium ${phaseColor(pod.phase)}`}>{pod.phase}</span>
              <span>{readyCount}/{pod.containers.length} ready</span>
              <span>{pod.age}</span>
            </div>
          </Link>
        );
      })}
    </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main content
// ---------------------------------------------------------------------------

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, agentName } = useParams<{ account: string; agentName: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated, personalAccount } = useAuth();
  const [searchParams] = useSearchParams();
  const podName = searchParams.get("pod");

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
  const pods = deployment.pods ?? [];
  const selectedPod = podName ? pods.find((p) => p.name === podName) ?? null : null;
  const basePath = `/${account}/agents/${deployment.name}`;

  const isPersonal = personalAccount?.name === account;
  const breadcrumbItems = [
    isPersonal
      ? { label: "My Agents", to: "/agents" }
      : { label: account, to: `/${account}` },
    ...(selectedPod
      ? [
          { label: displayName, to: basePath },
          { label: selectedPod.name },
        ]
      : [{ label: displayName }]),
  ];

  return (
    <div className="flex flex-1 flex-col">
      <PageBreadcrumb items={breadcrumbItems} />

      <div className={`mx-auto w-full ${selectedPod ? "max-w-6xl" : "max-w-3xl"}`}>
        {/* Header */}
        <div className="flex items-center gap-4 px-6 py-6">
          <AgentIdentity account={account} name={deployment.name} size={56} className="size-14 rounded-sm overflow-hidden" />
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

        <div className="mx-6 border-t border-border" />

        {/* Body */}
        <div className="px-6 py-6">
          {selectedPod ? (
            <PodLogViewer account={account} namespace={deployment.namespace} pod={selectedPod} />
          ) : (
            <PodGrid pods={pods} basePath={basePath} />
          )}
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
