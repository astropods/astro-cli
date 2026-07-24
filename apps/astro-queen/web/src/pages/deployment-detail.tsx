import { useState } from "react";
import { useParams, useNavigate, Link, useSearchParams } from "react-router";
import { useDeployment, useDeleteDeployment, useRestartPod, useWakeUpDeployment, useStopDeployment, useRollbackDeployment, useReapplyDeployment, useRepairNormalizedSpec, useDeploymentJobs, usePodLogs, usePodEnv } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Trash2, RotateCw, FileText, Settings, Sun, Undo2, AlertTriangle, Play, Pause, Wrench, CheckCircle2, Clock, Cog, UploadCloud, XCircle, Ban, CircleSlash, Loader2, type LucideIcon } from "lucide-react";
import { deriveDisplayDeploymentStatus, type DisplayDeploymentStatus } from "@/lib/display-deployment-status";
import {
  cn,
  formatDateTime,
  truncateUUID,
  formatClusterId,
  ecrRegionFromImage,
  specTargetClusterId,
} from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import type { K8sPodInfo, DeploymentEvent, DeploymentRevision, DeploymentJob, AdminVariable, AdminDeployment, K8sContainerStatus, K8sContainerResources } from "@/types/admin";

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
  const jobsQuery = useDeploymentJobs(id ?? "");
  const [selectedPod, setSelectedPod] = useState<{ deploymentId: string; name: string; container?: string; mode: "logs" | "env" } | null>(null);
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

      {(() => {
        const accountHref = dep.account_id
          ? `/admin/accounts?q=${encodeURIComponent(dep.account_id)}`
          : dep.account_name
            ? `/admin/accounts?q=${encodeURIComponent(dep.account_name)}`
            : undefined;
        return (
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-muted-foreground">
            <span>
              Account{" "}
              {accountHref ? (
                <Link to={accountHref} className="text-blue-600 hover:underline">{dep.account_name || "-"}</Link>
              ) : (
                <span className="text-foreground">{dep.account_name || "-"}</span>
              )}
            </span>
            <span>
              Cluster{" "}
              <span className={cn("font-mono", dep.placement_mismatch ? "font-medium text-amber-700" : "text-foreground")}>
                {formatClusterId(dep.cluster_id)}
              </span>
              {dep.placement_mismatch && (
                <span className="text-amber-700"> ≠ {formatClusterId(dep.account_cluster_id)}</span>
              )}
            </span>
            <span>Owner <span className="text-foreground">{dep.owner_email || "-"}</span></span>
            <span>
              Revision <span className="text-foreground">{dep.current_revision != null ? `rev ${dep.current_revision}` : "-"}</span>
            </span>
            <span>Created <span className="text-foreground">{formatDateTime(dep.created_at)}</span></span>
          </div>
        );
      })()}

      <DeploymentLifecycle dep={dep} display={displayStatus} />

      {dep.status === "stopped" && (
        <div className="flex items-start gap-2 rounded-lg bg-gray-50 p-3 text-sm text-gray-800">
          <Pause className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">Paused</p>
            <p className="mt-0.5 text-gray-700">This deployment was paused by an admin. All workloads are scaled to zero. Use the Wake Up button to restore it.</p>
          </div>
        </div>
      )}
      {data.placement_hint && dep.placement_mismatch && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-200/80 bg-amber-50/50 p-3 text-xs text-amber-950">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-700" />
          <p>{data.placement_hint}</p>
        </div>
      )}
      {(() => {
        const image = data.workloads?.[0]?.image;
        const ecrRegion = image ? ecrRegionFromImage(image) : null;
        const liveFrom = cs ? formatClusterId(cs.resolved_cluster_id) : null;
        if (!ecrRegion && !liveFrom) return null;
        return (
          <p className="text-[11px] text-muted-foreground">
            {liveFrom && <>Live K8s data from <span className="font-mono">{liveFrom}</span></>}
            {liveFrom && ecrRegion && " · "}
            {ecrRegion && <>agent image ECR region <span className="font-mono">{ecrRegion}</span></>}
          </p>
        );
      })()}

      {cs?.summary && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg glass px-3 py-2 text-xs text-muted-foreground">
          <StatItem value={cs.summary.total_pods} label="pods" note={cs.summary.total_pods > 0 ? `${cs.summary.running_pods} running` : undefined} />
          <StatItem value={cs.summary.total_deployments} label="deployments" />
          <StatItem value={cs.summary.total_services} label="services" />
          <StatItem value={cs.summary.total_ingresses} label="ingresses" />
          <StatItem
            value={cs.summary.total_events}
            label="events"
            note={cs.summary.warning_events > 0 ? `${cs.summary.warning_events} warnings` : undefined}
            warn={cs.summary.warning_events > 0}
          />
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
          <TabsTrigger value="pods">Pods ({cs?.pods?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="revisions">Revisions ({revisions?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="variables">Variables ({data.variables?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="spec">Spec</TabsTrigger>
        </TabsList>

        <TabsContent value="events" className="space-y-4 mt-2">
          <div className="space-y-4">
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
                <PodRow
                  key={pod.name}
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
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No pods found.</p>
          )}
          <Dialog open={!!selectedPod} onOpenChange={(open) => { if (!open) setSelectedPod(null); }}>
            {selectedPod && (
              <PodDetail
                deploymentId={selectedPod.deploymentId}
                pod={selectedPod.name}
                container={selectedPod.container}
                mode={selectedPod.mode}
              />
            )}
          </Dialog>
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

type LifecycleTone = "neutral" | "progress" | "good" | "bad" | "warn";

const STATE_META: Record<string, { label: string; icon: LucideIcon; tone: LifecycleTone }> = {
  pending:      { label: "Pending",      icon: Clock,        tone: "neutral" },
  provisioning: { label: "Provisioning", icon: Cog,          tone: "progress" },
  deploying:    { label: "Deploying",    icon: UploadCloud,  tone: "progress" },
  active:       { label: "Active",       icon: CheckCircle2, tone: "good" },
  failed:       { label: "Failed",       icon: XCircle,      tone: "bad" },
  stopped:      { label: "Paused",       icon: Pause,        tone: "neutral" },
  suspended:    { label: "Suspended",    icon: Ban,          tone: "warn" },
  undeploying:  { label: "Undeploying",  icon: Loader2,      tone: "warn" },
  undeployed:   { label: "Undeployed",   icon: CircleSlash,  tone: "neutral" },
};

const HAPPY_PATH = ["pending", "provisioning", "deploying", "active"] as const;
const HAPPY_RANK: Record<string, number> = { pending: 0, provisioning: 1, deploying: 2, active: 3 };
const EXCEPTION_STATES = ["failed", "stopped", "suspended", "undeploying", "undeployed"] as const;

const TONE_CURRENT: Record<LifecycleTone, string> = {
  neutral:  "border-blue-500 bg-blue-500/15 text-blue-600",
  progress: "border-blue-500 bg-blue-500/15 text-blue-600",
  good:     "border-green-500 bg-green-500/20 text-green-600",
  bad:      "border-red-500 bg-red-500/15 text-red-600",
  warn:     "border-amber-500 bg-amber-500/15 text-amber-600",
};

/**
 * Renders the deployment lifecycle state machine and marks where the current
 * deployment sits. The happy path (pending → provisioning → deploying → active)
 * is the spine; exception/teardown states hang below as off-ramps. Position is
 * driven by the DB status, with the one runtime nuance the product UI also shows:
 * DB `active` but workloads not yet ready reads as still-deploying.
 */
function DeploymentLifecycle({ dep, display }: { dep: AdminDeployment; display: DisplayDeploymentStatus }) {
  const status = dep.status;
  const inHappy = status in HAPPY_RANK;
  const isFailed = status === "failed";
  const isTeardown = status === "undeploying" || status === "undeployed";
  const isPaused = status === "stopped" || status === "suspended";
  // DB says active, but the agent workload isn't ready in-cluster yet.
  const runtimeDeploying = status === "active" && display.differsFromDB && display.value === "deploying";
  // A healthy, settled active deployment shouldn't pulse — it's at rest.
  const settledActive = status === "active" && !runtimeDeploying;
  // Only states actively transitioning should pulse; failed/paused are at rest.
  const inMotion =
    ["pending", "provisioning", "deploying", "undeploying"].includes(status) || runtimeDeploying;

  const nodeKind = (i: number): "done" | "current" | "upcoming" => {
    if (inHappy) {
      const rank = HAPPY_RANK[status];
      if (i < rank) return "done";
      if (i === rank) return settledActive ? "done" : "current";
      return "upcoming";
    }
    // Exception states: everything the deployment already cleared reads as done.
    if (isPaused || isTeardown) return "done"; // was active before diverging
    if (isFailed) return i < 3 ? "done" : "upcoming"; // active never reached (still recoverable)
    return "upcoming";
  };

  const meta = STATE_META[status] ?? { label: status, icon: CircleSlash, tone: "neutral" as LifecycleTone };
  const changedAgo = dep.status_changed_at
    ? formatDistanceToNow(new Date(dep.status_changed_at), { addSuffix: true })
    : null;
  // The failed banner below owns the error text; don't echo it here.
  const description = isFailed ? null : display.details;

  return (
    <div className="rounded-lg glass p-4">
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">Lifecycle</span>
          <StateChip label={meta.label} tone={meta.tone} pulse={inMotion} />
        </div>
        {(description || changedAgo) && (
          <p className="text-xs text-muted-foreground">
            {description}
            {changedAgo && (
              <span className="text-muted-foreground/60">
                {description ? " · " : ""}since {changedAgo}
              </span>
            )}
          </p>
        )}
      </div>

      {/* Happy-path spine */}
      <div className="flex items-start">
        {HAPPY_PATH.map((key, i) => {
          const kind = nodeKind(i);
          const nodeMeta = STATE_META[key];
          // The active node turns amber while runtime-deploying to flag the gap.
          const tone: LifecycleTone = key === "active" && runtimeDeploying ? "warn" : nodeMeta.tone;
          // The connector out of a completed node is filled; otherwise muted.
          const connectorFilled = kind === "done";
          return (
            <div key={key} className="flex flex-1 items-start last:flex-none">
              <LifecycleNode meta={{ ...nodeMeta, tone }} kind={kind} pulse={kind === "current"} />
              {i < HAPPY_PATH.length - 1 && (
                <div
                  className={cn(
                    "mt-4 h-0.5 flex-1 rounded-full",
                    connectorFilled ? "bg-honey/50" : "bg-border",
                  )}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Exception & teardown states */}
      <div className="mt-4 flex flex-wrap items-center gap-1.5 border-t border-glass-border-honey/50 pt-3">
        <span className="mr-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground/60">
          Off-ramps
        </span>
        {EXCEPTION_STATES.map((key) => (
          <ExceptionChip key={key} meta={STATE_META[key]} active={status === key} />
        ))}
      </div>

      {/* Failure reason, folded in subtly rather than as a separate red banner. */}
      {isFailed && dep.error_message && (
        <div className="mt-3 flex items-start gap-1.5 border-t border-red-500/15 pt-3 text-xs text-red-600/90">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <div className="min-w-0">
            <p className="font-medium">{dep.error_message}</p>
            {dep.error_details && dep.error_details.length > 0 && (
              <ul className="mt-1 list-disc space-y-0.5 pl-4 text-red-600/70">
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
    </div>
  );
}

function LifecycleNode({
  meta,
  kind,
  pulse,
}: {
  meta: { label: string; icon: LucideIcon; tone: LifecycleTone };
  kind: "done" | "current" | "upcoming";
  pulse: boolean;
}) {
  const Icon = meta.icon;
  const circle =
    kind === "current"
      ? TONE_CURRENT[meta.tone]
      : kind === "done"
        ? meta.tone === "good"
          ? "border-green-500/50 bg-green-500/15 text-green-600"
          : "border-honey/50 bg-honey/15 text-honey-dark"
        : "border-border bg-transparent text-muted-foreground/40";

  return (
    <div className="flex w-16 shrink-0 flex-col items-center gap-1.5">
      <span className={cn("relative flex size-8 items-center justify-center rounded-full border", circle)}>
        {pulse && (
          <span className="absolute inline-flex size-full animate-ping rounded-full border border-current opacity-40" />
        )}
        <Icon className={cn("size-4", meta.icon === Loader2 && kind === "current" && "animate-spin")} />
      </span>
      <span
        className={cn(
          "text-center text-[11px] leading-tight",
          kind === "upcoming" ? "text-muted-foreground/50" : "text-foreground",
          kind === "current" && "font-medium",
        )}
      >
        {meta.label}
      </span>
    </div>
  );
}

function StateChip({ label, tone, pulse }: { label: string; tone: LifecycleTone; pulse: boolean }) {
  const toneClass: Record<LifecycleTone, string> = {
    neutral:  "bg-blue-500/10 text-blue-600",
    progress: "bg-blue-500/10 text-blue-600",
    good:     "bg-green-500/10 text-green-600",
    bad:      "bg-red-500/10 text-red-600",
    warn:     "bg-amber-500/10 text-amber-600",
  };
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium", toneClass[tone])}>
      {pulse && (
        <span className="relative flex size-1.5">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-current opacity-75" />
          <span className="relative inline-flex size-1.5 rounded-full bg-current" />
        </span>
      )}
      {label}
    </span>
  );
}

function ExceptionChip({ meta, active }: { meta: { label: string; icon: LucideIcon; tone: LifecycleTone }; active: boolean }) {
  const Icon = meta.icon;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11px]",
        active ? cn("border", TONE_CURRENT[meta.tone]) : "text-muted-foreground/40",
      )}
    >
      <Icon className={cn("size-3", active && meta.icon === Loader2 && "animate-spin")} />
      {meta.label}
    </span>
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
    <div className="max-h-56 overflow-auto rounded-lg glass">
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
              <td className="px-3 py-1.5 text-muted-foreground whitespace-nowrap" title={ev.created_at}>
                {formatDateTime(ev.created_at)}
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

function StatItem({ value, label, note, warn }: { value: number; label: string; note?: string; warn?: boolean }) {
  return (
    <span>
      <span className={cn("font-medium", warn ? "text-yellow-600" : "text-foreground")}>{value}</span> {label}
      {note && <span className={warn ? "text-yellow-600" : "text-muted-foreground/70"}> ({note})</span>}
    </span>
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
  // Group every container's status + spec into one card so each container is
  // fully legible on its own, rather than smeared across parallel sections.
  const statusByName = new Map((pod.container_statuses ?? []).map((c) => [c.name, c]));
  const specByName = new Map((pod.containers ?? []).map((c) => [c.name, c]));
  const containerNames = Array.from(
    new Set([
      ...(pod.container_statuses ?? []).map((c) => c.name),
      ...(pod.containers ?? []).map((c) => c.name),
    ]),
  );

  return (
    <div className="space-y-2 rounded-lg glass px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{pod.name}</p>
          <p className="text-xs text-muted-foreground">
            {pod.phase} &middot; {pod.node_name} &middot; {pod.pod_ip}
          </p>
        </div>
        <Button variant="ghost" size="icon-xs" title="Restart pod" onClick={() => onRestart(pod.name)} disabled={isRestarting}>
          <RotateCw className="size-3.5" />
        </Button>
      </div>

      {/* One card per container */}
      {containerNames.length > 0 && (
        <div className="space-y-1.5">
          {containerNames.map((name) => (
            <ContainerCard
              key={name}
              name={name}
              status={statusByName.get(name)}
              spec={specByName.get(name)}
              deploymentId={deploymentId}
              podName={pod.name}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}

      {/* Pod-scoped context (applies to the whole pod, not a single container) */}
      {(pod.pod_security || pod.service_account != null || (pod.volumes?.length ?? 0) > 0) && (
        <div className="space-y-0.5 text-[10px] text-muted-foreground">
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
          {(pod.volumes?.length ?? 0) > 0 && (
            <div className="flex flex-wrap gap-x-3">
              <span className="font-medium">volumes:</span>
              {pod.volumes!.map((v) => (
                <span key={v.name}>{v.name}={v.type}{v.source ? `:${v.source}` : ""}</span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ContainerCard({
  name,
  status,
  spec,
  deploymentId,
  podName,
  onSelect,
}: {
  name: string;
  status?: K8sContainerStatus;
  spec?: K8sContainerResources;
  deploymentId: string;
  podName: string;
  onSelect: (sel: { deploymentId: string; name: string; container?: string; mode: "logs" | "env" }) => void;
}) {
  const sec = spec?.security;
  const stateClass = !status
    ? "text-muted-foreground"
    : status.state.startsWith("Running")
      ? "text-green-600"
      : status.state.startsWith("Waiting")
        ? "text-yellow-600"
        : "text-red-600";
  // Native sidecars are restartPolicy:Always init containers; plain init otherwise.
  const kindBadge = spec?.sidecar ? "sidecar" : (spec?.init || status?.init) ? "init" : null;

  return (
    <div className="rounded-md glass-subtle px-2.5 py-2">
      <div className="flex items-center gap-2">
        <span className={cn("size-1.5 shrink-0 rounded-full", status?.ready ? "bg-green-500" : "bg-yellow-500")} />
        <span className="text-xs font-medium">{name}</span>
        {kindBadge && (
          <span className="rounded-full bg-honey/15 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-honey-dark">
            {kindBadge}
          </span>
        )}
        {status && <span className={cn("text-[11px]", stateClass)}>{status.state}</span>}
        {status && status.restart_count > 0 && (
          <span className="text-[11px] text-yellow-600">{status.restart_count} restarts</span>
        )}
        <span className="ml-auto flex shrink-0 gap-0.5">
          <Button variant="ghost" size="icon-xs" title="Logs" onClick={() => onSelect({ deploymentId, name: podName, container: name, mode: "logs" })}>
            <FileText className="size-3.5" />
          </Button>
          <Button variant="ghost" size="icon-xs" title="Env" onClick={() => onSelect({ deploymentId, name: podName, container: name, mode: "env" })}>
            <Settings className="size-3.5" />
          </Button>
        </span>
      </div>
      {status?.image && (
        <p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">{status.image}</p>
      )}
      {spec && (
        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-muted-foreground">
          <span>{spec.request_cpu || "-"}/{spec.limit_cpu || "-"} CPU</span>
          <span>{spec.request_memory || "-"}/{spec.limit_memory || "-"} mem</span>
          {spec.image_pull_policy && <span>{spec.image_pull_policy}</span>}
          {sec?.run_as_non_root != null && <span>{sec.run_as_non_root ? "nonRoot" : "root-ok"}</span>}
          {sec?.run_as_user != null && <span>uid={sec.run_as_user}</span>}
          {sec?.read_only_root_filesystem != null && (
            <span className={sec.read_only_root_filesystem ? "" : "text-yellow-600"}>
              {sec.read_only_root_filesystem ? "ro-fs" : "rw-fs"}
            </span>
          )}
          {sec?.allow_privilege_escalation != null && <span>{sec.allow_privilege_escalation ? "escalation" : "no-escalation"}</span>}
          {sec?.privileged && <span className="text-red-600">PRIVILEGED</span>}
          {(sec?.capabilities?.length ?? 0) > 0 && <span>drop=[{sec!.capabilities!.join(",")}]</span>}
          {(sec?.add_capabilities?.length ?? 0) > 0 && <span className="text-yellow-600">add=[{sec!.add_capabilities!.join(",")}]</span>}
          {sec?.seccomp_profile && <span>seccomp={sec.seccomp_profile}</span>}
        </div>
      )}
      {(spec?.volume_mounts?.length ?? 0) > 0 && (
        <div className="mt-0.5 flex flex-wrap gap-x-3 text-[10px] text-muted-foreground">
          <span className="font-medium">mounts:</span>
          {spec!.volume_mounts!.map((vm) => (
            <span key={vm.name}>
              {vm.mount_path}{vm.sub_path ? `(${vm.sub_path})` : ""}{vm.read_only ? " ro" : ""}
              <span className="text-muted-foreground/60"> [{vm.name}]</span>
            </span>
          ))}
        </div>
      )}
      {(spec?.env_from?.length ?? 0) > 0 && (
        <div className="mt-0.5 flex flex-wrap gap-x-3 text-[10px] text-muted-foreground">
          <span className="font-medium">envFrom:</span>
          {spec!.env_from!.map((src) => (
            <span key={src}>{src}</span>
          ))}
        </div>
      )}
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

function PodDetail({ deploymentId, pod, container, mode }: { deploymentId: string; pod: string; container?: string; mode: "logs" | "env" }) {
  const logsQuery = usePodLogs(mode === "logs" ? deploymentId : "", mode === "logs" ? pod : "", container);
  const envQuery = usePodEnv(mode === "env" ? deploymentId : "", mode === "env" ? pod : "");

  return (
    <DialogContent className="sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle className="text-sm">
          {mode === "logs" ? "Logs" : "Environment"}
          <span className="ml-2 font-mono text-xs font-normal text-muted-foreground">
            {container ? `${container} · ${pod}` : pod}
          </span>
        </DialogTitle>
      </DialogHeader>
      {mode === "logs" && (
        logsQuery.isLoading ? <Skeleton className="h-96 w-full" /> :
        logsQuery.error ? <p className="text-destructive text-sm">{logsQuery.error.message}</p> :
        <pre className="max-h-[70vh] overflow-auto rounded-lg glass-subtle p-3 text-xs leading-relaxed text-foreground">{logsQuery.data?.logs || "No logs"}</pre>
      )}
      {mode === "env" && (
        envQuery.isLoading ? <Skeleton className="h-96 w-full" /> :
        envQuery.error ? <p className="text-destructive text-sm">{envQuery.error.message}</p> :
        <div className="max-h-[70vh] space-y-3 overflow-auto">
          {envQuery.data?.containers?.map((c) => (
            <div key={c.container}>
              <p className="mb-1 text-xs font-medium text-muted-foreground">{c.container}</p>
              <div className="overflow-x-auto rounded-lg glass-subtle">
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
    </DialogContent>
  );
}
