import { useState } from "react";
import { useParams, useNavigate } from "react-router";
import { useDeployment, useDeleteDeployment, useRestartPod, useWakeUpDeployment, useRollbackDeployment, useReapplyDeployment, useRepairNormalizedSpec, useRefreshDriftReport, useDeploymentJobs, usePodLogs, usePodEnv } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ChevronDown, Trash2, RotateCw, FileText, Settings, Sun, Undo2, AlertTriangle, Info, Play, Wrench } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import type { K8sPodInfo, DeploymentEvent, DeploymentRevision, DeploymentJob, DriftReport, DriftResourceItem, AdminVariable } from "@/types/admin";

export function DeploymentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error, refetch } = useDeployment(id ?? "", 5_000);
  const deleteMut = useDeleteDeployment();
  const wakeUpMut = useWakeUpDeployment();
  const rollbackMut = useRollbackDeployment();
  const reapplyMut = useReapplyDeployment();
  const repairMut = useRepairNormalizedSpec();
  const refreshDriftMut = useRefreshDriftReport();
  const restartPodMut = useRestartPod();
  const jobsQuery = useDeploymentJobs(id ?? "");
  const [selectedPod, setSelectedPod] = useState<{ deploymentId: string; name: string; container?: string; mode: "logs" | "env" } | null>(null);

  // Auto-refresh when in transitional states
  const isTransitional = data?.deployment?.status && ["pending", "provisioning", "undeploying"].includes(data.deployment.status);

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-destructive">Error: {error.message}</p>;
  if (!data) return null;

  const { deployment: dep, cluster_status: cs, events, revisions } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{dep.name}</h2>
          <p className="text-sm text-muted-foreground">{dep.namespace}</p>
        </div>
        <div className="flex gap-2">
          {dep.status === "scaled_down" && (
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
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              if (confirm("Re-apply this deployment? This will rebuild and apply all K8s resources.")) {
                reapplyMut.mutate(id!, { onSuccess: () => refetch() });
              }
            }}
            disabled={reapplyMut.isPending || dep.status === "undeploying"}
          >
            <Play className="size-3.5" />
            Re-apply
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              if (confirm("Repair normalized spec? This will re-parse the stored spec and rebuild workloads/services/ingresses.")) {
                repairMut.mutate(id!, { onSuccess: () => refetch() });
              }
            }}
            disabled={repairMut.isPending}
          >
            <Wrench className="size-3.5" />
            Repair
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
          </div>
        </div>
      )}
      {dep.status === "scaled_down" && (
        <div className="flex items-start gap-2 rounded-lg bg-purple-50 p-3 text-sm text-purple-800">
          <Info className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Scaled Down</p>
            <p className="mt-0.5 text-purple-700">This deployment has been scaled to zero by KEDA. Use the Wake Up button to restore it.</p>
          </div>
        </div>
      )}
      {isTransitional && (
        <div className="flex items-center gap-2 rounded-lg bg-blue-50 p-3 text-sm text-blue-800">
          <span className="relative flex size-2.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75" />
            <span className="relative inline-flex size-2.5 rounded-full bg-blue-500" />
          </span>
          <span className="capitalize">{dep.status}...</span>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
        <InfoCard label="Status" value={dep.status} />
        <InfoCard label="Account" value={dep.account_name} />
        <InfoCard label="Build ID" value={dep.build_id} mono />
        <InfoCard label="Revision" value={dep.current_revision != null ? `rev ${dep.current_revision}` : "-"} />
        <InfoCard label="Created" value={formatDateTime(dep.created_at)} />
      </div>

      {cs?.summary && (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
          <StatCard label="Pods" value={cs.summary.total_pods} sub={`${cs.summary.running_pods} running`} />
          <StatCard label="Deployments" value={cs.summary.total_deployments} />
          <StatCard label="Services" value={cs.summary.total_services} />
          <StatCard label="Ingresses" value={cs.summary.total_ingresses} />
          <StatCard label="Events" value={cs.summary.total_events} sub={cs.summary.warning_events > 0 ? `${cs.summary.warning_events} warnings` : undefined} warn={cs.summary.warning_events > 0} />
        </div>
      )}

      <Tabs defaultValue="drift">
        <TabsList variant="line">
          <TabsTrigger value="drift">Drift Report</TabsTrigger>
          <TabsTrigger value="events">Events & Jobs</TabsTrigger>
          <TabsTrigger value="revisions">Revisions ({revisions?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="pods">Pods ({cs?.pods?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="variables">Variables ({data.variables?.length ?? 0})</TabsTrigger>
        </TabsList>

        <TabsContent value="drift" className="space-y-4 mt-2">
          <DriftReportSection
            report={data.drift_report}
            checkedAt={data.drift_checked_at}
            onRefresh={() => refreshDriftMut.mutate(id!, { onSuccess: () => refetch() })}
            isRefreshing={refreshDriftMut.isPending}
            error={refreshDriftMut.error}
          />
        </TabsContent>

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
              {jobsQuery.data?.jobs?.length ? (
                <JobsTable jobs={jobsQuery.data.jobs} />
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
    scaled_down: "bg-purple-500",
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

function JobsTable({ jobs }: { jobs: DeploymentJob[] }) {
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
            <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Kind</th>
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

            return (
              <tr key={i} className="border-b border-comb-light">
                <td className="px-3 py-1.5 text-muted-foreground" title={j.created_at}>
                  {formatDistanceToNow(new Date(j.created_at), { addSuffix: true })}
                </td>
                <td className="px-3 py-1.5 font-medium">{j.kind}</td>
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

function InfoCard({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg glass px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-0.5 truncate text-sm ${mono ? "font-mono text-xs" : ""}`}>{value || "-"}</p>
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
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// --- Drift Report Section (renders pre-computed drift report from server) ---

function DriftReportSection({ report, checkedAt, onRefresh, isRefreshing, error }: {
  report?: DriftReport;
  checkedAt?: string;
  onRefresh: () => void;
  isRefreshing: boolean;
  error?: Error | null;
}) {
  if (!report) {
    return (
      <div className="space-y-2">
        <div className="flex items-center gap-3 rounded-lg glass px-4 py-3">
          <p className="text-sm text-muted-foreground">No drift report available yet.</p>
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={isRefreshing}>
            <RotateCw className={`size-3.5 ${isRefreshing ? "animate-spin" : ""}`} />
            Check Now
          </Button>
        </div>
        {error && (
          <p className="text-sm text-destructive px-4">{error.message}</p>
        )}
      </div>
    );
  }

  const { summary } = report;

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {checkedAt && (
            <span>Checked {formatDistanceToNow(new Date(checkedAt), { addSuffix: true })}</span>
          )}
          {summary.match === summary.total && summary.total > 0 && (
            <span className="rounded-full bg-green-100 px-1.5 py-0.5 text-[10px] text-green-700">all match</span>
          )}
          {summary.missing > 0 && (
            <span className="rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] text-red-700">{summary.missing} missing</span>
          )}
          {summary.drift > 0 && (
            <span className="rounded-full bg-orange-100 px-1.5 py-0.5 text-[10px] text-orange-700">{summary.drift} drifted</span>
          )}
          {summary.extra > 0 && (
            <span className="rounded-full bg-yellow-100 px-1.5 py-0.5 text-[10px] text-yellow-700">{summary.extra} extra</span>
          )}
        </div>
        <Button variant="outline" size="xs" onClick={onRefresh} disabled={isRefreshing}>
          <RotateCw className={`size-3 ${isRefreshing ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>
      {error && (
        <p className="text-sm text-destructive">{error.message}</p>
      )}

      {report.workloads?.length > 0 && (
        <DriftTable title="Workloads" items={report.workloads} />
      )}
      {report.services?.length > 0 && (
        <DriftTable title="Services" items={report.services} />
      )}
      {report.ingresses?.length > 0 && (
        <DriftTable title="Ingresses" items={report.ingresses} />
      )}
      {(report.env_vars?.length ?? 0) > 0 && (
        <DriftTable title="Environment Variables" items={report.env_vars!} />
      )}
      {(report.secrets?.length ?? 0) > 0 && (
        <DriftTable title="Secrets" items={report.secrets!} />
      )}
    </div>
  );
}

type DriftStatus = "match" | "missing" | "extra" | "drift";

function DriftTable({ title, items }: { title: string; items: DriftResourceItem[] }) {
  const missingCount = items.filter((i) => i.status === "missing").length;
  const extraCount = items.filter((i) => i.status === "extra").length;
  const driftCount = items.filter((i) => i.status === "drift").length;

  // Collect all field keys across expected and actual
  const allKeys = new Set<string>();
  for (const item of items) {
    for (const k of Object.keys(item.expected ?? {})) allKeys.add(k);
    for (const k of Object.keys(item.actual ?? {})) allKeys.add(k);
  }
  const columns = Array.from(allKeys);

  const rowBg: Record<DriftStatus, string> = {
    match: "",
    missing: "bg-red-500/5",
    extra: "bg-yellow-500/5",
    drift: "bg-orange-500/5",
  };

  const statusLabel: Record<DriftStatus, { text: string; cls: string }> = {
    match: { text: "ok", cls: "text-green-600" },
    missing: { text: "missing", cls: "text-red-600" },
    extra: { text: "extra", cls: "text-yellow-600" },
    drift: { text: "drift", cls: "text-orange-600" },
  };

  return (
    <Collapsible defaultOpen>
      <CollapsibleTrigger className="flex items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground">
        <ChevronDown className="size-4" />
        {title} ({items.length})
        {missingCount > 0 && <span className="rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] text-red-700">{missingCount} missing</span>}
        {extraCount > 0 && <span className="rounded-full bg-yellow-100 px-1.5 py-0.5 text-[10px] text-yellow-700">{extraCount} extra</span>}
        {driftCount > 0 && <span className="rounded-full bg-orange-100 px-1.5 py-0.5 text-[10px] text-orange-700">{driftCount} drifted</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2">
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground" rowSpan={2}>Name</th>
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground" rowSpan={2}>Type</th>
                {columns.length > 0 && (
                  <>
                    <th className="px-3 py-1.5 text-center font-medium text-blue-600 border-l border-glass-border-honey" colSpan={columns.length}>Expected</th>
                    <th className="px-3 py-1.5 text-center font-medium text-green-600 border-l border-glass-border-honey" colSpan={columns.length}>Actual</th>
                  </>
                )}
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground border-l border-glass-border-honey" rowSpan={2}>Status</th>
              </tr>
              {columns.length > 0 && (
                <tr className="border-b border-glass-border-honey glass-subtle">
                  {columns.map((col) => (
                    <th key={`exp-${col}`} className="px-3 py-1 text-left font-normal text-muted-foreground text-[10px] first:border-l first:border-glass-border-honey">{col}</th>
                  ))}
                  {columns.map((col) => (
                    <th key={`cur-${col}`} className="px-3 py-1 text-left font-normal text-muted-foreground text-[10px] first:border-l first:border-glass-border-honey">{col}</th>
                  ))}
                </tr>
              )}
            </thead>
            <tbody>
              {items.map((item) => {
                const status = item.status as DriftStatus;
                return (
                  <tr key={item.name} className={`border-b border-comb-light ${rowBg[status] ?? ""}`}>
                    <td className="px-3 py-1.5 font-medium">{item.name}</td>
                    <td className="px-3 py-1.5 text-muted-foreground">{item.type}</td>
                    {columns.map((col, i) => (
                      <td key={`exp-${col}`} className={`px-3 py-1.5 font-mono text-muted-foreground ${i === 0 ? "border-l border-glass-border-honey" : ""}`}>
                        {item.expected?.[col] ?? <span className="text-muted-foreground/40">-</span>}
                      </td>
                    ))}
                    {columns.map((col, i) => {
                      const expVal = item.expected?.[col] ?? "";
                      const actVal = item.actual?.[col] ?? "";
                      const differs = status === "drift" && expVal !== actVal && expVal !== "" && actVal !== "";
                      return (
                        <td key={`cur-${col}`} className={`px-3 py-1.5 font-mono ${i === 0 ? "border-l border-glass-border-honey" : ""}`}>
                          {status === "missing" ? (
                            <span className="text-red-500">-</span>
                          ) : differs ? (
                            <span className="text-orange-600">{actVal}</span>
                          ) : (
                            actVal || <span className="text-muted-foreground/40">-</span>
                          )}
                        </td>
                      );
                    })}
                    <td className="px-3 py-1.5 border-l border-glass-border-honey">
                      <span className={statusLabel[status]?.cls ?? ""}>{statusLabel[status]?.text ?? status}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </CollapsibleContent>
    </Collapsible>
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
