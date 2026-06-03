import { useMemo, useState, type ReactNode } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { formatTimeAgo } from "@/lib/time-format";
import { type AgentDeploymentRef } from "./AgentNameLink";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatCost, formatCompact, formatLatency } from "@/lib/format-utils";
import type {
  AccountDeploymentsSummaryResponse,
  AccountUsersSummaryResponse,
} from "@/lib/api";
import { AgentsUsedChips } from "./AgentsUsedChips";
import { UsersUsedAvatars } from "./UsersUsedAvatars";
import { UserBadge } from "@/components/UserBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Info, CircleUserRound, Server, TriangleAlert, Slack } from "lucide-react";
import { useAccountMembers } from "@/api/queries/accounts";
import { isSlackUserId } from "./user-classification";

// motion.create wraps TableRow so the agents-mode rows keep the shared
// row chrome (data-slot, border, interactive hover state) while
// AnimatePresence drives the opacity transition. Defined at module
// scope per Motion's guidance — wrapping inside the component body
// would create a new motion component on every render.
const MotionTableRow = motion.create(TableRow);

type DeploymentRow = AccountDeploymentsSummaryResponse["deployments"][number];
type UserRow = AccountUsersSummaryResponse["users"][number];

type AgentSortKey = "cost_usd" | "requests" | "cost_per_request" | "tok_per_request" | "p95_latency_ms";
type UserSortKey = "cost_usd" | "requests" | "tokens" | "last_seen";

function useSort<K extends string>(initial: K) {
  const [sortKey, setSortKey] = useState<K>(initial);
  const [asc, setAsc] = useState(false);
  function handleSort(key: K) {
    if (key === sortKey) setAsc((v) => !v);
    else { setSortKey(key); setAsc(false); }
  }
  return { sortKey, asc, handleSort };
}

function sortDirFor<K extends string>(active: K, asc: boolean, col: K) {
  return active === col ? (asc ? "asc" : "desc") : undefined;
}

function formatShare(cost: number, total: number): string {
  if (total <= 0) return "—";
  return `${((cost / total) * 100).toFixed(1)}%`;
}

function GhostRow({ columns }: { columns: number }) {
  return (
    <TableRow>
      {Array.from({ length: columns }).map((_, i) => (
        <TableCell key={i} className={i === 0 ? "pr-4" : ""}>
          <div className={cn("h-3.5 animate-pulse rounded bg-muted", i === 0 ? "w-[70%]" : "w-1/2")} />
        </TableCell>
      ))}
    </TableRow>
  );
}

type TopSpendersTableProps =
  | {
      mode: "agents";
      /** One row per deployment — multi-region deployments of the same
       *  agent_name surface as separate rows. */
      deployments: DeploymentRow[];
      loading: boolean;
      groupLabel?: string;
      /** Account name used to linkify the row. When omitted (e.g. a future
       *  Models view), the cell renders text-only. */
      account?: string;
      /** Denominator for the `% Total` column. Owned by the caller so it can
       *  stay stable across search-driven row filtering — otherwise typing in
       *  the search input shifts every visible percentage. */
      totalCost?: number;
      /** Rendered inside the table's bordered container, above the column
       *  headers — used for the view toggle + search row. */
      panelHeader?: ReactNode;
    }
  | {
      mode: "users";
      users: UserRow[];
      account: string;
      loading: boolean;
      /** Map of agent_name → all matching deployments. Forwarded to
       *  `AgentsUsedChips` so each chip / popover row routes to a
       *  deployment's Monitor tab instead of the blueprint detail page. */
      deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
      /** Denominator for the `% Total` column. Owned by the caller — see the
       *  agents-mode note. */
      totalCost?: number;
      /** Rendered inside the table's bordered container, above the column
       *  headers — used for the view toggle + search row. */
      panelHeader?: ReactNode;
    };

export function TopSpendersTable(props: TopSpendersTableProps) {
  if (props.mode === "users") {
    return <UsersTopSpenders {...props} />;
  }
  return <DeploymentsTopSpenders {...props} />;
}

// ── Agents mode ──────────────────────────────────────────────────────────────

function DeploymentsTopSpenders({
  deployments,
  loading,
  groupLabel = "Name",
  account,
  totalCost,
  panelHeader,
}: Extract<TopSpendersTableProps, { mode: "agents" }>) {
  const { sortKey, asc, handleSort } = useSort<AgentSortKey>("cost_usd");

  const sorted = useMemo(
    () => [...deployments].sort((a, b) => {
      const diff = a[sortKey] - b[sortKey];
      return asc ? diff : -diff;
    }),
    [deployments, sortKey, asc],
  );

  // Fall back to a self-computed sum when the caller hasn't wired a stable
  // denominator — percentages still render, they just re-base if rows get
  // filtered.
  const denom = totalCost ?? deployments.reduce((s, b) => s + b.cost_usd, 0);

  const dir = (col: AgentSortKey) => sortDirFor(sortKey, asc, col);

  return (
    <Table header={panelHeader}>
      <TableHeader>
        <TableRow>
          <TableHead>{groupLabel}</TableHead>
          <TableHead>People</TableHead>
          <TableHead sortable sortDirection={dir("requests")} onSort={() => handleSort("requests")} className="text-right">Requests</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Total Spend</TableHead>
          <TableHead className="text-right">% Total</TableHead>
          <TableHead sortable sortDirection={dir("cost_per_request")} onSort={() => handleSort("cost_per_request")} className="text-right">Spend/Req</TableHead>
          <TableHead sortable sortDirection={dir("tok_per_request")} onSort={() => handleSort("tok_per_request")} className="text-right">Tok/Req</TableHead>
          <TableHead sortable sortDirection={dir("p95_latency_ms")} onSort={() => handleSort("p95_latency_ms")} className="text-right">P95</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {loading ? (
          Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={8} />)
        ) : sorted.length === 0 ? (
          <TableRow>
            <TableCell colSpan={8} className="py-10 text-center text-body-sm text-faint-foreground">
              No deployment activity in this period
            </TableCell>
          </TableRow>
        ) : (
          // AnimatePresence handles the "Show deleted" toggle smoothly:
          // rows fade in/out instead of snapping. initial={false} prevents
          // the first mount of the table from animating (only subsequent
          // membership changes do). Live rows keep their identity across
          // toggles since the key (deployment_id) is stable, so only the
          // archived rows actually animate.
          <AnimatePresence initial={false}>
          {sorted.map((b) => {
            // Zero requests in the period means no traces ever landed for
            // this deployment — usually the agent isn't sending observability
            // data. Flag with a small warning icon so the all-zero row reads
            // as "not instrumented" rather than "deployment did nothing".
            const notInstrumented = b.requests === 0;
            // Archived deployments still surface in the table when the user
            // toggles "Show archived". The backend's `is_archived` flag is
            // the source of truth — `undeployed_at` can be nil even on
            // archived rows (e.g. status='undeploying' mid-tear-down), so
            // checking the date alone misses tombstones.
            const isDeleted = !!b.is_archived;
            const label = b.display_name || b.agent_name;
            // The muted avatar + grayed text is enough to read as a
            // tombstone row — no status dot needed. Hovering the identity
            // unit still surfaces the deletion date via the tooltip
            // wrapper below. Date includes the year because deletions can
            // be years old and an unqualified "Apr 10" reads ambiguously.
            const deletedTooltipLabel = b.undeployed_at
              ? `Deleted ${new Date(b.undeployed_at).toLocaleDateString("en-US", {
                  month: "short",
                  day: "numeric",
                  year: "numeric",
                  timeZone: "UTC",
                })}`
              : "Deleted";
            const identityRow = (
              <span className="inline-flex min-w-0 items-center gap-2">
                {account && (
                  <BlueprintIdentity
                    account={account}
                    name={b.agent_name}
                    size={20}
                    className={cn("size-5 shrink-0 rounded-full", isDeleted && "opacity-60")}
                  />
                )}
                <span
                  className={cn(
                    "min-w-0 truncate font-medium",
                    isDeleted ? "text-muted-foreground" : "text-foreground",
                  )}
                >
                  {label}
                </span>
              </span>
            );
            const nameNode = isDeleted ? (
              <TooltipProvider delayDuration={150}>
                <Tooltip>
                  <TooltipTrigger asChild>{identityRow}</TooltipTrigger>
                  <TooltipContent side="top">{deletedTooltipLabel}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ) : (
              identityRow
            );
            // Live rows deep-link to their deployment's Monitor tab — rows
            // are per-deployment, so the Monitor view is the most direct
            // landing target. Deleted rows render a non-interactive span
            // (the deployment is gone; a click would 404).
            return (
              <MotionTableRow
                key={b.deployment_id}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.18, ease: "easeOut" }}
              >
                <TableCell className="pr-4">
                  <span className="inline-flex items-center gap-1.5">
                    {account && !isDeleted ? (
                      <Link
                        to={`/${account}/agents/${b.deployment_id}/monitor`}
                        className="inline-flex items-center hover:underline"
                      >
                        {nameNode}
                      </Link>
                    ) : (
                      nameNode
                    )}
                    {notInstrumented && !isDeleted && (
                      <TooltipProvider delayDuration={200}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span
                              className="text-faint-foreground"
                              aria-label="Not instrumented"
                            >
                              <TriangleAlert className="size-3.5" />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent side="top" className="max-w-[240px] [text-wrap:initial]">
                            Instrumentation not available — no requests, spend, or token data available.
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    )}
                  </span>
                </TableCell>
                <TableCell className={cn(isDeleted && "opacity-60")}>
                  <UsersUsedAvatars userIds={b.users_used ?? []} account={account ?? ""} />
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {formatCompact(b.requests)}
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {formatCost(b.cost_usd)}
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {formatShare(b.cost_usd, denom)}
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {formatCost(b.cost_per_request)}
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {formatCompact(b.tok_per_request)}
                </TableCell>
                <TableCell className={cn("text-right", isDeleted ? "text-muted-foreground" : "text-foreground")}>
                  {b.p95_latency_ms > 0 ? formatLatency(b.p95_latency_ms) : "—"}
                </TableCell>
              </MotionTableRow>
            );
          })}
          </AnimatePresence>
        )}
      </TableBody>
    </Table>
  );
}

// ── Users mode ───────────────────────────────────────────────────────────────

function userSortValue(u: UserRow, k: UserSortKey): number {
  switch (k) {
    case "cost_usd": return u.cost_usd;
    case "requests": return u.requests;
    case "tokens": return u.tokens;
    case "last_seen": return u.last_seen ? new Date(u.last_seen).getTime() : 0;
  }
}

function formatLastSeen(ts: string | undefined): string {
  if (!ts) return "—";
  return formatTimeAgo(ts);
}

type AgentRef = UserRow["agents_used"][number];

interface UserMetrics {
  cost_usd: number;
  requests: number;
  tokens: number;
  last_seen?: string;
  agents_used: AgentRef[];
}

interface Aggregate {
  count: number;
  metrics: UserMetrics;
}

function aggregateUsers(rows: UserRow[]): Aggregate | null {
  if (rows.length === 0) return null;
  let lastSeen: string | undefined;
  // Dedupe by deployment_id so two deployments of the same blueprint each
  // render their own chip — mirrors the server-side dedup in buildUsersSummary
  // and the "one row per deployment" shape on the Agents tab.
  const agents = new Map<string, AgentRef>();
  let cost = 0, reqs = 0, tokens = 0;
  for (const r of rows) {
    cost += r.cost_usd;
    reqs += r.requests;
    tokens += r.tokens;
    if (r.last_seen && (!lastSeen || r.last_seen > lastSeen)) lastSeen = r.last_seen;
    for (const a of r.agents_used) agents.set(a.deployment_id, a);
  }
  return {
    count: rows.length,
    metrics: {
      cost_usd: parseFloat(cost.toFixed(2)),
      requests: reqs,
      tokens,
      last_seen: lastSeen,
      agents_used: [...agents.values()],
    },
  };
}

function MetricsCells({
  row,
  totalCost,
  deploymentsByAgent,
}: {
  row: UserMetrics;
  totalCost: number;
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
}) {
  return (
    <>
      <TableCell>
        <AgentsUsedChips agents={row.agents_used} deploymentsByAgent={deploymentsByAgent} />
      </TableCell>
      <TableCell className="text-right text-foreground">{formatCost(row.cost_usd)}</TableCell>
      <TableCell className="text-right text-foreground">{formatShare(row.cost_usd, totalCost)}</TableCell>
      <TableCell className="text-right text-foreground">{formatCompact(row.requests)}</TableCell>
      <TableCell className="text-right text-foreground">{formatCompact(row.tokens)}</TableCell>
      <TableCell className="text-right text-foreground">{formatLastSeen(row.last_seen)}</TableCell>
    </>
  );
}

function UsersTopSpenders({
  users,
  account,
  loading,
  deploymentsByAgent,
  totalCost,
  panelHeader,
}: Extract<TopSpendersTableProps, { mode: "users" }>) {
  const { sortKey, asc, handleSort } = useSort<UserSortKey>("cost_usd");

  // Members feeds classification — without it every named user falls into
  // Unidentified. Treat its load as part of the table's loading state so the
  // skeleton stays up until classification can be trusted.
  const { data: membersData, isLoading: membersLoading } = useAccountMembers(account);
  const memberIds = useMemo(
    () => new Set(membersData?.members.map((m) => m.user_id) ?? []),
    [membersData],
  );
  const isLoading = loading || membersLoading;

  // Three "real" buckets land in the table:
  //   - rows: per-id rows for named members (WorkOS user → account member)
  //     and unlinked Slack users (bare `U07ABCDEF` — the only format the
  //     messaging adapter writes for unlinked senders). They're merged
  //     into one sorted list so a Slack user who's out-spending a named
  //     member sorts above them — the row label differentiates them
  //     visually (UserBadge vs SlackUserIdentity).
  //   - unidentified: everything else that isn't empty (rare; arbitrary
  //     trace user_ids agents may emit). Stays aggregated.
  //   - unattributed: empty user_id. Stays aggregated (system spend).
  const { rows, unidentified, unattributed } = useMemo(() => {
    const realRows: UserRow[] = [];
    const unidentifiedRows: UserRow[] = [];
    const unattributedRows: UserRow[] = [];
    for (const u of users) {
      if (!u.user_id) unattributedRows.push(u);
      else if (memberIds.has(u.user_id) || isSlackUserId(u.user_id)) realRows.push(u);
      else unidentifiedRows.push(u);
    }
    realRows.sort((a, b) => {
      const diff = userSortValue(a, sortKey) - userSortValue(b, sortKey);
      return asc ? diff : -diff;
    });
    return {
      rows: realRows,
      unidentified: aggregateUsers(unidentifiedRows),
      unattributed: aggregateUsers(unattributedRows),
    };
  }, [users, memberIds, sortKey, asc]);

  const dir = (col: UserSortKey) => sortDirFor(sortKey, asc, col);
  const totalRows = rows.length + (unidentified ? 1 : 0) + (unattributed ? 1 : 0);
  // Fall back to a self-computed sum when the caller hasn't wired a stable
  // denominator. The fallback derives from the filtered rows + buckets, so
  // search interactions will re-base the percentages — same caveat as the
  // agents-mode branch.
  const denom = totalCost ?? (
    rows.reduce((s, u) => s + u.cost_usd, 0) +
    (unidentified?.metrics.cost_usd ?? 0) +
    (unattributed?.metrics.cost_usd ?? 0)
  );

  return (
    <Table header={panelHeader}>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Agents Used</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Total Spend</TableHead>
          <TableHead className="text-right">% Total</TableHead>
          <TableHead sortable sortDirection={dir("requests")} onSort={() => handleSort("requests")} className="text-right">Requests</TableHead>
          <TableHead sortable sortDirection={dir("tokens")} onSort={() => handleSort("tokens")} className="text-right">Tokens</TableHead>
          <TableHead sortable sortDirection={dir("last_seen")} onSort={() => handleSort("last_seen")} className="text-right">Last Used</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={7} />)
        ) : totalRows === 0 ? (
          <TableRow>
            <TableCell colSpan={7} className="py-10 text-center text-body-sm text-faint-foreground">
              No activity from people in this period
            </TableCell>
          </TableRow>
        ) : (
          <>
            {rows.map((u) => (
              <TableRow key={u.user_id}>
                <TableCell className="pr-4">
                  {memberIds.has(u.user_id) ? (
                    <UserBadge userId={u.user_id} account={account} linkToProfile />
                  ) : (
                    <SlackUserIdentity uid={u.user_id} teamId={u.slack_team_id} />
                  )}
                </TableCell>
                <MetricsCells row={u} totalCost={denom} deploymentsByAgent={deploymentsByAgent} />
              </TableRow>
            ))}
            {unidentified && <BucketRow variant="unidentified" agg={unidentified} totalCost={denom} deploymentsByAgent={deploymentsByAgent} />}
            {unattributed && <BucketRow variant="unattributed" agg={unattributed} totalCost={denom} deploymentsByAgent={deploymentsByAgent} />}
          </>
        )}
      </TableBody>
    </Table>
  );
}

interface BucketRowProps {
  variant: "unidentified" | "unattributed";
  agg: Aggregate;
  totalCost: number;
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
}

// SlackUserIdentity renders the per-row label for an unlinked Slack user.
//
// When the server attaches a team_id (via the slack_identity_mappings
// directory join — populated by the live-ingest path on /authorize and the
// one-time backfill), the label deep-links into Slack's user-profile UI
// (`slack://user?team=T&id=U`) so an admin can click through and see who
// the human behind the id is. Slack's OS protocol handler routes to the
// desktop app or to the open web tab. Rows without a team_id (directory
// miss — tombstoned user pre-backfill) render as plain text.
function SlackUserIdentity({ uid, teamId }: { uid: string; teamId?: string }) {
  const display = uid;
  const deepLink = teamId ? `slack://user?team=${teamId}&id=${uid}` : undefined;

  const body = (
    <span className="inline-flex items-center gap-2 text-body-sm text-foreground">
      <Slack className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      <span className={cn("truncate", deepLink && "hover:underline")}>
        Slack user - {display}
      </span>
    </span>
  );

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          {deepLink ? (
            <a href={deepLink} rel="noreferrer" className="inline-flex">
              {body}
            </a>
          ) : (
            body
          )}
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-[260px] [text-wrap:initial]">
          {deepLink
            ? "Slack user not linked to an Astro account. Click to open their Slack profile."
            : "Slack user not linked to an Astro account. Connect to attribute their usage to a member."}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function BucketRow({ variant, agg, totalCost, deploymentsByAgent }: BucketRowProps) {
  const isUnidentified = variant === "unidentified";
  const label = isUnidentified
    ? `Unidentified · ${agg.count} ${agg.count === 1 ? "person" : "people"}`
    : "System spend";
  const tooltipText = isUnidentified
    ? "Traces with user identifiers we don't recognize — typically custom agent integrations or direct SDK calls that emit non-WorkOS, non-Slack ids."
    : "Traces not associated with any user — typically background jobs, system tasks, or SDK calls that didn't forward a user identifier.";
  return (
    <TableRow>
      <TableCell className="pr-4">
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex items-center gap-2 text-body-sm text-foreground">
                {isUnidentified ? (
                  <CircleUserRound className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                ) : (
                  <Server className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                )}
                {label}
                <Info className="size-3 text-muted-foreground" aria-hidden />
              </span>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-[260px] [text-wrap:initial]">{tooltipText}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </TableCell>
      <MetricsCells row={agg.metrics} totalCost={totalCost} deploymentsByAgent={deploymentsByAgent} />
    </TableRow>
  );
}
