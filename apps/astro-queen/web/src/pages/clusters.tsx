import { useState } from "react";
import {
  useClusters,
  useDeployments,
  useDeregisterCluster,
  useClusterBlockers,
  useCheckClusterHealth,
} from "@/api/admin";
import type { RegisteredCluster, GetClusterBlockersResponse, UrlReachability } from "@/types/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import { CircleCheck, CircleX, Copy, RefreshCw } from "lucide-react";
import { cn, formatDateTime, countDeploymentsByRoutedCluster, mutationErrorMessage } from "@/lib/utils";

// ClusterDeployFields is the per-cluster deploy config astro-server tracks
// (ingress, Langfuse collector URL, netpol inputs). Synced from cluster-config
// at boot; astro-queen only displays it.
type ClusterDeployFields = {
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
  /** Optional — only needed for IPv6 clusters (e.g. the pm-eu pilot). Empty otherwise. */
  pod_subnet_ipv6_cidrs: string;
  /** Optional — only needed when this cluster ships to its own local Loki/
   * Prometheus instead of the shared one. Empty otherwise. */
  loki_url: string;
  prometheus_url: string;
  /** Optional — only needed once this cluster has its own PrivateLink path to
   * the tenant-router Envoy. Empty means the in-app chat proxy falls back to
   * the K8s apiserver's services/proxy subresource. */
  tenant_router_internal_url: string;
};

/** Fields may be empty until boot sync completes; coalesce before use. */
function clusterDeployFromCluster(cluster: Pick<RegisteredCluster, keyof ClusterDeployFields>): ClusterDeployFields {
  return {
    agent_ingress_domain: cluster.agent_ingress_domain ?? "",
    ingestion_ingress_domain: cluster.ingestion_ingress_domain ?? "",
    langfuse_base_url_ext: cluster.langfuse_base_url_ext ?? "",
    langfuse_vpce_ips: cluster.langfuse_vpce_ips ?? "",
    pod_subnet_cidrs: cluster.pod_subnet_cidrs ?? "",
    pod_subnet_ipv6_cidrs: cluster.pod_subnet_ipv6_cidrs ?? "",
    loki_url: cluster.loki_url ?? "",
    prometheus_url: cluster.prometheus_url ?? "",
    tenant_router_internal_url: cluster.tenant_router_internal_url ?? "",
  };
}

// pod_subnet_ipv6_cidrs, loki_url, prometheus_url, and
// tenant_router_internal_url are intentionally excluded — they're optional
// server-side, unlike every other deploy field, so they don't gate completeness.
function clusterDeployComplete(f: ClusterDeployFields): boolean {
  return (
    f.agent_ingress_domain.trim() !== "" &&
    f.ingestion_ingress_domain.trim() !== "" &&
    f.langfuse_base_url_ext.trim() !== "" &&
    f.langfuse_vpce_ips.trim() !== "" &&
    f.pod_subnet_cidrs.trim() !== ""
  );
}

export function ClustersPage() {
  const { data, isLoading, error } = useClusters();
  const { data: deploymentsData } = useDeployments();

  const [selectedId, setSelectedId] = useState<string | null>(null);

  const clusters = data?.clusters ?? [];
  const deploymentCounts = countDeploymentsByRoutedCluster(deploymentsData?.deployments ?? []);
  const deployCountFor = (cluster: RegisteredCluster) =>
    cluster.is_primary ? (deploymentCounts.get("primary") ?? 0) : (deploymentCounts.get(cluster.id) ?? 0);
  const selectedCluster = clusters.find((c) => c.id === selectedId) ?? clusters[0] ?? null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">Clusters</h2>
          <p className="text-[10px] text-muted-foreground">
            Clusters are registered via deploy-time config (astro-infra) and synced at boot.
          </p>
        </div>
      </div>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      {!isLoading && !error && (
        <div className="flex gap-4">
          <div className="w-56 shrink-0 space-y-1">
            {clusters.length === 0 ? (
              <p className="rounded-lg glass px-3 py-4 text-center text-xs text-muted-foreground">
                No clusters found.
              </p>
            ) : (
              clusters.map((cluster) => (
                <ClusterListItem
                  key={cluster.id}
                  cluster={cluster}
                  deploymentCount={deployCountFor(cluster)}
                  selected={selectedCluster?.id === cluster.id}
                  onSelect={() => setSelectedId(cluster.id)}
                />
              ))
            )}
          </div>
          <div className="min-w-0 flex-1">
            {selectedCluster ? (
              <ClusterDetail cluster={selectedCluster} deploymentCount={deployCountFor(selectedCluster)} />
            ) : (
              <div className="flex h-40 items-center justify-center rounded-lg glass text-xs text-muted-foreground">
                Select a cluster to view details.
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function ClusterListItem({
  cluster,
  deploymentCount,
  selected,
  onSelect,
}: {
  cluster: RegisteredCluster;
  deploymentCount: number;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "w-full rounded-lg border px-3 py-2 text-left transition-colors",
        selected ? "glass-subtle border-glass-border-honey" : "border-transparent hover:glass-subtle",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-xs font-medium">{cluster.id}</span>
        {cluster.healthy ? (
          <CircleCheck className="size-3 shrink-0 text-green-600" />
        ) : (
          <CircleX className="size-3 shrink-0 text-red-500" />
        )}
      </div>
      <div className="mt-0.5 flex flex-wrap items-center gap-1 text-[10px] text-muted-foreground">
        <span>{cluster.region || "—"}</span>
        <span>·</span>
        <span>
          {deploymentCount} deploy{deploymentCount !== 1 ? "s" : ""}
        </span>
        {cluster.is_primary && (
          <span className="rounded bg-blue-500/10 px-1 text-blue-600">Primary</span>
        )}
      </div>
    </button>
  );
}

function DetailField({
  label,
  value,
  mono,
  small,
  reachability,
}: {
  label: string;
  value: string;
  mono?: boolean;
  small?: boolean;
  reachability?: UrlReachability;
}) {
  return (
    <div className="space-y-0.5">
      <div className="flex items-center gap-1 text-[10px] text-muted-foreground">
        {label}
        {reachability && <ReachabilityBadge result={reachability} />}
      </div>
      <div className={cn("break-all", mono && "font-mono", small ? "text-[10px]" : "text-xs")}>{value}</div>
    </div>
  );
}

function ReachabilityBadge({ result }: { result: UrlReachability }) {
  return (
    <span
      className={cn(
        "rounded px-1 py-0 text-[9px]",
        result.reachable ? "bg-green-500/10 text-green-600" : "bg-red-500/10 text-red-600",
      )}
      title={result.reachable ? "TCP dial succeeded" : result.error}
    >
      {result.reachable ? "reachable" : "unreachable"}
    </span>
  );
}

function ClusterBlockersPanel({ blockers }: { blockers: GetClusterBlockersResponse }) {
  const accounts = blockers.accounts ?? [];
  const deployments = blockers.deployments ?? [];
  const accountCount = blockers.account_count ?? 0;
  const deploymentCount = blockers.deployment_count ?? 0;
  const nothingListed = accounts.length === 0 && deployments.length === 0;
  return (
    <div className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-xs font-medium text-destructive">
        Blocked by {accountCount} account{accountCount !== 1 ? "s" : ""} and{" "}
        {deploymentCount} deployment{deploymentCount !== 1 ? "s" : ""} still
        referencing this cluster
      </p>
      {accounts.length > 0 && (
        <div>
          <div className="text-[10px] text-muted-foreground">Accounts pinned to this cluster</div>
          <ul className="text-xs">
            {accounts.map((a) => (
              <li key={a.id} className="font-mono">{a.name || a.id}</li>
            ))}
          </ul>
          {accountCount > accounts.length && (
            <p className="text-[10px] text-muted-foreground">
              +{accountCount - accounts.length} more
            </p>
          )}
        </div>
      )}
      {deployments.length > 0 && (
        <div>
          <div className="text-[10px] text-muted-foreground">Deployments routed here</div>
          <ul className="text-xs">
            {deployments.map((d) => (
              <li key={d.id} className="font-mono">
                {d.name || d.id} <span className="text-muted-foreground">({d.status})</span>
              </li>
            ))}
          </ul>
          {deploymentCount > deployments.length && (
            <p className="text-[10px] text-muted-foreground">
              +{deploymentCount - deployments.length} more
            </p>
          )}
        </div>
      )}
      {nothingListed && (
        <p className="text-[10px] text-muted-foreground">
          Nothing found — the block may have cleared. Try deregistering again.
        </p>
      )}
    </div>
  );
}

function ClusterDetail({
  cluster,
  deploymentCount,
}: {
  cluster: RegisteredCluster;
  deploymentCount: number;
}) {
  const deregisterMut = useDeregisterCluster();
  const healthMut = useCheckClusterHealth();

  const busy = deregisterMut.isPending || healthMut.isPending;
  const actionError = deregisterMut.error ?? healthMut.error;

  const blockersQuery = useClusterBlockers(cluster.id, deregisterMut.isError);

  const urlChecksByLabel = new Map<string, UrlReachability>(
    (healthMut.data?.url_checks ?? []).map((r) => [r.label, r]),
  );

  const runDeregister = () => deregisterMut.mutate(cluster.id);
  const runHealthCheck = () => healthMut.mutate(cluster.id);

  const deployFields = clusterDeployFromCluster(cluster);
  const ingressOk = cluster.is_primary || clusterDeployComplete(deployFields);

  const [copied, setCopied] = useState(false);
  const handleCopyError = async () => {
    if (!cluster.health_error) return;
    try {
      await navigator.clipboard.writeText(cluster.health_error);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable; text remains selectable in the pre block.
    }
  };

  return (
    <div className="space-y-4 rounded-lg glass p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="font-mono text-base font-semibold">{cluster.id}</h3>
            {cluster.is_primary && (
              <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-[10px] text-blue-600">Primary</span>
            )}
          </div>
          <p className="mt-0.5 text-[10px] text-muted-foreground">
            {deploymentCount} deployment{deploymentCount !== 1 ? "s" : ""} routed here
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Button size="xs" variant="outline" disabled={busy} onClick={runHealthCheck} title="Retry health check">
            <RefreshCw className={cn("size-3", healthMut.isPending && "animate-spin")} />
            Check health
          </Button>
          {cluster.is_primary ? (
            <span className="text-[10px] text-muted-foreground">Env kubeconfig</span>
          ) : (
            <DeregisterButton clusterId={cluster.id} disabled={busy} onConfirm={runDeregister} />
          )}
        </div>
      </div>

      {actionError && (
        <p className="text-[10px] text-destructive">{mutationErrorMessage(actionError)}</p>
      )}
      {deregisterMut.isError && blockersQuery.data && (
        <ClusterBlockersPanel blockers={blockersQuery.data} />
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <DetailField label="Region" value={cluster.region || "—"} />
        <DetailField label="EKS cluster name" value={cluster.eks_cluster_name || "—"} />
        <DetailField label="EKS endpoint" value={cluster.eks_cluster_endpoint || "—"} mono />
        <DetailField label="Created" value={cluster.created_at ? formatDateTime(cluster.created_at) : "—"} />
        <DetailField label="Updated" value={cluster.updated_at ? formatDateTime(cluster.updated_at) : "—"} />
      </div>

      <div className="space-y-1.5 rounded-md border border-glass-border-honey/60 p-3">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Ingress</span>
          <span className={cn("text-[10px]", ingressOk ? "text-green-600" : "text-amber-700")}>
            {ingressOk ? "OK" : "Incomplete — check cluster-config"}
          </span>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <DetailField label="Agent domain" value={deployFields.agent_ingress_domain || "—"} mono small />
          <DetailField label="Ingestion domain" value={deployFields.ingestion_ingress_domain || "—"} mono small />
        </div>
      </div>

      <div className="space-y-1.5 rounded-md border border-glass-border-honey/60 p-3">
        <span className="text-xs font-medium text-muted-foreground">Langfuse / netpol</span>
        <div className="grid gap-2 sm:grid-cols-2">
          <DetailField
            label="Langfuse base URL"
            value={deployFields.langfuse_base_url_ext || "—"}
            mono
            small
            reachability={urlChecksByLabel.get("langfuse_base_url_ext")}
          />
          <DetailField label="Langfuse VPCE IPs" value={deployFields.langfuse_vpce_ips || "—"} mono small />
          <DetailField label="Pod subnet CIDRs" value={deployFields.pod_subnet_cidrs || "—"} mono small />
          <DetailField label="Pod subnet IPv6 CIDRs" value={deployFields.pod_subnet_ipv6_cidrs || "—"} mono small />
          <DetailField
            label="Loki URL"
            value={deployFields.loki_url || "—"}
            mono
            small
            reachability={urlChecksByLabel.get("loki_url")}
          />
          <DetailField
            label="Prometheus URL"
            value={deployFields.prometheus_url || "—"}
            mono
            small
            reachability={urlChecksByLabel.get("prometheus_url")}
          />
          <DetailField
            label="Tenant router internal URL"
            value={deployFields.tenant_router_internal_url || "—"}
            mono
            small
            reachability={urlChecksByLabel.get("tenant_router_internal_url")}
          />
        </div>
      </div>

      <div className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">Health</span>
        <div className="flex items-center gap-1.5 text-xs">
          {cluster.healthy ? (
            <>
              <CircleCheck className="size-3.5 text-green-600" />
              <span className="text-green-600">Healthy</span>
            </>
          ) : (
            <>
              <CircleX className="size-3.5 text-red-500" />
              <span className="text-red-500">Unhealthy</span>
            </>
          )}
        </div>
        {!cluster.healthy && cluster.health_error && (
          <div className="flex items-start gap-1.5">
            <pre className="min-w-0 flex-1 select-all whitespace-pre-wrap break-all rounded border border-red-500/20 bg-red-500/5 px-2 py-1.5 font-mono text-[10px] leading-relaxed text-red-600">
              {cluster.health_error}
            </pre>
            <Button size="xs" variant="outline" className="shrink-0" onClick={handleCopyError}>
              <Copy className="size-3" />
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

function DeregisterButton({
  clusterId,
  disabled,
  onConfirm,
}: {
  clusterId: string;
  disabled: boolean;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="xs" variant="destructive" disabled={disabled}>
          Deregister
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Deregister cluster {clusterId}?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the cluster from the registry. Deployments still referencing this cluster
            must be moved or deleted first.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Deregister
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
