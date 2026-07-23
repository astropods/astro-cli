import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import {
  useJobKinds,
  useJobStates,
  useAdminQueues,
  useAdminJobs,
  usePauseQueue,
  useResumeQueue,
  useCancelJobs,
  useRetryJobs,
  useTriggerJob,
  useAdminJob,
  type AdminJob,
  type JobKindInfo,
} from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { formatDistanceToNow, parseISO } from "date-fns";
import { AlertTriangle, ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";

const PAGE_SIZE = 25;

const JOB_STATES = ["available", "running", "scheduled", "pending", "retryable", "completed", "discarded", "cancelled"] as const;

type HistoryCursor = {
  beforeId?: number;
  anchorId?: number;
};

function stateBadgeClass(state: string): string {
  switch (state) {
    case "completed": return "bg-emerald-100/60 text-emerald-700";
    case "running":
    case "available":
    case "pending": return "bg-sky-100/60 text-sky-700";
    case "retryable":
    case "scheduled": return "bg-amber-100/60 text-amber-700";
    case "discarded":
    case "cancelled": return "bg-red-100/60 text-red-700";
    default: return "bg-muted text-muted-foreground";
  }
}

function formatRelative(iso: string | null | undefined): string {
  if (!iso) return "—";
  try { return formatDistanceToNow(parseISO(iso), { addSuffix: true }); }
  catch { return iso; }
}

function TableSkeleton() {
  return (
    <div className="space-y-1">
      {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-8 w-full" />)}
    </div>
  );
}

function formatErrorMessage(error: unknown): string {
  const formatted = formatApiError(error);
  let detail = formatted.detail;
  try {
    const parsed = JSON.parse(detail) as { error?: unknown };
    if (typeof parsed.error === "string") detail = parsed.error;
  } catch {
  }
  return formatted.status != null ? `HTTP ${formatted.status}: ${detail}` : detail;
}

function ErrorNotice({ label, error }: { label: string; error: unknown }) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
      <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
      <div className="min-w-0">
        <p className="font-medium">{label}</p>
        <p className="mt-0.5 break-words font-mono text-[10px]">{formatErrorMessage(error)}</p>
      </div>
    </div>
  );
}

function StatCard({ label, value, warn }: { label: string; value: number; warn?: boolean }) {
  return (
    <div className="glass rounded px-3 py-2 text-center min-w-[80px]">
      <div className={cn("text-lg font-mono font-semibold leading-tight", warn && value > 0 ? "text-red-600" : "")}>{value}</div>
      <div className="text-[10px] text-muted-foreground capitalize">{label}</div>
    </div>
  );
}

function ExpandedJob({ job, colSpan }: { job: AdminJob; colSpan: number }) {
  const { data: detail, isLoading } = useAdminJob(job.id);
  const errors = detail?.errors ?? job.errors;

  return (
    <tr>
      <td colSpan={colSpan} className="bg-pollen-light px-4 py-3">
        <div className="space-y-2 text-xs">
          <div>
            <span className="font-medium text-muted-foreground">Args</span>
            <pre className="mt-1 overflow-x-auto rounded bg-white/60 px-2 py-1 font-mono text-[11px]">
              {JSON.stringify(job.args, null, 2)}
            </pre>
          </div>
          <div>
            <span className="font-medium text-muted-foreground">
              Errors {errors && errors.length > 0 ? `(${errors.length})` : ""}
            </span>
            {isLoading && !errors ? (
              <p className="mt-1 text-[10px] text-muted-foreground">Loading…</p>
            ) : !errors || errors.length === 0 ? (
              <p className="mt-1 text-[10px] text-muted-foreground">None</p>
            ) : (
              <div className="mt-1 space-y-1">
                {errors.map((e, i) => (
                  <div key={i} className="rounded bg-red-50 px-2 py-1 font-mono text-[11px] text-red-700">
                    <span className="font-semibold">#{e.attempt}</span> {formatRelative(e.at)} — {e.error}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </td>
    </tr>
  );
}

function TriggerModal({ kindInfo, onClose }: { kindInfo: JobKindInfo; onClose: () => void }) {
  const [argsText, setArgsText] = useState(() => JSON.stringify(kindInfo.args_schema, null, 2));
  const [parseError, setParseError] = useState("");
  const trigger = useTriggerJob();

  const handleSubmit = () => {
    let args: Record<string, unknown>;
    try {
      args = JSON.parse(argsText);
      setParseError("");
    } catch {
      setParseError("Invalid JSON");
      return;
    }
    trigger.mutate({ kind: kindInfo.kind, args }, { onSuccess: onClose });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div className="glass-heavy w-full max-w-lg rounded-lg p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <h3 className="mb-1 text-sm font-semibold">Trigger job</h3>
        <p className="mb-3 font-mono text-xs text-muted-foreground">{kindInfo.kind}</p>
        <textarea
          className="w-full rounded border border-glass-border-honey bg-white/60 p-2 font-mono text-xs focus:outline-none"
          rows={10}
          value={argsText}
          onChange={(e) => setArgsText(e.target.value)}
        />
        {parseError && <p className="mt-1 text-xs text-destructive">{parseError}</p>}
        {trigger.error && <p className="mt-1 text-xs text-destructive">{trigger.error.message}</p>}
        {trigger.data && <p className="mt-1 text-xs text-emerald-600">Enqueued job #{trigger.data.job_id}</p>}
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>Cancel</Button>
          <Button size="sm" onClick={handleSubmit} disabled={trigger.isPending}>
            {trigger.isPending ? "Triggering…" : "Trigger"}
          </Button>
        </div>
      </div>
    </div>
  );
}

// Per-worker job history: the state filter + paginated job list for a single kind,
// with retry/cancel actions and expandable args/errors. Replaces the old global
// History tab so history is always scoped to the worker you're looking at.
function WorkerHistory({ kind, anchorJobId }: { kind: string; anchorJobId?: number | null }) {
  const [stateFilter, setStateFilter] = useState(() => (anchorJobId != null ? "all" : "completed"));
  const [cursors, setCursors] = useState<HistoryCursor[]>([]);
  const [expandedId, setExpandedId] = useState<number | null>(anchorJobId ?? null);
  const highlightRef = useRef<HTMLTableRowElement | null>(null);

  const currentCursor = cursors[cursors.length - 1] ?? (anchorJobId != null ? { anchorId: anchorJobId } : {});
  const jobsQuery = useAdminJobs({
    state: stateFilter === "all" ? "" : stateFilter,
    kinds: [kind],
    limit: PAGE_SIZE,
    beforeId: currentCursor.beforeId,
    anchorId: currentCursor.anchorId,
  });

  const cancelJobs = useCancelJobs();
  const retryJobs = useRetryJobs();

  const jobs = jobsQuery.data?.jobs ?? [];
  const canGoBack = cursors.length > 0;
  const nextBeforeID = jobsQuery.data?.next_before_id;
  const canGoForward = Boolean(jobsQuery.data?.has_more && nextBeforeID != null);

  useEffect(() => { setCursors([]); }, [stateFilter]);

  useEffect(() => {
    if (highlightRef.current) {
      highlightRef.current.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  }, [jobs]);

  return (
    <div className="space-y-2 px-4 py-3">
      <Select value={stateFilter} onValueChange={setStateFilter}>
        <SelectTrigger className="h-7 w-36 text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">all states</SelectItem>
          {JOB_STATES.map((s) => <SelectItem key={s} value={s}>{s}</SelectItem>)}
        </SelectContent>
      </Select>

      {jobsQuery.isLoading ? (
        <TableSkeleton />
      ) : jobsQuery.error ? (
        <ErrorNotice label="Failed to load job history" error={jobsQuery.error} />
      ) : jobs.length === 0 ? (
        <p className="text-xs text-muted-foreground">No jobs match the current filter.</p>
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg glass">
            <table className="w-full text-xs whitespace-nowrap">
              <thead>
                <tr className="border-b border-glass-border-honey glass-subtle text-left text-[10px] text-muted-foreground">
                  <th className="px-3 py-2">ID</th>
                  <th className="px-3 py-2">Queue</th>
                  <th className="px-3 py-2">State</th>
                  <th className="px-3 py-2">Created</th>
                  <th className="px-3 py-2">Finalized</th>
                  <th className="px-3 py-2 text-right">Attempt</th>
                  <th className="px-3 py-2" />
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => {
                  const isHighlight = j.id === anchorJobId;
                  const isExpanded = expandedId === j.id;
                  return [
                    <tr
                      key={j.id}
                      ref={isHighlight ? highlightRef : undefined}
                      onClick={() => setExpandedId(isExpanded ? null : j.id)}
                      className={cn(
                        "cursor-pointer border-b border-glass-border-honey/40 last:border-0 hover:bg-pollen-light/40 transition-colors",
                        isHighlight ? "bg-amber-100/60" : "",
                        isExpanded ? "bg-pollen-light/60" : "",
                      )}
                    >
                      <td className="px-3 py-1.5 font-mono text-[10px] text-muted-foreground">{j.id}</td>
                      <td className="px-3 py-1.5 font-mono">{j.queue}</td>
                      <td className="px-3 py-1.5">
                        <span className={cn("rounded-full px-1.5 py-0.5 text-[10px]", stateBadgeClass(j.state))}>{j.state}</span>
                      </td>
                      <td className="px-3 py-1.5">{formatRelative(j.created_at)}</td>
                      <td className="px-3 py-1.5">{formatRelative(j.finalized_at)}</td>
                      <td className="px-3 py-1.5 text-right">{j.attempt}/{j.max_attempts}</td>
                      <td className="px-3 py-1.5" onClick={(e) => e.stopPropagation()}>
                        <div className="flex items-center gap-1">
                          {(j.state === "discarded" || j.state === "cancelled") && (
                            <Button size="sm" variant="outline" className="h-5 px-2 text-[10px]" onClick={() => retryJobs.mutate([j.id])}>Retry</Button>
                          )}
                          {(j.state === "available" || j.state === "scheduled" || j.state === "pending") && (
                            <Button size="sm" variant="outline" className="h-5 px-2 text-[10px]" onClick={() => { if (confirm(`Cancel job ${j.id}?`)) cancelJobs.mutate([j.id]); }}>Cancel</Button>
                          )}
                        </div>
                      </td>
                    </tr>,
                    isExpanded ? <ExpandedJob key={`${j.id}-exp`} job={j} colSpan={7} /> : null,
                  ];
                })}
              </tbody>
            </table>
          </div>

          {(canGoBack || canGoForward) && (
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>Page {cursors.length + 1} ({jobs.length} jobs)</span>
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="icon" disabled={!canGoBack} onClick={() => setCursors((c) => c.slice(0, -1))} className="size-6">
                  <ChevronLeft className="size-3.5" />
                </Button>
                <Button variant="ghost" size="icon" disabled={!canGoForward} onClick={() => nextBeforeID != null && setCursors((c) => [...c, { beforeId: nextBeforeID }])} className="size-6">
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

export function JobsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const highlightJob = searchParams.get("job");
  const parsedHighlightJobID = highlightJob ? Number(highlightJob) : null;
  const highlightJobID =
    parsedHighlightJobID != null && Number.isFinite(parsedHighlightJobID) ? parsedHighlightJobID : null;
  const activeTab = searchParams.get("tab") ?? (highlightJob ? "workers" : "overview");

  const [expandedKind, setExpandedKind] = useState<string | null>(null);
  const [triggerKind, setTriggerKind] = useState<JobKindInfo | null>(null);

  const jobKindsQuery = useJobKinds();
  const jobStatesQuery = useJobStates();
  const adminQueuesQuery = useAdminQueues();
  const runningJobsQuery = useAdminJobs({ state: "running", limit: 100 });
  const recentJobsQuery = useAdminJobs({ state: "completed", limit: 200 });

  // Deep-links land as ?job=<id>; resolve the job's kind so we can auto-expand
  // that worker's history and highlight the row inside it.
  const highlightJobDetail = useAdminJob(highlightJobID);
  const highlightKind = highlightJobDetail.data?.kind ?? null;

  const kinds = jobKindsQuery.data;
  const states = jobStatesQuery.data;
  const queues = adminQueuesQuery.data;
  const runningJobs = runningJobsQuery.data?.jobs;
  const recentJobs = recentJobsQuery.data?.jobs;

  const pauseQueue = usePauseQueue();
  const resumeQueue = useResumeQueue();
  const cancelJobs = useCancelJobs();
  const mutationError = pauseQueue.error ?? resumeQueue.error ?? cancelJobs.error;

  useEffect(() => {
    if (highlightKind) setExpandedKind(highlightKind);
  }, [highlightKind]);

  const setTab = (tab: string) => {
    const next = new URLSearchParams(searchParams);
    if (tab === "overview") next.delete("tab");
    else next.set("tab", tab);
    setSearchParams(next, { replace: true });
  };

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Jobs</h2>
      {mutationError && <ErrorNotice label="Job action failed" error={mutationError} />}

      <Tabs value={activeTab} onValueChange={setTab}>
        <TabsList variant="line">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="workers">
            Workers {kinds ? `(${kinds.length})` : ""}
          </TabsTrigger>
          <TabsTrigger value="running">
            Running {runningJobs && runningJobs.length > 0 ? `(${runningJobs.length})` : ""}
          </TabsTrigger>
        </TabsList>

        {/* Overview tab */}
        <TabsContent value="overview" className="space-y-5 mt-4">
          {/* State counts */}
          <div>
            <p className="mb-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Job States</p>
            {jobStatesQuery.error ? (
              <ErrorNotice label="Failed to load job state counts" error={jobStatesQuery.error} />
            ) : !states ? (
              <div className="flex gap-2">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-14 w-20" />)}</div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {JOB_STATES.map((s) => (
                  <StatCard
                    key={s}
                    label={s}
                    value={states[s as keyof typeof states]}
                    warn={s === "discarded"}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Queues */}
          <div>
            <p className="mb-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Queues</p>
            {adminQueuesQuery.error ? (
              <ErrorNotice label="Failed to load queues" error={adminQueuesQuery.error} />
            ) : !queues ? (
              <TableSkeleton />
            ) : (
              <div className="overflow-x-auto rounded-lg glass">
                <table className="w-full text-xs whitespace-nowrap">
                  <thead>
                    <tr className="border-b border-glass-border-honey glass-subtle text-left text-[10px] text-muted-foreground">
                      <th className="px-3 py-2">Queue</th>
                      <th className="px-3 py-2 text-right">Jobs waiting</th>
                      <th className="px-3 py-2 text-right">Jobs running</th>
                      <th className="px-3 py-2">Status</th>
                      <th className="px-3 py-2" />
                    </tr>
                  </thead>
                  <tbody>
                    {queues.map((q) => (
                      <tr key={q.name} className="border-b border-glass-border-honey/40 last:border-0">
                        <td className="px-3 py-1.5 font-mono">{q.name}</td>
                        <td className="px-3 py-1.5 text-right">{q.count_available}</td>
                        <td className="px-3 py-1.5 text-right">{q.count_running}</td>
                        <td className="px-3 py-1.5">
                          <span className={cn("rounded-full px-1.5 py-0.5 text-[10px]", q.paused_at ? "bg-amber-100/60 text-amber-700" : "bg-emerald-100/60 text-emerald-700")}>
                            {q.paused_at ? "paused" : "active"}
                          </span>
                        </td>
                        <td className="px-3 py-1.5">
                          {q.paused_at ? (
                            <Button size="sm" variant="outline" className="h-5 px-2 text-[10px]" onClick={() => resumeQueue.mutate(q.name)}>Resume</Button>
                          ) : (
                            <Button size="sm" variant="outline" className="h-5 px-2 text-[10px]" onClick={() => pauseQueue.mutate(q.name)}>Pause</Button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </TabsContent>

        {/* Running tab */}
        <TabsContent value="running" className="mt-4">
          {runningJobsQuery.error ? (
            <ErrorNotice label="Failed to load running jobs" error={runningJobsQuery.error} />
          ) : !runningJobs ? (
            <TableSkeleton />
          ) : runningJobs.length === 0 ? (
            <p className="text-xs text-muted-foreground">No running jobs.</p>
          ) : (
            <div className="overflow-x-auto rounded-lg glass">
              <table className="w-full text-xs whitespace-nowrap">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle text-left text-[10px] text-muted-foreground">
                    <th className="px-3 py-2">ID</th>
                    <th className="px-3 py-2">Kind</th>
                    <th className="px-3 py-2">Queue</th>
                    <th className="px-3 py-2">Started</th>
                    <th className="px-3 py-2 text-right">Attempt</th>
                    <th className="px-3 py-2" />
                  </tr>
                </thead>
                <tbody>
                  {runningJobs.map((j) => (
                    <tr key={j.id} className="border-b border-glass-border-honey/40 last:border-0">
                      <td className="px-3 py-1.5 font-mono text-[10px] text-muted-foreground">{j.id}</td>
                      <td className="px-3 py-1.5 font-mono">{j.kind}</td>
                      <td className="px-3 py-1.5 font-mono">{j.queue}</td>
                      <td className="px-3 py-1.5">{formatRelative(j.attempted_at)}</td>
                      <td className="px-3 py-1.5 text-right">{j.attempt}/{j.max_attempts}</td>
                      <td className="px-3 py-1.5">
                        <Button
                          size="sm" variant="outline" className="h-5 px-2 text-[10px]"
                          onClick={() => { if (confirm(`Cancel job ${j.id}?`)) cancelJobs.mutate([j.id]); }}
                        >
                          Cancel
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </TabsContent>

        {/* Workers tab */}
        <TabsContent value="workers" className="mt-4">
          {jobKindsQuery.error ? (
            <ErrorNotice label="Failed to load worker registry" error={jobKindsQuery.error} />
          ) : !kinds ? (
            <TableSkeleton />
          ) : (
            <>
              {recentJobsQuery.error && (
                <div className="mb-3">
                  <ErrorNotice label="Failed to load recent completed jobs" error={recentJobsQuery.error} />
                </div>
              )}
              <div className="overflow-x-auto rounded-lg glass">
                <table className="w-full text-xs whitespace-nowrap">
                  <thead>
                    <tr className="border-b border-glass-border-honey glass-subtle text-left text-[10px] text-muted-foreground">
                      <th className="px-3 py-2 w-4" />
                      <th className="px-3 py-2">Kind</th>
                      <th className="px-3 py-2">Queue (recent)</th>
                      <th className="px-3 py-2">Last seen</th>
                      <th className="px-3 py-2" />
                    </tr>
                  </thead>
                  <tbody>
                    {kinds.map((kindInfo) => {
                      const recent = (recentJobs ?? []).find((j) => j.kind === kindInfo.kind);
                      const isExpanded = expandedKind === kindInfo.kind;
                      return [
                        <tr
                          key={kindInfo.kind}
                          onClick={() => setExpandedKind(isExpanded ? null : kindInfo.kind)}
                          className={cn(
                            "cursor-pointer border-b border-glass-border-honey/40 last:border-0 hover:bg-pollen-light/40 transition-colors",
                            isExpanded ? "bg-pollen-light/60" : "",
                          )}
                        >
                          <td className="px-3 py-1.5 text-muted-foreground">
                            <ChevronDown className={cn("size-3.5 transition-transform", isExpanded ? "" : "-rotate-90")} />
                          </td>
                          <td className="px-3 py-1.5 font-mono text-honey-dark">{kindInfo.kind}</td>
                          <td className="px-3 py-1.5 font-mono text-muted-foreground">{recent?.queue ?? "—"}</td>
                          <td className="px-3 py-1.5 text-muted-foreground">{recent ? formatRelative(recent.created_at) : "—"}</td>
                          <td className="px-3 py-1.5" onClick={(e) => e.stopPropagation()}>
                            <Button size="sm" variant="outline" className="h-5 px-2 text-[10px]" onClick={() => setTriggerKind(kindInfo)}>
                              Trigger
                            </Button>
                          </td>
                        </tr>,
                        isExpanded ? (
                          <tr key={`${kindInfo.kind}-hist`}>
                            <td colSpan={5} className="bg-pollen-light/30 p-0">
                              <WorkerHistory
                                kind={kindInfo.kind}
                                anchorJobId={highlightKind === kindInfo.kind ? highlightJobID : null}
                              />
                            </td>
                          </tr>
                        ) : null,
                      ];
                    })}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </TabsContent>
      </Tabs>

      {triggerKind && <TriggerModal kindInfo={triggerKind} onClose={() => setTriggerKind(null)} />}
    </div>
  );
}
