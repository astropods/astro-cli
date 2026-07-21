import { useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useDeployments, useClusters, useRepairNormalizedSpec, useReapplyDeployment, useStopDeployment, useWakeUpDeployment } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import {
  formatDateTime,
  truncateUUID,
  formatClusterId,
  PRIMARY_CLUSTER_FILTER,
  deploymentMatchesClusterFilter,
} from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import { X, ChevronLeft, ChevronRight } from "lucide-react";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";
import type { AdminDeployment } from "@/types/admin";

type StatusFilter = "all" | "active" | "pending" | "provisioning" | "failed" | "undeploying" | "stopped";

const PAGE_SIZE = 25;

function filterDeployments(
  deployments: AdminDeployment[],
  search: string,
  status: StatusFilter,
  cluster: string,
  mismatchOnly: boolean,
): AdminDeployment[] {
  return deployments.filter((d) => {
    if (search) {
      const q = search.toLowerCase();
      if (
        !d.name.toLowerCase().includes(q) &&
        !d.namespace.toLowerCase().includes(q) &&
        !d.account_name.toLowerCase().includes(q) &&
        !d.deployment_id.toLowerCase().includes(q) &&
        !(d.owner_email ?? "").toLowerCase().includes(q) &&
        !formatClusterId(d.cluster_id).toLowerCase().includes(q) &&
        !formatClusterId(d.account_cluster_id).toLowerCase().includes(q)
      ) return false;
    }
    if (status !== "all" && d.status !== status) return false;
    if (!deploymentMatchesClusterFilter(d.cluster_id, cluster)) return false;
    if (mismatchOnly && !d.placement_mismatch) return false;
    return true;
  });
}

export function DeploymentsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { data, isLoading, error } = useDeployments();
  const { data: clustersData } = useClusters(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [lastClicked, setLastClicked] = useState<number | null>(null);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const cluster = searchParams.get("cluster") ?? "all";
  const mismatchOnly = searchParams.get("mismatch") === "1";
  const [page, setPage] = useState(0);

  const setClusterFilter = (value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value === "all") next.delete("cluster");
    else next.set("cluster", value);
    setSearchParams(next, { replace: true });
    setPage(0);
  };

  const setMismatchFilter = (enabled: boolean) => {
    const next = new URLSearchParams(searchParams);
    if (enabled) next.set("mismatch", "1");
    else next.delete("mismatch");
    setSearchParams(next, { replace: true });
    setPage(0);
  };

  const deployments = data?.deployments ?? [];
  const additionalClusters = (clustersData?.clusters ?? []).filter((c) => !c.is_primary);

  const filtered = useMemo(
    () => filterDeployments(deployments, search, status, cluster, mismatchOnly),
    [deployments, search, status, cluster, mismatchOnly],
  );

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages - 1);
  const pageDeployments = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

  const updateFilter = <T,>(setter: React.Dispatch<React.SetStateAction<T>>) => (v: T) => {
    setter(v);
    setPage(0);
  };

  const handleRowClick = (index: number, e: { shiftKey: boolean; metaKey: boolean; ctrlKey: boolean }) => {
    const d = pageDeployments[index];
    const id = d.deployment_id;
    setSelected((prev) => {
      const next = new Set(prev);
      if (e.shiftKey && lastClicked !== null) {
        const start = Math.min(lastClicked, index);
        const end = Math.max(lastClicked, index);
        for (let i = start; i <= end; i++) next.add(pageDeployments[i].deployment_id);
      } else if (e.metaKey || e.ctrlKey) {
        if (next.has(id)) next.delete(id); else next.add(id);
      } else {
        if (next.has(id)) next.delete(id); else next.add(id);
      }
      return next;
    });
    setLastClicked(index);
  };

  const toggleAll = () => {
    setSelected((s) => s.size === filtered.length ? new Set() : new Set(filtered.map((d) => d.deployment_id)));
  };

  const selectedDeployments = deployments.filter((d) => selected.has(d.deployment_id));

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Deployments</h2>

      {/* Filters */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          placeholder="Search name, namespace, account, cluster, owner..."
          value={search}
          onChange={(e) => updateFilter(setSearch)(e.target.value)}
          className="h-7 w-64 text-xs"
        />
        <Select value={status} onValueChange={updateFilter(setStatus)}>
          <SelectTrigger className="h-7 w-32 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="pending">Pending</SelectItem>
            <SelectItem value="provisioning">Provisioning</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="undeploying">Undeploying</SelectItem>
            <SelectItem value="stopped">Stopped</SelectItem>
          </SelectContent>
        </Select>
        <Select value={cluster} onValueChange={setClusterFilter}>
          <SelectTrigger className="h-7 w-36 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All clusters</SelectItem>
            <SelectItem value={PRIMARY_CLUSTER_FILTER}>Primary</SelectItem>
            {additionalClusters.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            className="size-3 accent-amber"
            checked={mismatchOnly}
            onChange={(e) => setMismatchFilter(e.target.checked)}
          />
          Placement mismatch only
        </label>
        <span className="ml-auto text-xs text-muted-foreground">
          {filtered.length} deployment{filtered.length !== 1 ? "s" : ""}
          {deployments.length > 0 && filtered.length !== deployments.length && ` of ${deployments.length}`}
        </span>
      </div>

      <BulkActions
        deployments={selectedDeployments}
        onDone={() => setSelected(new Set())}
        disabled={selected.size === 0}
      />

      {isLoading && <TableSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <>
          <div className="overflow-x-auto rounded-lg glass mt-3">
            <table className="w-full text-[11px] whitespace-nowrap">
              <thead>
                <tr className="border-b border-glass-border-honey glass-subtle">
                  <th className="px-2 py-0.5 w-6">
                    <input
                      type="checkbox"
                      checked={filtered.length > 0 && selected.size === filtered.length}
                      onChange={toggleAll}
                      className="size-3 accent-amber"
                    />
                  </th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Namespace</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Status</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Rev</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Account</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Cluster</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Owner</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Build</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Error</th>
                </tr>
              </thead>
              <tbody>
                {pageDeployments.map((d, i) => (
                  <tr
                    key={d.deployment_id}
                    className={`border-b border-comb-light select-none cursor-pointer transition-colors ${
                      selected.has(d.deployment_id)
                        ? "bg-pollen/10"
                        : "hover:bg-glass-light"
                    }`}
                    onClick={(e) => {
                      if ((e.target as HTMLElement).closest("a, button")) return;
                      if (e.shiftKey || e.metaKey || e.ctrlKey) {
                        handleRowClick(i, e);
                      } else if (selected.size > 0) {
                        handleRowClick(i, e);
                      } else {
                        navigate(`/admin/deployments/${d.deployment_id}`);
                      }
                    }}
                  >
                    <td className="px-2 py-0.5">
                      <input
                        type="checkbox"
                        checked={selected.has(d.deployment_id)}
                        readOnly
                        className="size-3 accent-amber cursor-pointer"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRowClick(i, e);
                        }}
                      />
                    </td>
                    <td className="px-2 py-0.5">
                      <span className="text-amber">{d.name}</span>
                    </td>
                    <td className="px-2 py-0.5 text-muted-foreground">{d.namespace}</td>
                    <td className="px-2 py-0.5">
                      <StatusBadge status={d.status} />
                      {d.status_changed_at && (
                        <span className="ml-1 text-[10px] text-muted-foreground" title={d.status_changed_at}>
                          {formatDistanceToNow(new Date(d.status_changed_at), { addSuffix: true })}
                        </span>
                      )}
                    </td>
                    <td className="px-2 py-0.5 text-muted-foreground font-mono">
                      {d.current_revision != null ? `rev ${d.current_revision}` : "-"}
                    </td>
                    <td className="px-2 py-0.5 text-muted-foreground">{d.account_name}</td>
                    <td className="px-2 py-0.5">
                      <span className="font-mono text-muted-foreground">{formatClusterId(d.cluster_id)}</span>
                      <PlacementMismatchBadge deployment={d} />
                    </td>
                    <td className="px-2 py-0.5 text-muted-foreground">{d.owner_email || "-"}</td>
                    <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{d.build_id ? truncateUUID(d.build_id) : "-"}</td>
                    <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(d.created_at)}</td>
                    <td className="max-w-[200px] truncate px-2 py-0.5 text-muted-foreground" title={d.error_message}>
                      {d.error_message || ""}
                    </td>
                  </tr>
                ))}
                {pageDeployments.length === 0 && (
                  <tr>
                    <td colSpan={11} className="px-2 py-4 text-center text-muted-foreground">
                      No deployments match the current filters.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
              <span>
                Page {safePage + 1} of {totalPages}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  disabled={safePage === 0}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  disabled={safePage >= totalPages - 1}
                  onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                >
                  <ChevronRight className="size-3.5" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function BulkActions({ deployments, onDone, disabled }: { deployments: AdminDeployment[]; onDone: () => void; disabled: boolean }) {
  const repairMut = useRepairNormalizedSpec();
  const reapplyMut = useReapplyDeployment();
  const stopMut = useStopDeployment();
  const wakeUpMut = useWakeUpDeployment();

  const [action, setAction] = useState<string>("");
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(0);

  const run = async () => {
    setRunning(true);
    setProgress(0);

    for (let i = 0; i < deployments.length; i++) {
      const d = deployments[i];
      try {
        if (action === "repair") {
          await repairMut.mutateAsync(d.deployment_id);
        } else if (action === "reapply") {
          await reapplyMut.mutateAsync(d.deployment_id);
        } else if (action === "stop") {
          await stopMut.mutateAsync(d.deployment_id);
        } else if (action === "wakeup") {
          await wakeUpMut.mutateAsync(d.deployment_id);
        }
      } catch {
        // continue on error
      }
      setProgress(i + 1);
    }

    setRunning(false);
    setAction("");
    onDone();
  };

  return (
    <div className={`rounded-lg glass px-3 py-2 flex items-center gap-3 ${disabled ? "opacity-50" : ""}`}>
      <span className="text-[11px] font-semibold shrink-0">
        {disabled ? "Select deployments to apply bulk actions" : `${deployments.length} selected`}
      </span>

      {!disabled && (
        <Button variant="ghost" size="icon-xs" onClick={onDone} title="Clear selection" className="shrink-0">
          <X className="size-3" />
        </Button>
      )}

      <Select value={action} onValueChange={(v) => setAction(v)} disabled={disabled}>
        <SelectTrigger className="w-48 h-7"><SelectValue placeholder="Choose action..." /></SelectTrigger>
        <SelectContent>
          <SelectItem value="repair">Repair Normalized Spec</SelectItem>
          <SelectItem value="reapply">Redeploy to K8s</SelectItem>
          <SelectItem value="stop">Pause (Stop)</SelectItem>
          <SelectItem value="wakeup">Wake Up</SelectItem>
        </SelectContent>
      </Select>

      {action && !disabled && (
        <Button
          size="xs"
          onClick={run}
          disabled={running}
        >
          {running ? `${progress}/${deployments.length}` : "Apply"}
        </Button>
      )}
    </div>
  );
}

function PlacementMismatchBadge({ deployment }: { deployment: AdminDeployment }) {
  if (!deployment.placement_mismatch) return null;
  const tooltip = `Account: ${formatClusterId(deployment.account_cluster_id)} · Deployment: ${formatClusterId(deployment.cluster_id)}. Redeploy does not change cluster.`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="ml-1 inline-block cursor-help rounded-full bg-amber-100/60 px-1.5 py-0.5 text-[10px] text-amber-800">
          mismatch
        </span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-[280px] text-xs">
        {tooltip}
      </TooltipContent>
    </Tooltip>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active: "bg-green-100/60 backdrop-blur-sm text-green-700",
    running: "bg-green-100/60 backdrop-blur-sm text-green-700",
    pending: "bg-yellow-100/60 backdrop-blur-sm text-yellow-700",
    provisioning: "bg-blue-100/60 backdrop-blur-sm text-blue-700",
    failed: "bg-red-100/60 backdrop-blur-sm text-red-700",
    undeploying: "bg-orange-100/60 backdrop-blur-sm text-orange-700",
    stopped: "bg-gray-100/60 backdrop-blur-sm text-gray-700",
  };
  return (
    <span className={`inline-block rounded-full px-2 py-0.5 text-xs ${colors[status?.toLowerCase()] ?? "rounded-full bg-pollen-light text-honey-dark"}`}>
      {status || "unknown"}
    </span>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
