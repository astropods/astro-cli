import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useClusterMigrations } from "@/api/admin";
import { formatApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, formatClusterId } from "@/lib/utils";
import { formatDistanceToNow, parseISO } from "date-fns";
import { AlertTriangle, ExternalLink, RefreshCw } from "lucide-react";
import type { ClusterMigrationEvent, ClusterMigrationJob } from "@/types/admin";

function formatRelative(iso: string | undefined): string {
  if (!iso) return "—";
  try {
    return formatDistanceToNow(parseISO(iso), { addSuffix: true });
  } catch {
    return iso;
  }
}

function filterEvents(rows: ClusterMigrationEvent[], search: string): ClusterMigrationEvent[] {
  if (!search) return rows;
  const q = search.toLowerCase();
  return rows.filter(
    (r) =>
      r.account_name.toLowerCase().includes(q) ||
      r.agent_name.toLowerCase().includes(q) ||
      r.deployment_id.toLowerCase().includes(q) ||
      r.message.toLowerCase().includes(q),
  );
}

function filterJobs(rows: ClusterMigrationJob[], search: string): ClusterMigrationJob[] {
  if (!search) return rows;
  const q = search.toLowerCase();
  return rows.filter(
    (r) =>
      (r.account_name ?? "").toLowerCase().includes(q) ||
      (r.agent_name ?? "").toLowerCase().includes(q) ||
      r.deployment_id.toLowerCase().includes(q) ||
      r.kind.toLowerCase().includes(q) ||
      r.state.toLowerCase().includes(q) ||
      r.args_json.toLowerCase().includes(q) ||
      String(r.job_id).includes(q),
  );
}

function deploymentEventsHref(deploymentId: string, jobId?: number): string {
  const path = `/admin/deployments/${encodeURIComponent(deploymentId)}`;
  if (jobId == null) return `${path}?tab=events`;
  return `${path}?tab=events&job=${jobId}`;
}

function jobStateClass(state: string): string {
  if (state === "completed") return "text-emerald-600 dark:text-emerald-400";
  if (state === "running" || state === "available" || state === "pending") {
    return "text-sky-600 dark:text-sky-400";
  }
  if (state === "retryable" || state === "scheduled") return "text-amber-600 dark:text-amber-400";
  return "text-destructive";
}

function migrateRouteLabel(j: ClusterMigrationJob): string {
  if (j.kind !== "migrate_deployment_cluster") return "—";
  return `${formatClusterId(j.source_cluster_id)} → ${formatClusterId(j.target_cluster_id)}`;
}

export function MigrationsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const mismatchOnly = searchParams.get("mismatch") === "1";
  const [search, setSearch] = useState("");
  const { data, isLoading, isRefetching, error, refetch } = useClusterMigrations(mismatchOnly);

  const loadError = error ? formatApiError(error) : null;
  const events = useMemo(() => filterEvents(data?.events ?? [], search), [data?.events, search]);
  const jobs = useMemo(() => filterJobs(data?.jobs ?? [], search), [data?.jobs, search]);

  const setMismatchOnly = (enabled: boolean) => {
    const next = new URLSearchParams(searchParams);
    if (enabled) next.set("mismatch", "1");
    else next.delete("mismatch");
    setSearchParams(next, { replace: true });
  };

  return (
    <div className="space-y-6">
        <div>
          <h1 className="text-xl font-semibold">Cluster migrations</h1>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            Audit trail for account cluster moves: <code className="text-xs">deployment_events</code> and River{" "}
            <code className="text-xs">migrate_deployment_cluster</code> / <code className="text-xs">deploy</code> jobs.
            For placement alignment use{" "}
            <Link to="/admin/deployments?mismatch=1" className="text-honey-dark hover:underline">
              Deployments (mismatch)
            </Link>
            . Fast migrate rows are normal — that job only
            updates routing and enqueues <code className="text-xs">deploy</code>.
          </p>
        </div>

        {(data?.mismatch_count ?? 0) > 0 && (
          <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
            <div>
              <span className="font-medium">{data?.mismatch_count} placement mismatch(es)</span>
              {" — "}
              <Link to="/admin/deployments?mismatch=1" className="font-medium text-honey-dark hover:underline">
                View on Deployments
              </Link>
            </div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-3">
          <Input
            placeholder="Filter account, agent, deployment id, job id…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-8 max-w-xs text-sm"
          />
          <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={mismatchOnly}
              onChange={(e) => setMismatchOnly(e.target.checked)}
              className="size-3.5 rounded border"
            />
            Mismatched deployments only
          </label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8 gap-1.5 text-xs"
            disabled={isRefetching}
            onClick={() => void refetch()}
          >
            <RefreshCw className={cn("size-3", isRefetching && "animate-spin")} />
            Refresh
          </Button>
        </div>

        {error && loadError && (
          <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-md border border-destructive/40 bg-destructive/10 p-3 font-mono text-xs text-destructive">
            {loadError.status != null ? `HTTP ${loadError.status}\n` : ""}
            {loadError.detail}
          </pre>
        )}

        <section className="space-y-2">
          <h2 className="text-sm font-semibold">River jobs ({jobs.length})</h2>
          {isLoading ? (
            <TableSkeleton />
          ) : jobs.length === 0 ? (
            <p className="text-xs text-muted-foreground">No migrate/deploy jobs in the last {50} rows.</p>
          ) : (
            <JobsTable jobs={jobs} />
          )}
        </section>

        <section className="space-y-2">
          <h2 className="text-sm font-semibold">Migration events ({events.length})</h2>
          <p className="text-xs text-muted-foreground">
            <code className="text-xs">deployment_events</code> mentioning cluster placement or account migration.
          </p>
          {isLoading ? (
            <TableSkeleton />
          ) : events.length === 0 ? (
            <p className="text-xs text-muted-foreground">No migration-related events in the last {50} rows.</p>
          ) : (
            <EventsTable events={events} />
          )}
        </section>
    </div>
  );
}

function JobsTable({ jobs }: { jobs: ClusterMigrationJob[] }) {
  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-xs">
        <thead className="bg-muted/50 text-left">
          <tr>
            <th className="px-3 py-2 font-medium">When</th>
            <th className="px-3 py-2 font-medium">Job</th>
            <th className="px-3 py-2 font-medium">Kind</th>
            <th className="px-3 py-2 font-medium">Route</th>
            <th className="px-3 py-2 font-medium">State</th>
            <th className="px-3 py-2 font-medium">Agent</th>
            <th className="px-3 py-2 font-medium">Duration</th>
            <th className="px-3 py-2 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((j) => (
            <tr key={j.job_id} className={cn("border-t", j.errors && "bg-destructive/5")}>
              <td className="whitespace-nowrap px-3 py-1.5 text-muted-foreground">
                {formatRelative(j.created_at)}
              </td>
              <td className="px-3 py-1.5 font-mono">
                {j.deployment_id ? (
                  <Link
                    to={deploymentEventsHref(j.deployment_id, j.job_id)}
                    className="text-honey-dark hover:underline"
                  >
                    {j.job_id}
                  </Link>
                ) : (
                  j.job_id
                )}
              </td>
              <td className="px-3 py-1.5 font-mono">{j.kind}</td>
              <td className="px-3 py-1.5 font-mono">
                {j.kind === "migrate_deployment_cluster"
                  ? migrateRouteLabel(j)
                  : formatClusterId(j.deploy_cluster_id)}
              </td>
              <td className={cn("px-3 py-1.5 font-medium", jobStateClass(j.state))}>
                <div>
                  {j.state}
                  {j.errors ? (
                    <p className="mt-0.5 max-w-[14rem] truncate font-normal text-destructive" title={j.errors}>
                      {j.errors}
                    </p>
                  ) : null}
                </div>
              </td>
              <td className="px-3 py-1.5">
                {j.deployment_id ? (
                  <Link
                    to={deploymentEventsHref(j.deployment_id)}
                    className="hover:underline"
                    title={j.deployment_id}
                  >
                    {j.account_name}/{j.agent_name}
                  </Link>
                ) : (
                  "—"
                )}
              </td>
              <td className="px-3 py-1.5 font-mono" title={j.kind === "migrate_deployment_cluster" ? "Migrate job only updates DB and enqueues deploy" : undefined}>
                {j.finalized_at ? `${j.duration_ms}ms` : "—"}
              </td>
              <td className="px-3 py-1.5">
                {j.deployment_id ? (
                  <div className="flex flex-wrap gap-2">
                    <Link
                      to={deploymentEventsHref(j.deployment_id, j.job_id)}
                      className="text-honey-dark hover:underline"
                    >
                      Jobs
                    </Link>
                    <Link
                      to={`/admin/jobs?job=${j.job_id}`}
                      className="inline-flex items-center gap-0.5 text-honey-dark hover:underline"
                    >
                      Job
                      <ExternalLink className="size-3" />
                    </Link>
                  </div>
                ) : (
                  <Link
                    to={`/admin/jobs?job=${j.job_id}`}
                    className="inline-flex items-center gap-0.5 text-honey-dark hover:underline"
                  >
                    River
                    <ExternalLink className="size-3" />
                  </Link>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EventsTable({ events }: { events: ClusterMigrationEvent[] }) {
  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-xs">
        <thead className="bg-muted/50 text-left">
          <tr>
            <th className="px-3 py-2 font-medium">When</th>
            <th className="px-3 py-2 font-medium">Agent</th>
            <th className="px-3 py-2 font-medium">Status</th>
            <th className="px-3 py-2 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e, i) => (
            <tr key={`${e.deployment_id}-${e.created_at}-${i}`} className="border-t">
              <td className="whitespace-nowrap px-3 py-1.5 text-muted-foreground">
                {formatRelative(e.created_at)}
              </td>
              <td className="px-3 py-1.5">
                <Link
                  to={deploymentEventsHref(e.deployment_id)}
                  className="hover:underline"
                >
                  {e.account_name}/{e.agent_name}
                </Link>
              </td>
              <td className="px-3 py-1.5 font-mono">{e.status}</td>
              <td className="max-w-xl px-3 py-1.5">{e.message}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 4 }).map((_, i) => (
        <Skeleton key={i} className="h-8 w-full" />
      ))}
    </div>
  );
}
