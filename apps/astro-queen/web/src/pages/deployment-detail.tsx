import { useState } from "react";
import { useParams, useNavigate, Link, useSearchParams } from "react-router";
import { useDeployment, useDeleteDeployment, useRestartPod, useWakeUpDeployment, useStopDeployment, useRollbackDeployment, useReapplyDeployment, useRepairNormalizedSpec, useDeploymentJobs, usePodLogs, usePodEnv, useSetAdapters } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Trash2, RotateCw, FileText, Settings, Sun, Undo2, AlertTriangle, Play, Pause, Wrench, Layers, CheckCircle2 } from "lucide-react";
import { deriveDisplayDeploymentStatus } from "@/lib/display-deployment-status";
import {
  cn,
  formatDateTime,
  truncateUUID,
  formatClusterId,
  ecrRegionFromImage,
  specTargetClusterId,
} from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import type { K8sPodInfo, DeploymentEvent, DeploymentRevision, DeploymentJob, AdminVariable } from "@/types/admin";

export function DeploymentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get("tab") ?? "events";
  const highlightJobParam = searchParams.get("job");
  const highlightJobId =
    highlightJobParam != null && highlightJobParam !== "" ? Number(highlightJobParam) : undefined;
  const { data, isLoading, error, refetch } = useDeployment(id ?? "");
  const deleteMut = useDeleteDeployment();
  const wakeUpMut = useWakeUpDeployment();
  const stopMut = useStopDeployment();
  const rollbackMut = useRollbackDeployment();
  const reapplyMut = useReapplyDeployment();
  const repairMut = useRepairNormalizedSpec();
  const restartPodMut = useRestartPod();
  const setAdaptersMut = useSetAdapters();
  const jobsQuery = useDeploymentJobs(id ?? "");
  const [selectedPod, setSelectedPod] = useState<{ deploymentId: string; name: string; container?: string; mode: "logs" | "env" } | null>(null);
  const [adaptersOpen, setAdaptersOpen] = useState(false);
  const [pendingAdapters, setPendingAdapters] = useState<string>("none");
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const showSuccess = (msg: string) => {
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(null), 4000);
  };

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-destructive">Error: {error.message}</p>;
  if (!data) return null;

  const { deployment: dep, cluster_status: cs, events, revisions } = data;
  const displayStatus = deriveDisplayDeploymentStatus(dep, cs);
  const isTransitional =
    ["pending", "provisioning", "undeploying"].includes(dep.status) ||
    displayStatus.value === "deploying" ||
    displayStatus.value === "undeploying";
  const isRuntimeDeploying = displayStatus.differsFromDB && displayStatus.value === "deploying";

  return (
    <div className="space-y-6">
      {successMsg && (
        <div className="flex items-center gap-2 rounded-lg bg-green-500/10 px-3 py-2 text-xs text-green-600">
          <CheckCircle2 className="size-3.5 shrink-0" />
          {successMsg}
        </div>
      )}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{dep.name}</h2>
          <p className="text-sm text-muted-foreground">{dep.namespace}</p>
          {cs && (
            <p className="text-xs text-muted-foreground">
              Live K8s data from{" "}
              <span className="font-mono">{formatClusterId(cs.resolved_cluster_id)}</span>
            </p>
          )}
        </div>
        <div className="flex gap-2">
          {dep.status === "stopped" && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => wakeUpMut.mutate(id!, { onSuccess: () => refetch() })}
              disabled={wakeUpMut.isPending}
            >
              <Sun className="size-3.5" />
              Wake Up
            </Button>
          )}
          {dep.status === "active" && displayStatus.value === "active" && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                if (confirm("Pause this deployment? This will scale all workloads to zero.")) {
                  stopMut.mutate(id!, { onSuccess: () => { refetch(); showSuccess("Deployment paused — all workloads scaled to zero."); } });
                }
              }}
              disabled={stopMut.isPending}
            >
              <Pause className="size-3.5" />
              Pause
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const mismatchNote = dep.placement_mismatch
                ? "\n\nRouting will be synced to the account cluster before redeploy. Existing pods may stay on the previous cluster until the deploy worker finishes."
                : "";
              if (confirm(`Redeploy this deployment? This will rebuild and apply all K8s resources.${mismatchNote}`)) {
                reapplyMut.mutate(id!, {
                  onSuccess: (resp) => {
                    refetch();
                    const msg = resp.cluster_placement_updated && resp.message
                      ? resp.message
                      : "Redeploy initiated — K8s resources are being rebuilt.";
                    showSuccess(msg);
                  },
                });
              }
            }}
            disabled={reapplyMut.isPending || dep.status === "undeploying"}
          >
            <Play className="size-3.5" />
            Redeploy
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              if (confirm("Repair normalized spec? This will re-parse the stored spec and rebuild workloads/services/ingresses.")) {
                repairMut.mutate(id!, { onSuccess: () => { refetch(); showSuccess("Repair complete — spec re-parsed and resources rebuilt."); } });
              }
            }}
            disabled={repairMut.isPending}
          >
            <Wrench className="size-3.5" />
            Repair
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setPendingAdapters(data.adapters?.length ? data.adapters.sort().join(",") : "none");
              setAdaptersOpen(true);
            }}
          >
            <Layers className="size-3.5" />
            Set Adapters
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => {
              if (confirm("Delete this deployment?")) {
                deleteMut.mutate(id!, { onSuccess: () => navigate("/admin/deployments") });
              }
            }}
            disabled={deleteMut.isPending}
          >
            <Trash2 className="size-3.5" />
            Delete
          </Button>
        </div>
      </div>

      {/* Status banner */}
      {dep.status === "failed" && dep.error_message && (
        <div className="flex items-start gap-2 rounded-lg bg-red-50 p-3 text-sm text-red-800">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Deployment Failed</p>
            <p className="mt-0.5 text-red-700">{dep.error_message}</p>
            {dep.error_details && dep.error_details.length > 0 && (
              <ul className="mt-1.5 list-disc pl-4 text-red-700 space-y-0.5">
                {dep.error_details.map((d, i) => (
                  <li key={i}>
                    {d.kind && d.resource ? (
                      <><span className="font-medium">{d.kind}/{d.resource}:</span> {d.error}</>
                    ) : (
                      d.error
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
      {dep.status === "stopped" && (
        <div className="flex items-start gap-2 rounded-lg bg-gray-50 p-3 text-sm text-gray-800">
          <Pause className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Paused</p>
            <p className="mt-0.5 text-gray-700">This deployment was paused by an admin. All workloads are scaled to zero. Use the Wake Up button to restore it.</p>
          </div>
        </div>
      )}
      {isRuntimeDeploying && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-950 dark:text-amber-100">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
          <div>
            <p className="font-medium">Deploying in cluster</p>
            <p className="mt-0.5 text-amber-900/90 dark:text-amber-100/90">
              DB status is <span className="font-mono">active</span> but the agent workload is not ready yet
              {displayStatus.details ? ` — ${displayStatus.details}` : ""} (same as product UI).
            </p>
          </div>
        </div>
      )}
      {isTransitional && !isRuntimeDeploying && (
        <div className="flex items-center gap-2 rounded-lg bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950/40 dark:text-blue-100">
          <span className="relative flex size-2.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75" />
            <span className="relative inline-flex size-2.5 rounded-full bg-blue-500" />
          </span>
          <span className="capitalize">{displayStatus.value}…</span>
        </div>
      )}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(11.5rem,1fr))] gap-3 sm:gap-4">
        <InfoCard
          label="Status"
          value={displayStatus.value}
          mismatch={displayStatus.differsFromDB}
          mismatchHint={displayStatus.differsFromDB ? `DB record: ${dep.status}` : undefined}
        />
        <InfoCard
          label="Account"
          value={dep.account_name}
          href={
            dep.account_id
              ? `/admin/accounts?q=${encodeURIComponent(dep.account_id)}`
              : dep.account_name
                ? `/admin/accounts?q=${encodeURIComponent(dep.account_name)}`
                : undefined
          }
        />
        <InfoCard
          label="Deployment cluster"
          value={formatClusterId(dep.cluster_id)}
          mono
          mismatch={dep.placement_mismatch}
          mismatchHint={
            dep.placement_mismatch
              ? `Account pinned to ${formatClusterId(dep.account_cluster_id)}`
              : undefined
          }
        />
        <InfoCard
          label="Account cluster"
          value={formatClusterId(dep.account_cluster_id)}
          mono
          mismatch={dep.placement_mismatch}
          mismatchHint={
            dep.placement_mismatch
              ? `Deployment routes to ${formatClusterId(dep.cluster_id)}`
              : undefined
          }
        />
        <InfoCard label="Owner" value={dep.owner_email || "-"} />
        <InfoCard label="Build ID" value={dep.build_id} mono />
        <InfoCard label="Revision" value={dep.current_revision != null ? `rev ${dep.current_revision}` : "-"} />
        <InfoCard label="Created" value={formatDateTime(dep.created_at)} />
      </div>
      {data.placement_hint && dep.placement_mismatch && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-200/80 bg-amber-50/50 p-3 text-xs text-amber-950">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-700" />
          <p>{data.placement_hint}</p>
        </div>
      )}
      {(() => {
        const image = data.workloads?.[0]?.image;
        const ecrRegion = image ? ecrRegionFromImage(image) : null;
        if (!ecrRegion) return null;
        const depCluster = formatClusterId(dep.cluster_id);
        return (
          <p className="text-[11px] text-muted-foreground">
            Agent image ECR region: <span className="font-mono">{ecrRegion}</span>
            {depCluster !== "primary" && depCluster !== ecrRegion && (
              <> · deployment routes to <span className="font-mono">{depCluster}</span></>
            )}
          </p>
        );
      })()}

      <Dialog open={adaptersOpen} onOpenChange={setAdaptersOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Set Adapters</DialogTitle>
          </DialogHeader>
          <div className="py-4">
            <Select value={pendingAdapters} onValueChange={setPendingAdapters}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="slack">Slack</SelectItem>
                <SelectItem value="web">Web</SelectItem>
                <SelectItem value="slack,web">Slack + Web</SelectItem>
                <SelectItem value="none">None</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setAdaptersOpen(false)}>Cancel</Button>
            <Button
              size="sm"
              disabled={setAdaptersMut.isPending}
              onClick={() => {
                const adapters = pendingAdapters === "none" ? [] : pendingAdapters.split(",");
                setAdaptersMut.mutate({ id: id!, adapters }, {
                  onSuccess: () => {
                    refetch();
                    setAdaptersOpen(false);
                  },
                });
              }}
            >
              {setAdaptersMut.isPending ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {cs?.summary && (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
          <StatCard label="Pods" value={cs.summary.total_pods} sub={`${cs.summary.running_pods} running`} />
          <StatCard label="Deployments" value={cs.summary.total_deployments} />
          <StatCard label="Services" value={cs.summary.total_services} />
          <StatCard label="Ingresses" value={cs.summary.total_ingresses} />
          <StatCard label="Events" value={cs.summary.total_events} sub={cs.summary.warning_events > 0 ? `${cs.summary.warning_events} warnings` : undefined} warn={cs.summary.warning_events > 0} />
        </div>
      )}

      <Tabs
        value={activeTab}
        onValueChange={(tab) => {
          const next = new URLSearchParams(searchParams);
          if (tab === "events") next.delete("tab");
          else next.set("tab", tab);
          setSearchParams(next, { replace: true });
        }}
      >
        <TabsList variant="line">
          <TabsTrigger value="events">Events & Jobs</TabsTrigger>
          <TabsTrigger value="revisions">Revisions ({revisions?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="pods">Pods ({cs?.pods?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="variables">Variables ({data.variables?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="spec">Spec</TabsTrigger>
        </TabsList>

        <TabsContent value="events" className="space-y-4 mt-2">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* Event Timeline */}
            <div>
              <h3 className="text-sm font-medium text-muted-foreground mb-2">Event Timeline ({events?.length ?? 0})</h3>
              {events && events.length > 0 ? (
                <EventTimeline events={events} />
              ) : (
                <p className="text-xs text-muted-foreground">No events. Run "Backfill Revisions" on the deployments list to migrate legacy deployments.</p>
              )}
            </div>

            {/* Job History */}
            <div>
              <h3 className="text-sm font-medium text-muted-foreground mb-2">
                Job History ({jobsQuery.data?.jobs?.length ?? 0})
                {jobsQuery.data?.last_reconcile_at && (
                  <span className="ml-2 text-[10px] font-normal text-muted-foreground">
                    last reconcile: {formatDistanceToNow(new Date(jobsQuery.data.last_reconcile_at), { addSuffix: true })}
                  </span>
                )}
              </h3>
              {jobsQuery.isLoading && <Skeleton className="h-20 w-full" />}
              {jobsQuery.error && <p className="text-destructive text-sm">{jobsQuery.error.message}</p>}
              {highlightJobId != null && !Number.isNaN(highlightJobId) && (
                <p className="mb-2 text-xs text-muted-foreground">
                  Highlighting River job{" "}
                  <span className="font-mono text-foreground">{highlightJobId}</span>
                  {jobsQuery.data?.jobs?.every((j) => j.job_id !== highlightJobId) && jobsQuery.data?.jobs?.length
                    ? " (not in the last 25 jobs for this deployment — check River UI)"
                    : null}
                </p>
              )}
              {jobsQuery.data?.jobs?.length ? (
                <JobsTable jobs={jobsQuery.data.jobs} highlightJobId={highlightJobId} />
              ) : (
                !jobsQuery.isLoading && <p className="text-xs text-muted-foreground">No jobs found for this deployment.</p>
              )}
            </div>
          </div>

          {/* K8s Events */}
          {cs?.events?.length > 0 && (
            <div>
              <h3 className="text-sm font-medium text-muted-foreground mb-2">K8s Events ({cs.events.length})</h3>
              <div className="max-h-[400px] overflow-auto rounded-lg glass">
                <table className="w-full text-xs">
                  <thead className="sticky top-0 z-10 glass-subtle">
                    <tr className="border-b border-glass-border-honey">
                      <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Last Seen</th>
                      <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Type</th>
                      <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Reason</th>
                      <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Object</th>
                      <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Message</th>
                      <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Count</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[...cs.events].sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime()).map((ev, i) => (
                      <tr key={i} className="border-b border-comb-light">
                        <td className="px-3 py-1.5 text-muted-foreground" title={ev.last_seen}>
                          {ev.last_seen ? formatDistanceToNow(new Date(ev.last_seen), { addSuffix: true }) : "-"}
                        </td>
                        <td className={`px-3 py-1.5 ${ev.type === "Warning" ? "text-yellow-600" : ev.type === "Normal" ? "text-green-600" : "text-muted-foreground"}`}>{ev.type}</td>
                        <td className="px-3 py-1.5">{ev.reason}</td>
                        <td className="px-3 py-1.5 text-muted-foreground">{ev.involved_object}</td>
                        <td className="max-w-md whitespace-normal break-words px-3 py-1.5 text-muted-foreground">{ev.message}</td>
                        <td className="px-3 py-1.5 text-right text-muted-foreground">{ev.count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent value="revisions" className="mt-2">
          {revisions && revisions.length > 0 ? (
            <RevisionTable
              revisions={revisions}
              currentRevision={dep.current_revision}
              canRollback={dep.status === "active" || dep.status === "failed"}
              onRollback={(rev) => {
                if (confirm(`Rollback to revision ${rev}?`)) {
                  rollbackMut.mutate({ id: id!, revision: rev }, { onSuccess: () => refetch() });
                }
              }}
              isRollingBack={rollbackMut.isPending}
            />
          ) : (
            <p className="text-xs text-muted-foreground">No revisions found.</p>
          )}
        </TabsContent>

        <TabsContent value="pods" className="mt-2">
          {cs?.pods?.length > 0 ? (
            <div className="space-y-2">
              {cs.pods.map((pod) => (
                <div key={pod.name}>
                  <PodRow
                    pod={pod}
                    deploymentId={id!}
                    onSelect={setSelectedPod}
                    onRestart={(podName) => {
                      if (confirm(`Delete pod ${podName}? Kubernetes will recreate it.`)) {
                        restartPodMut.mutate({ id: id!, pod: podName });
                      }
                    }}
                    isRestarting={restartPodMut.isPending}
                  />
                  {selectedPod && selectedPod.name === pod.name && (
                    <PodDetail
                      deploymentId={selectedPod.deploymentId}
                      pod={selectedPod.name}
                      container={selectedPod.container}
                      mode={selectedPod.mode}
                      onClose={() => setSelectedPod(null)}
                    />
                  )}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No pods found.</p>
          )}
        </TabsContent>

        <TabsContent value="variables" className="mt-2">
          {data.variables?.length ? (
            <VariablesTable variables={data.variables} />
          ) : (
            <p className="text-xs text-muted-foreground">No variables configured for this deployment.</p>
          )}
        </TabsContent>

        <TabsContent value="spec" className="mt-2 space-y-2">
          {(() => {
            const specCluster = specTargetClusterId(data.spec_json);
            if (specCluster === null) return null;
            return (
              <div className="rounded-lg border border-glass-border-honey/60 bg-glass-light px-3 py-2 text-xs">
                <span className="text-muted-foreground">target.cluster_id in stored spec: </span>
                <span className="font-mono font-medium">{specCluster}</span>
              </div>
            );
          })()}
          {data.spec_json ? (
            <pre className="max-h-[600px] overflow-auto rounded-lg glass p-4 text-xs font-mono text-foreground">
              {(() => { try { return JSON.stringify(JSON.parse(data.spec_json), null, 2); } catch { return data.spec_json; } })()}
            </pre>
          ) : (
            <p className="text-xs text-muted-foreground">No spec available.</p>
          )}
        </TabsContent>
      </Tabs>

    </div>
  );
}

function EventTimeline({ events }: { events: DeploymentEvent[] }) {
  const statusColors: Record<string, string> = {
    active: "bg-green-500",
    pending: "bg-yellow-500",
    provisioning: "bg-blue-500",
    failed: "bg-red-500",
    undeploying: "bg-orange-500",
    undeployed: "bg-gray-400",
  };

  return (
    <div className="max-h-[400px] overflow-auto rounded-lg glass">
      <table className="w-full text-xs">
        <thead className="sticky top-0 z-10 glass-subtle">
          <tr className="border-b border-glass-border-honey">
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Time</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Status</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Message</th>
          </tr>
        </thead>
        <tbody>
          {events.map((ev, i) => (
            <tr key={i} className="border-b border-comb-light">
              <td className="px-3 py-1.5 text-muted-foreground whitespace-nowrap" title={formatDateTime(ev.created_at)}>
                {formatDistanceToNow(new Date(ev.created_at), { addSuffix: true })}
              </td>
              <td className="px-3 py-1.5 whitespace-nowrap">
                <span className="flex items-center gap-1.5">
                  <span className={`size-1.5 rounded-full shrink-0 ${statusColors[ev.status] ?? "bg-gray-400"}`} />
                  <span className="capitalize">{ev.status}</span>
                </span>
              </td>
              <td className="px-3 py-1.5 text-muted-foreground">{ev.message || "-"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RevisionTable({
  revisions,
  currentRevision,
  canRollback,
  onRollback,
  isRollingBack,
}: {
  revisions: DeploymentRevision[];
  currentRevision?: number;
  canRollback: boolean;
  onRollback: (revision: number) => void;
  isRollingBack: boolean;
}) {
  return (
    <div className="overflow-x-auto rounded-lg glass">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-glass-border-honey glass-subtle">
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Revision</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Build ID</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Created</th>
            <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Actions</th>
          </tr>
        </thead>
        <tbody>
          {revisions.map((rev) => {
            const isCurrent = rev.revision === currentRevision;
            return (
              <tr key={rev.revision} className={`border-b border-comb-light ${isCurrent ? "bg-amber-50/50" : ""}`}>
                <td className="px-3 py-1.5">
                  <span className="font-mono">rev {rev.revision}</span>
                  {isCurrent && (
                    <span className="ml-1.5 rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] text-amber-700">current</span>
                  )}
                </td>
                <td className="px-3 py-1.5 font-mono text-muted-foreground">{truncateUUID(rev.build_id)}</td>
                <td className="px-3 py-1.5 text-muted-foreground">{formatDateTime(rev.created_at)}</td>
                <td className="px-3 py-1.5 text-right">
                  {!isCurrent && canRollback && (
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => onRollback(rev.revision)}
                      disabled={isRollingBack}
                    >
                      <Undo2 className="size-3" />
                      Rollback
                    </Button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function JobsTable({ jobs, highlightJobId }: { jobs: DeploymentJob[]; highlightJobId?: number }) {
  const stateColors: Record<string, string> = {
    completed: "text-green-600",
    running: "text-blue-600",
    available: "text-yellow-600",
    retryable: "text-orange-600",
    discarded: "text-red-600",
    cancelled: "text-muted-foreground",
  };

  return (
    <div className="max-h-[400px] overflow-auto rounded-lg glass">
      <table className="w-full text-xs">
        <thead className="sticky top-0 z-10 glass-subtle">
          <tr className="border-b border-glass-border-honey">
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Time</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Job</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Kind</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Cluster</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">State</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Attempts</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Duration</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Errors</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((j, i) => {
            const duration = j.attempted_at && j.finalized_at
              ? `${((new Date(j.finalized_at).getTime() - new Date(j.attempted_at).getTime()) / 1000).toFixed(1)}s`
              : j.attempted_at ? "running..." : "-";

            // Parse River error JSON array to show last error
            let lastError = "";
            if (j.errors) {
              try {
                const errs = JSON.parse(j.errors);
                if (Array.isArray(errs) && errs.length > 0) {
                  const last = errs[errs.length - 1];
                  lastError = last.error || last.trace || JSON.stringify(last);
                }
              } catch {
                lastError = j.errors;
              }
            }

            const highlighted = highlightJobId != null && j.job_id === highlightJobId;

            return (
              <tr
                key={j.job_id ?? i}
                className={cn("border-b border-comb-light", highlighted && "bg-amber-500/10")}
              >
                <td className="px-3 py-1.5 text-muted-foreground" title={j.created_at}>
                  {formatDistanceToNow(new Date(j.created_at), { addSuffix: true })}
                </td>
                <td className="px-3 py-1.5 font-mono text-muted-foreground">
                  {j.job_id != null ? (
                    <Link to={`/admin/jobs?job=${j.job_id}`} className="text-honey-dark hover:underline">
                      {j.job_id}
                    </Link>
                  ) : (
                    "—"
                  )}
                </td>
                <td className="px-3 py-1.5 font-medium">{j.kind}</td>
                <td className="px-3 py-1.5 font-mono text-muted-foreground">{formatClusterId(j.cluster_id)}</td>
                <td className={`px-3 py-1.5 ${stateColors[j.state] ?? "text-muted-foreground"}`}>{j.state}</td>
                <td className="px-3 py-1.5 text-muted-foreground">{j.attempt}/{j.max_attempt}</td>
                <td className="px-3 py-1.5 text-muted-foreground">{duration}</td>
                <td className="max-w-sm whitespace-normal break-words px-3 py-1.5 text-red-600">{lastError}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function InfoCard({
  label,
  value,
  mono,
  mismatch,
  mismatchHint,
  href,
}: {
  label: string;
  value: string;
  mono?: boolean;
  mismatch?: boolean;
  mismatchHint?: string;
  href?: string;
}) {
  const valueClass = `mt-0.5 block break-words text-sm leading-snug ${
    mono ? "font-mono text-xs" : ""
  } ${mismatch ? "font-semibold text-amber-950" : ""}`;

  return (
    <div
      className={`rounded-lg px-3 py-2 ${
        mismatch ? "border border-amber-300/70 bg-amber-50/40 ring-1 ring-amber-200/50" : "glass"
      }`}
    >
      <p className={`text-xs ${mismatch ? "font-medium text-amber-900" : "text-muted-foreground"}`}>
        {label}
        {mismatch && <span className="ml-1 text-[10px] font-normal text-amber-700">≠</span>}
      </p>
      {href ? (
        <Link to={href} className={`${valueClass} text-blue-600 hover:underline`}>
          {value || "-"}
        </Link>
      ) : (
        <p className={valueClass}>{value || "-"}</p>
      )}
      {mismatchHint && (
        <p className="mt-1 text-[10px] leading-snug text-amber-800" title={mismatchHint}>
          {mismatchHint}
        </p>
      )}
    </div>
  );
}

function StatCard({ label, value, sub, warn }: { label: string; value: number; sub?: string; warn?: boolean }) {
  return (
    <div className="rounded-lg glass px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-lg font-semibold">{value}</p>
      {sub && <p className={`text-xs ${warn ? "text-yellow-600" : "text-muted-foreground"}`}>{sub}</p>}
    </div>
  );
}

function PodRow({
  pod,
  deploymentId,
  onSelect,
  onRestart,
  isRestarting,
}: {
  pod: K8sPodInfo;
  deploymentId: string;
  onSelect: (sel: { deploymentId: string; name: string; container?: string; mode: "logs" | "env" }) => void;
  onRestart: (podName: string) => void;
  isRestarting: boolean;
}) {
  const hasMultipleContainers = (pod.container_statuses?.length ?? 0) > 1;

  return (
    <div className="rounded-lg glass px-3 py-2">
      <div className="flex items-center justify-between">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{pod.name}</p>
          <p className="text-xs text-muted-foreground">
            {pod.phase} &middot; {pod.node_name} &middot; {pod.pod_ip}
          </p>
        </div>
        <div className="flex shrink-0 gap-1">
          {!hasMultipleContainers && (
            <>
              <Button variant="ghost" size="icon-xs" title="Logs" onClick={() => onSelect({ deploymentId, name: pod.name, container: pod.container_statuses?.[0]?.name, mode: "logs" })}>
                <FileText className="size-3.5" />
              </Button>
              <Button variant="ghost" size="icon-xs" title="Env" onClick={() => onSelect({ deploymentId, name: pod.name, mode: "env" })}>
                <Settings className="size-3.5" />
              </Button>
            </>
          )}
          <Button variant="ghost" size="icon-xs" title="Restart pod" onClick={() => onRestart(pod.name)} disabled={isRestarting}>
            <RotateCw className="size-3.5" />
          </Button>
        </div>
      </div>
      {/* Container statuses with per-container actions */}
      {pod.container_statuses?.length > 0 && (
        <div className="mt-1.5 space-y-1">
          {pod.container_statuses.map((cs) => (
            <div key={cs.name} className="flex items-center gap-2 text-[11px]">
              <span className={`size-1.5 rounded-full shrink-0 ${cs.ready ? "bg-green-500" : "bg-yellow-500"}`} />
              <span className="font-medium">{cs.name}</span>
              <span className={`${cs.state.startsWith("Running") ? "text-green-600" : cs.state.startsWith("Waiting") ? "text-yellow-600" : "text-red-600"}`}>
                {cs.state}
              </span>
              {cs.restart_count > 0 && (
                <span className="text-yellow-600">{cs.restart_count} restarts</span>
              )}
              <span className="truncate text-muted-foreground">{cs.image}</span>
              {hasMultipleContainers && (
                <span className="ml-auto flex shrink-0 gap-0.5">
                  <Button variant="ghost" size="icon-xs" title={`Logs: ${cs.name}`} onClick={() => onSelect({ deploymentId, name: pod.name, container: cs.name, mode: "logs" })}>
                    <FileText className="size-3" />
                  </Button>
                  <Button variant="ghost" size="icon-xs" title={`Env: ${cs.name}`} onClick={() => onSelect({ deploymentId, name: pod.name, container: cs.name, mode: "env" })}>
                    <Settings className="size-3" />
                  </Button>
                </span>
              )}
            </div>
          ))}
        </div>
      )}
      {/* Container resources */}
      {pod.containers?.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-[10px] text-muted-foreground">
          {pod.containers.map((c) => (
            <span key={c.name}>
              {c.name}: {c.request_cpu || "-"}/{c.limit_cpu || "-"} CPU, {c.request_memory || "-"}/{c.limit_memory || "-"} mem
              {c.image_pull_policy ? ` (${c.image_pull_policy})` : ""}
            </span>
          ))}
        </div>
      )}
      {/* Pod security + service account */}
      <div className="mt-1.5 space-y-0.5 text-[10px] text-muted-foreground">
        {(pod.pod_security || pod.service_account != null) && (
          <div className="flex flex-wrap gap-x-3">
            {pod.pod_security && (
              <span>
                uid={pod.pod_security.run_as_user ?? "-"} gid={pod.pod_security.run_as_group ?? "-"} fsGroup={pod.pod_security.fs_group ?? "-"}
                {pod.pod_security.seccomp_profile ? ` seccomp=${pod.pod_security.seccomp_profile}` : ""}
              </span>
            )}
            {pod.service_account && <span>sa={pod.service_account}</span>}
            {pod.automount_service_token != null && (
              <span>automount={pod.automount_service_token ? "true" : "false"}</span>
            )}
          </div>
        )}
        {/* Per-container security */}
        {pod.containers?.map((c) =>
          c.security ? (
            <div key={`sec-${c.name}`} className="flex flex-wrap gap-x-2">
              <span className="font-medium">{c.name}:</span>
              {c.security.run_as_non_root != null && <span>{c.security.run_as_non_root ? "nonRoot" : "root-ok"}</span>}
              {c.security.run_as_user != null && <span>uid={c.security.run_as_user}</span>}
              {c.security.read_only_root_filesystem != null && (
                <span className={c.security.read_only_root_filesystem ? "" : "text-yellow-600"}>
                  {c.security.read_only_root_filesystem ? "ro-fs" : "rw-fs"}
                </span>
              )}
              {c.security.allow_privilege_escalation != null && <span>{c.security.allow_privilege_escalation ? "escalation" : "no-escalation"}</span>}
              {c.security.privileged && <span className="text-red-600">PRIVILEGED</span>}
              {(c.security.capabilities?.length ?? 0) > 0 && <span>drop=[{c.security.capabilities!.join(",")}]</span>}
              {(c.security.add_capabilities?.length ?? 0) > 0 && <span className="text-yellow-600">add=[{c.security.add_capabilities!.join(",")}]</span>}
              {c.security.seccomp_profile && <span>seccomp={c.security.seccomp_profile}</span>}
            </div>
          ) : null
        )}
        {/* Volume mounts */}
        {pod.containers?.some((c) => c.volume_mounts?.length) && (
          <div className="mt-0.5">
            {pod.containers.map((c) =>
              c.volume_mounts?.length ? (
                <div key={`mnt-${c.name}`} className="flex flex-wrap gap-x-3">
                  <span className="font-medium">{c.name} mounts:</span>
                  {c.volume_mounts.map((vm) => (
                    <span key={vm.name}>
                      {vm.mount_path}{vm.sub_path ? `(${vm.sub_path})` : ""}{vm.read_only ? " ro" : ""}
                      <span className="text-muted-foreground/60"> [{vm.name}]</span>
                    </span>
                  ))}
                </div>
              ) : null
            )}
          </div>
        )}
        {/* Volumes */}
        {(pod.volumes?.length ?? 0) > 0 && (
          <div className="flex flex-wrap gap-x-3 mt-0.5">
            <span className="font-medium">volumes:</span>
            {pod.volumes!.map((v) => (
              <span key={v.name}>{v.name}={v.type}{v.source ? `:${v.source}` : ""}</span>
            ))}
          </div>
        )}
        {/* EnvFrom sources */}
        {pod.containers?.some((c) => c.env_from?.length) && (
          <div className="flex flex-wrap gap-x-3 mt-0.5">
            <span className="font-medium">envFrom:</span>
            {pod.containers.flatMap((c) => c.env_from ?? []).filter((v, i, a) => a.indexOf(v) === i).map((src) => (
              <span key={src}>{src}</span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function VariablesTable({ variables }: { variables: AdminVariable[] }) {
  return (
    <div className="overflow-x-auto rounded-lg glass">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-glass-border-honey glass-subtle">
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Value</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Targets</th>
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Flags</th>
          </tr>
        </thead>
        <tbody>
          {variables.map((v) => (
            <tr key={v.name} className="border-b border-comb-light">
              <td className="px-3 py-1.5 font-mono font-medium">{v.name}</td>
              <td className="px-3 py-1.5 font-mono text-muted-foreground">
                {v.secret ? <span className="text-yellow-600">***</span> : v.value || <span className="text-muted-foreground/40">-</span>}
              </td>
              <td className="px-3 py-1.5 text-muted-foreground">
                {v.targets?.length > 0 ? v.targets.join(", ") : <span className="text-muted-foreground/40">all</span>}
              </td>
              <td className="px-3 py-1.5">
                <span className="flex gap-1.5">
                  {v.secret && <span className="rounded-full bg-yellow-100 px-1.5 py-0.5 text-[10px] text-yellow-700">secret</span>}
                  {v.optional && <span className="rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] text-blue-700">optional</span>}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PodDetail({ deploymentId, pod, container, mode, onClose }: { deploymentId: string; pod: string; container?: string; mode: "logs" | "env"; onClose: () => void }) {
  const logsQuery = usePodLogs(mode === "logs" ? deploymentId : "", mode === "logs" ? pod : "", container);
  const envQuery = usePodEnv(mode === "env" ? deploymentId : "", mode === "env" ? pod : "");

  return (
    <div className="mt-1 rounded-lg glass-heavy p-4">
      <div className="mb-3 flex items-center justify-between">
        <h4 className="font-medium">{container ? `${container}` : pod} - {mode === "logs" ? "Logs" : "Environment"}</h4>
        <Button variant="ghost" size="xs" onClick={onClose}>Close</Button>
      </div>
      {mode === "logs" && (
        logsQuery.isLoading ? <Skeleton className="h-40 w-full" /> :
        logsQuery.error ? <p className="text-destructive text-sm">{logsQuery.error.message}</p> :
        <pre className="max-h-96 overflow-auto rounded glass-subtle p-3 text-xs text-foreground">{logsQuery.data?.logs || "No logs"}</pre>
      )}
      {mode === "env" && (
        envQuery.isLoading ? <Skeleton className="h-40 w-full" /> :
        envQuery.error ? <p className="text-destructive text-sm">{envQuery.error.message}</p> :
        <div className="space-y-3">
          {envQuery.data?.containers?.map((c) => (
            <div key={c.container}>
              <p className="mb-1 text-xs font-medium text-muted-foreground">{c.container}</p>
              <div className="overflow-x-auto rounded glass-subtle">
                <table className="w-full text-xs">
                  <tbody>
                    {c.vars?.map((v) => (
                      <tr key={v.name} className="border-b border-glass-border-honey">
                        <td className="whitespace-nowrap px-2 py-1 font-mono text-amber">{v.name}</td>
                        <td className="px-2 py-1 font-mono text-muted-foreground">{v.value || v.value_from || "-"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
