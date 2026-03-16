import { useState } from "react";
import { useParams, useNavigate } from "react-router";
import { useDeployment, useDeleteDeployment, useRestartDeployment, useWakeUpDeployment, useRollbackDeployment, useReapplyDeployment, useDeploymentJobs, usePodLogs, usePodEnv } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { ChevronDown, Trash2, RotateCw, FileText, Settings, Sun, Undo2, AlertTriangle, Info, Play } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import type { K8sPodInfo, DeploymentEvent, DeploymentRevision, DeploymentJob, AdminWorkload, ClusterStatusResponse } from "@/types/admin";

export function DeploymentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error, refetch } = useDeployment(id ?? "", 5_000);
  const deleteMut = useDeleteDeployment();
  const restartMut = useRestartDeployment();
  const wakeUpMut = useWakeUpDeployment();
  const rollbackMut = useRollbackDeployment();
  const reapplyMut = useReapplyDeployment();
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
            onClick={() => restartMut.mutate(id!)}
            disabled={restartMut.isPending || dep.status !== "active"}
          >
            <RotateCw className="size-3.5" />
            Restart
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

      {/* Expected vs Current */}
      <ExpectedVsCurrent workloads={data.workloads} clusterStatus={cs} />

      {/* Event Timeline */}
      <Collapsible>
        <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
          <ChevronDown className="size-4" />
          Event Timeline ({events?.length ?? 0})
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-2">
          {events && events.length > 0 ? (
            <EventTimeline events={events} />
          ) : (
            <p className="text-xs text-muted-foreground">No events. Run "Backfill Revisions" on the deployments list to migrate legacy deployments.</p>
          )}
        </CollapsibleContent>
      </Collapsible>

      {/* Revision History */}
      {revisions && revisions.length > 0 && (
        <Collapsible>
          <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
            <ChevronDown className="size-4" />
            Revisions ({revisions.length})
          </CollapsibleTrigger>
          <CollapsibleContent className="mt-2">
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
          </CollapsibleContent>
        </Collapsible>
      )}

      {/* Job History */}
      <Collapsible>
        <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
          <ChevronDown className="size-4" />
          Job History ({jobsQuery.data?.jobs?.length ?? 0})
          {jobsQuery.data?.last_reconcile_at && (
            <span className="ml-2 text-[10px] font-normal text-muted-foreground">
              last reconcile: {formatDistanceToNow(new Date(jobsQuery.data.last_reconcile_at), { addSuffix: true })}
            </span>
          )}
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-2">
          {jobsQuery.isLoading && <Skeleton className="h-20 w-full" />}
          {jobsQuery.error && <p className="text-destructive text-sm">{jobsQuery.error.message}</p>}
          {jobsQuery.data?.jobs?.length ? (
            <JobsTable jobs={jobsQuery.data.jobs} />
          ) : (
            !jobsQuery.isLoading && <p className="text-xs text-muted-foreground">No jobs found for this deployment.</p>
          )}
        </CollapsibleContent>
      </Collapsible>

      {cs && (
        <>
          {cs.deployments?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
                <ChevronDown className="size-4" />
                K8s Deployments ({cs.deployments.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="overflow-x-auto rounded-lg glass">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-glass-border-honey glass-subtle">
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                        <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Replicas</th>
                        <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Ready</th>
                        <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Available</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Created</th>
                      </tr>
                    </thead>
                    <tbody>
                      {cs.deployments.map((d) => (
                        <tr key={d.name} className="border-b border-comb-light">
                          <td className="px-3 py-1.5">{d.name}</td>
                          <td className="px-3 py-1.5 text-right">{d.replicas}</td>
                          <td className={`px-3 py-1.5 text-right ${d.ready_replicas < d.replicas ? "text-yellow-600" : ""}`}>{d.ready_replicas}</td>
                          <td className="px-3 py-1.5 text-right">{d.available_replicas}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">{formatDateTime(d.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CollapsibleContent>
            </Collapsible>
          )}

          {cs.services?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
                <ChevronDown className="size-4" />
                Services ({cs.services.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="overflow-x-auto rounded-lg glass">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-glass-border-honey glass-subtle">
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Type</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Cluster IP</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Ports</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">External</th>
                      </tr>
                    </thead>
                    <tbody>
                      {cs.services.map((svc) => (
                        <tr key={svc.name} className="border-b border-comb-light">
                          <td className="px-3 py-1.5">{svc.name}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">{svc.type}</td>
                          <td className="px-3 py-1.5 font-mono text-muted-foreground">{svc.cluster_ip}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {svc.ports?.map((p) => `${p.port}→${p.target_port}/${p.protocol}`).join(", ")}
                          </td>
                          <td className="px-3 py-1.5 text-muted-foreground">{svc.external_ip?.join(", ") || "-"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CollapsibleContent>
            </Collapsible>
          )}

          {cs.ingresses?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
                <ChevronDown className="size-4" />
                Ingresses ({cs.ingresses.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="overflow-x-auto rounded-lg glass">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-glass-border-honey glass-subtle">
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Class</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Hosts</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Paths</th>
                        <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">TLS</th>
                      </tr>
                    </thead>
                    <tbody>
                      {cs.ingresses.map((ing) => (
                        <tr key={ing.name} className="border-b border-comb-light">
                          <td className="px-3 py-1.5">{ing.name}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">{ing.ingress_class_name || "-"}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {ing.rules?.map((r) => r.host).join(", ")}
                          </td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {ing.rules?.flatMap((r) => r.paths?.map((p) => `${p.path}→${p.backend_service}:${p.backend_port}`) ?? []).join(", ")}
                          </td>
                          <td className="px-3 py-1.5 text-muted-foreground">
                            {ing.tls?.length ? ing.tls.flatMap((t) => t.hosts).join(", ") : "-"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CollapsibleContent>
            </Collapsible>
          )}

          {cs.pods?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
                <ChevronDown className="size-4" />
                Pods ({cs.pods.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="space-y-2">
                  {cs.pods.map((pod) => (
                    <div key={pod.name}>
                      <PodRow pod={pod} deploymentId={id!} onSelect={setSelectedPod} />
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
              </CollapsibleContent>
            </Collapsible>
          )}

          {cs.events?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
                <ChevronDown className="size-4" />
                K8s Events ({cs.events.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="overflow-x-auto rounded-lg glass">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-glass-border-honey glass-subtle">
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
              </CollapsibleContent>
            </Collapsible>
          )}
        </>
      )}

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
    <div className="space-y-0">
      {events.map((ev, i) => (
        <div key={i} className="flex items-start gap-3 py-1.5">
          <div className="flex flex-col items-center">
            <div className={`size-2 rounded-full ${statusColors[ev.status] ?? "bg-gray-400"}`} />
            {i < events.length - 1 && <div className="w-px flex-1 bg-border" />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="text-[10px] text-muted-foreground">{formatDateTime(ev.created_at)}</span>
              <span className="text-xs font-medium capitalize">{ev.status}</span>
              <span className="text-[10px] text-muted-foreground">
                ({formatDistanceToNow(new Date(ev.created_at), { addSuffix: true })})
              </span>
            </div>
            {ev.message && <p className="text-xs text-muted-foreground">{ev.message}</p>}
          </div>
        </div>
      ))}
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
    <div className="overflow-x-auto rounded-lg glass">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-glass-border-honey glass-subtle">
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
}: {
  pod: K8sPodInfo;
  deploymentId: string;
  onSelect: (sel: { deploymentId: string; name: string; container?: string; mode: "logs" | "env" }) => void;
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
        {/* Pod-level env button (shows all containers) */}
        {!hasMultipleContainers && (
          <div className="flex shrink-0 gap-1">
            <Button variant="ghost" size="icon-xs" title="Logs" onClick={() => onSelect({ deploymentId, name: pod.name, container: pod.container_statuses?.[0]?.name, mode: "logs" })}>
              <FileText className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon-xs" title="Env" onClick={() => onSelect({ deploymentId, name: pod.name, mode: "env" })}>
              <Settings className="size-3.5" />
            </Button>
          </div>
        )}
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

type WorkloadStatus = "running" | "degraded" | "missing" | "pending";

function resolveWorkloadStatus(
  w: AdminWorkload,
  cs?: ClusterStatusResponse,
): { status: WorkloadStatus; ready: string; detail?: string } {
  const k8sDeps = cs?.deployments ?? [];
  const k8sPods = cs?.pods ?? [];

  // Sidecars: check for the container inside agent pods
  if (w.workload_type === "sidecar") {
    const containerName = w.component_kind === "interfaces" ? "messaging" : w.component_key || w.component_kind;
    const hasSidecar = k8sPods.some((p) => p.container_statuses?.some((c) => c.name === containerName));
    if (hasSidecar) {
      const broken = k8sPods.flatMap((p) =>
        (p.container_statuses ?? []).filter((c) => c.name === containerName && (!c.ready || c.restart_count > 2))
      );
      if (broken.length > 0) return { status: "degraded", ready: "sidecar", detail: `${broken[0].state}` };
      return { status: "running", ready: "sidecar" };
    }
    return { status: "missing", ready: "-" };
  }

  // Match by K8s deployment/statefulset name
  const dep = k8sDeps.find((d) => d.name === w.name);
  if (!dep) return { status: "missing", ready: `0/${w.replicas}` };

  const ready = `${dep.ready_replicas}/${dep.replicas}`;
  if (dep.ready_replicas >= dep.replicas && dep.replicas > 0) return { status: "running", ready };

  // Check pods for crash details
  const pods = k8sPods.filter((p) => p.name.startsWith(w.name));
  const crashing = pods.flatMap((p) =>
    (p.container_statuses ?? []).filter((c) => !c.ready || c.state.startsWith("Waiting") || c.restart_count > 2)
  );
  if (crashing.length > 0) {
    const worst = crashing[0];
    return { status: "degraded", ready, detail: `${worst.name}: ${worst.state}${worst.restart_count > 0 ? ` (${worst.restart_count} restarts)` : ""}` };
  }

  if (dep.ready_replicas === 0) return { status: "pending", ready };
  return { status: "degraded", ready };
}

const wStatusColors: Record<WorkloadStatus, string> = {
  running: "text-green-600",
  degraded: "text-yellow-600",
  missing: "text-red-600",
  pending: "text-blue-600",
};

const wStatusDots: Record<WorkloadStatus, string> = {
  running: "bg-green-500",
  degraded: "bg-yellow-500",
  missing: "bg-red-500",
  pending: "bg-blue-500",
};

const kindBadge: Record<string, string> = {
  agent: "bg-amber-100 text-amber-700",
  model: "bg-purple-100 text-purple-700",
  knowledge: "bg-blue-100 text-blue-700",
  tool: "bg-green-100 text-green-700",
  ingestion: "bg-orange-100 text-orange-700",
  interfaces: "bg-pink-100 text-pink-700",
  collector: "bg-gray-100 text-gray-700",
};

function shortImage(img: string) {
  const parts = img.split("/");
  return parts[parts.length - 1];
}

function ExpectedVsCurrent({ workloads, clusterStatus }: { workloads?: AdminWorkload[]; clusterStatus?: ClusterStatusResponse }) {
  if (!workloads || workloads.length === 0) return null;

  return (
    <Collapsible defaultOpen>
      <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground">
        <ChevronDown className="size-4" />
        Workloads ({workloads.length})
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2">
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Component</th>
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Kind</th>
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Image</th>
                <th className="px-3 py-1.5 text-center font-medium text-muted-foreground">Ready</th>
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Status</th>
                <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Detail</th>
              </tr>
            </thead>
            <tbody>
              {workloads.map((w) => {
                const { status, ready, detail } = resolveWorkloadStatus(w, clusterStatus);
                return (
                  <tr key={w.name} className="border-b border-comb-light">
                    <td className="px-3 py-1.5 font-medium">{w.name}</td>
                    <td className="px-3 py-1.5">
                      <span className={`inline-block rounded-full px-1.5 py-0.5 text-[10px] ${kindBadge[w.component_kind] ?? "bg-gray-100 text-gray-700"}`}>
                        {w.component_kind}{w.persistent ? " (pvc)" : ""}
                      </span>
                    </td>
                    <td className="px-3 py-1.5 font-mono text-muted-foreground" title={w.image}>{shortImage(w.image)}</td>
                    <td className="px-3 py-1.5 text-center font-mono">{ready}</td>
                    <td className="px-3 py-1.5">
                      <span className="flex items-center gap-1.5">
                        <span className={`size-1.5 rounded-full ${wStatusDots[status]}`} />
                        <span className={wStatusColors[status]}>{status}</span>
                      </span>
                    </td>
                    <td className="max-w-[250px] truncate px-3 py-1.5 text-muted-foreground" title={detail}>{detail ?? ""}</td>
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
