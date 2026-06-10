import { useEffect, useMemo, useState, type ReactNode } from "react";
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
  TableShowMore,
} from "@/components/ui/table";
import { formatCost, formatCompact, formatLatency } from "@/lib/format-utils";
import type {
  AccountDeploymentsSummaryResponse,
  AccountUsersSummaryResponse,
} from "@/lib/api";
import { AgentsUsedChips } from "./AgentsUsedChips";
import { UsersUsedAvatars } from "./UsersUsedAvatars";
import { SlackUserIdentity } from "./SlackUserIdentity";
import { insightsUserIdentityKey } from "./insights-user-identity";
import { UserBadge } from "@/components/UserBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Info, CircleUserRound, Server, TriangleAlert } from "lucide-react";
import { useAccountMembers } from "@/api/queries/accounts";
import { isSlackUserId } from "./user-classification";

type DeploymentRow = AccountDeploymentsSummaryResponse["deployments"][number];
type UserRow = AccountUsersSummaryResponse["users"][number];

type AgentSortKey = "cost_usd" | "requests" | "cost_per_request" | "tok_per_request" | "p95_latency_ms";
type UserSortKey = "cost_usd" | "requests" | "tokens" | "last_seen";

// Collapse long lists so the Insights page fits without an outer scrollbar,
// with progressive "Show top 10" / "Show all" controls. Agents and People
// views share the same cap so the affordance reads consistently. Tighter
// than the Monitor traces table (which uses 10) — that page is just the
// trace list; this one stacks stat cards, charts, and the table above the fold.
const DEFAULT_VISIBLE_ROWS = 5;
const TOP_VISIBLE_ROWS = 10;

function useProgressiveRows(totalRows: number, resetSignal: unknown) {
  const [visibleCount, setVisibleCount] = useState(DEFAULT_VISIBLE_ROWS);

  useEffect(() => {
    setVisibleCount(DEFAULT_VISIBLE_ROWS);
  }, [resetSignal]);

  const cappedVisibleCount = Math.min(visibleCount, totalRows);
  const hiddenCount = Math.max(totalRows - cappedVisibleCount, 0);
  const revealedCount = Math.max(cappedVisibleCount - DEFAULT_VISIBLE_ROWS, 0);
  const showMoreLabel =
    cappedVisibleCount < TOP_VISIBLE_ROWS && totalRows > TOP_VISIBLE_ROWS
      ? `Show top ${TOP_VISIBLE_ROWS}`
      : "Show all";

  return {
    visibleCount: cappedVisibleCount,
    hiddenCount,
    revealedCount,
    showMoreLabel,
    showMore: () =>
      setVisibleCount((count) =>
        count < TOP_VISIBLE_ROWS ? Math.min(totalRows, TOP_VISIBLE_ROWS) : totalRows,
      ),
    showLess: () => setVisibleCount(DEFAULT_VISIBLE_ROWS),
  };
}

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

function RankMarker({ rank }: { rank: number }) {
  return (
    <span className="w-5 shrink-0 text-right text-mono-sm tabular-nums text-faint-foreground">
      {rank}
    </span>
  );
}

function IdentityTableCell({
  rank,
  children,
}: {
  rank: number;
  children: ReactNode;
}) {
  return (
    <TableCell className="pl-3 pr-4">
      <div className="flex min-w-0 items-center gap-2.5">
        <RankMarker rank={rank} />
        <div className="min-w-0 flex-1">
          {children}
        </div>
      </div>
    </TableCell>
  );
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

function renderAgentRowContent(
  b: DeploymentRow,
  rank: number,
  ctx: { denom: number; account?: string },
) {
  // Zero requests in the period means no traces ever landed for this
  // deployment — usually the agent isn't sending observability data. Flag
  // with a small warning icon so the all-zero row reads as "not instrumented"
  // rather than "deployment did nothing".
  const notInstrumented = b.requests === 0;
  const label = b.display_name || b.agent_name;
  const identityRow = (
    <span className="inline-flex min-w-0 items-center gap-2">
      {ctx.account && (
        <BlueprintIdentity
          account={ctx.account}
          name={b.agent_name}
          size={20}
          className="size-5 shrink-0 rounded-full"
        />
      )}
      <span className="min-w-0 truncate text-foreground">{label}</span>
    </span>
  );
  return (
    <>
      <IdentityTableCell rank={rank}>
        <span className="inline-flex items-center gap-1.5">
          {ctx.account ? (
            <Link
              to={`/${ctx.account}/agents/${b.deployment_id}/monitor`}
              className="inline-flex items-center hover:underline"
            >
              {identityRow}
            </Link>
          ) : (
            identityRow
          )}
          {notInstrumented && (
            <TooltipProvider delayDuration={200}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="text-faint-foreground" aria-label="Not instrumented">
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
      </IdentityTableCell>
      <TableCell>
        <UsersUsedAvatars
          userIds={b.users_used ?? []}
          users={b.users_used_details}
          account={ctx.account ?? ""}
        />
      </TableCell>
      <TableCell className="text-right text-foreground">{formatCompact(b.requests)}</TableCell>
      <TableCell className="text-right text-foreground">{formatCost(b.cost_usd)}</TableCell>
      <TableCell className="text-right text-foreground">{formatShare(b.cost_usd, ctx.denom)}</TableCell>
      <TableCell className="text-right text-foreground">{formatCost(b.cost_per_request)}</TableCell>
      <TableCell className="text-right text-foreground">{formatCompact(b.tok_per_request)}</TableCell>
      <TableCell className="text-right text-foreground">
        {b.p95_latency_ms > 0 ? formatLatency(b.p95_latency_ms) : "—"}
      </TableCell>
    </>
  );
}

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

  const { visibleCount, hiddenCount, revealedCount, showMoreLabel, showMore, showLess } =
    useProgressiveRows(sorted.length, sorted);
  const visibleSorted = sorted.slice(0, visibleCount);

  return (
    <Table
      header={panelHeader}
      containerClassName="bg-card dark:bg-surface"
      footer={
        hiddenCount > 0 || revealedCount > 0 ? (
          <TableShowMore
            hiddenCount={hiddenCount}
            expanded={revealedCount > 0 && hiddenCount === 0}
            onToggle={showMore}
            showMoreLabel={showMoreLabel}
            revealedCount={revealedCount}
            onShowLess={showLess}
          />
        ) : undefined
      }
    >
      <TableHeader>
        <TableRow>
          <TableHead className="pl-3">{groupLabel}</TableHead>
          <TableHead>Used by</TableHead>
          <TableHead sortable sortDirection={dir("requests")} onSort={() => handleSort("requests")} className="text-right">Requests</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Spend</TableHead>
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
          <>
            {visibleSorted.map((b, i) => (
              <TableRow key={b.deployment_id}>{renderAgentRowContent(b, i + 1, { denom, account })}</TableRow>
            ))}
          </>
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

type UserDisplayItem =
  | { kind: "real"; row: UserRow }
  | { kind: "unidentified"; row: UserRow }
  | { kind: "system"; agg: Aggregate };

function userItemKey(d: UserDisplayItem): string {
  if (d.kind === "system") return "__system_spend__";
  // Composite key — guards against a theoretical collision where the same
  // user_id ends up in both buckets (e.g. classification change mid-render).
  return `${d.kind}:${insightsUserIdentityKey(d.row)}`;
}

function renderUserRowContent(
  d: UserDisplayItem,
  rank: number,
  ctx: {
    denom: number;
    memberIds: Set<string>;
    account: string;
    deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
  },
) {
  if (d.kind === "unidentified") {
    return (
      <UnidentifiedUserCells
        rank={rank}
        user={d.row}
        totalCost={ctx.denom}
        deploymentsByAgent={ctx.deploymentsByAgent}
      />
    );
  }
  if (d.kind === "system") {
    return (
      <SystemSpendCells
        rank={rank}
        agg={d.agg}
        totalCost={ctx.denom}
        deploymentsByAgent={ctx.deploymentsByAgent}
      />
    );
  }
  const u = d.row;
  return (
    <>
      <IdentityTableCell rank={rank}>
        {ctx.memberIds.has(u.user_id) ? (
          <UserBadge userId={u.user_id} account={ctx.account} linkToProfile />
        ) : (
          <SlackUserIdentity user={u} />
        )}
      </IdentityTableCell>
      <MetricsCells row={u} totalCost={ctx.denom} deploymentsByAgent={ctx.deploymentsByAgent} />
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
  //     trace user_ids agents may emit). Rendered per-row with a soft
  //     circle + mono user_id.
  //   - unattributed: empty user_id. Stays aggregated (system spend).
  // Bucketize only — the displayItems memo below merges real + unidentified
  // and applies the active sort key. Doing the sort there avoids duplicating
  // it per bucket here.
  const { rows, unidentified, unattributed } = useMemo(() => {
    const realRows: UserRow[] = [];
    const unidentifiedRows: UserRow[] = [];
    const unattributedRows: UserRow[] = [];
    for (const u of users) {
      if (!u.user_id) unattributedRows.push(u);
      else if (memberIds.has(u.user_id) || isSlackUserId(u.user_id)) realRows.push(u);
      else unidentifiedRows.push(u);
    }
    return {
      rows: realRows,
      unidentified: unidentifiedRows,
      unattributed: aggregateUsers(unattributedRows),
    };
  }, [users, memberIds]);

  const dir = (col: UserSortKey) => sortDirFor(sortKey, asc, col);
  const totalRows = rows.length + unidentified.length + (unattributed ? 1 : 0);
  // Fall back to a self-computed sum when the caller hasn't wired a stable
  // denominator. The fallback derives from the filtered rows + buckets, so
  // search interactions will re-base the percentages — same caveat as the
  // agents-mode branch.
  const denom = totalCost ?? (
    rows.reduce((s, u) => s + u.cost_usd, 0) +
    unidentified.reduce((s, u) => s + u.cost_usd, 0) +
    (unattributed?.metrics.cost_usd ?? 0)
  );

  // All user rows compete on the same sort — cost (or whichever key is
  // active) wins regardless of identification, so a high-spend unidentified
  // user_id can rank above named members. System spend is the only kind
  // pinned last; it's an aggregate, not a user, so it sits outside the
  // ranking competition. Slice top 5 across the merged list, Show-more
  // reveals in predictable steps without per-row animation, which keeps
  // large workspaces responsive.
  const displayItems: UserDisplayItem[] = useMemo(() => {
    type UserItem = { kind: "real" | "unidentified"; row: UserRow };
    const userItems: UserItem[] = [
      ...rows.map((row) => ({ kind: "real" as const, row })),
      ...unidentified.map((row) => ({ kind: "unidentified" as const, row })),
    ];
    userItems.sort((a, b) => {
      const diff = userSortValue(a.row, sortKey) - userSortValue(b.row, sortKey);
      return asc ? diff : -diff;
    });
    return unattributed
      ? [...userItems, { kind: "system" as const, agg: unattributed }]
      : userItems;
  }, [rows, unidentified, unattributed, sortKey, asc]);
  const { visibleCount, hiddenCount, revealedCount, showMoreLabel, showMore, showLess } =
    useProgressiveRows(displayItems.length, displayItems);
  const visibleDisplay = displayItems.slice(0, visibleCount);

  return (
    <Table
      header={panelHeader}
      containerClassName="bg-card dark:bg-surface"
      footer={
        hiddenCount > 0 || revealedCount > 0 ? (
          <TableShowMore
            hiddenCount={hiddenCount}
            expanded={revealedCount > 0 && hiddenCount === 0}
            onToggle={showMore}
            showMoreLabel={showMoreLabel}
            revealedCount={revealedCount}
            onShowLess={showLess}
          />
        ) : undefined
      }
    >
      <TableHeader>
        <TableRow>
          <TableHead className="pl-3">Name</TableHead>
          <TableHead>Agents Used</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Spend</TableHead>
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
            {visibleDisplay.map((d, i) => (
              <TableRow key={userItemKey(d)}>
                {renderUserRowContent(d, i + 1, {
                  denom,
                  memberIds,
                  account,
                  deploymentsByAgent,
                })}
              </TableRow>
            ))}
          </>
        )}
      </TableBody>
    </Table>
  );
}

function UnidentifiedUserCells({
  rank,
  user,
  totalCost,
  deploymentsByAgent,
}: {
  rank: number;
  user: UserRow;
  totalCost: number;
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
}) {
  return (
    <>
      <IdentityTableCell rank={rank}>
        <div className="flex min-w-0 items-center gap-2">
          <span
            className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
            aria-hidden
          >
            <CircleUserRound className="size-3" strokeWidth={1.75} />
          </span>
          <span
            className="truncate text-mono-sm text-foreground"
            title={user.user_id}
          >
            {user.user_id}
          </span>
        </div>
      </IdentityTableCell>
      <MetricsCells row={user} totalCost={totalCost} deploymentsByAgent={deploymentsByAgent} />
    </>
  );
}

function SystemSpendCells({
  rank,
  agg,
  totalCost,
  deploymentsByAgent,
}: {
  rank: number;
  agg: Aggregate;
  totalCost: number;
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
}) {
  return (
    <>
      <IdentityTableCell rank={rank}>
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex items-center gap-2 text-body-sm text-foreground">
                <span
                  className="flex size-5 shrink-0 items-center justify-center text-muted-foreground"
                  aria-hidden
                >
                  <Server className="size-3" strokeWidth={1.75} />
                </span>
                System spend
                <Info className="size-3 text-muted-foreground" aria-hidden />
              </span>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-[260px] [text-wrap:initial]">
              Traces not associated with any user — typically background jobs, system tasks, or SDK calls that didn't forward a user identifier.
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </IdentityTableCell>
      <MetricsCells row={agg.metrics} totalCost={totalCost} deploymentsByAgent={deploymentsByAgent} />
    </>
  );
}
