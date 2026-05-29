import { useMemo, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";
import { formatTimeAgo } from "@/lib/time-format";
import { AgentNameLink, type AgentDeploymentRef } from "./AgentNameLink";
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
  AccountBlueprintsSummaryResponse,
  AccountUsersSummaryResponse,
} from "@/lib/api";
import { AgentsUsedChips } from "./AgentsUsedChips";
import { UsersUsedAvatars } from "./UsersUsedAvatars";
import { UserBadge } from "@/components/UserBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Info, CircleUserRound, Server, TriangleAlert } from "lucide-react";
import { useAccountMembers } from "@/api/queries/accounts";
import { classifyUserId, UNATTRIBUTED_USER_KEY, UNIDENTIFIED_USER_KEY } from "./user-classification";

type AgentRow = AccountBlueprintsSummaryResponse["blueprints"][number];
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
      blueprints: AgentRow[];
      loading: boolean;
      groupLabel?: string;
      /** Account name used to render the agent avatar + linkify the row. When
       *  omitted (e.g. Models view), the cell renders text-only. */
      account?: string;
      /** Optional map of agent_name → all matching deployments. Behavior:
       *   - 0 entries: row links to the blueprint detail page
       *   - 1 entry:   row links straight to that deployment's Monitor tab
       *   - 2+ entries: row opens a popover so the user picks a deployment
       *  Lets the table cover the 1-to-many agent_name → deployment case
       *  without rendering duplicate rows for multi-region deployments. */
      deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
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
      /** Same agent_name → deployments map as the agents-mode prop. Forwarded
       *  to `AgentsUsedChips` so each chip / popover row routes to a
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
  return <AgentsTopSpenders {...props} />;
}

// ── Agents mode ──────────────────────────────────────────────────────────────

function AgentsTopSpenders({
  blueprints,
  loading,
  groupLabel = "Agent",
  account,
  deploymentsByAgent,
  totalCost,
  panelHeader,
}: Extract<TopSpendersTableProps, { mode: "agents" }>) {
  const { sortKey, asc, handleSort } = useSort<AgentSortKey>("cost_usd");

  const sorted = useMemo(
    () => [...blueprints].sort((a, b) => {
      const diff = a[sortKey] - b[sortKey];
      return asc ? diff : -diff;
    }),
    [blueprints, sortKey, asc],
  );

  // Fall back to a self-computed sum when the caller hasn't wired a stable
  // denominator (e.g. the Models view) — percentages still render, they just
  // re-base if rows get filtered.
  const denom = totalCost ?? blueprints.reduce((s, b) => s + b.cost_usd, 0);

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
              No agent activity in this period
            </TableCell>
          </TableRow>
        ) : (
          sorted.map((b) => {
            // Zero requests in the period means no traces ever landed for
            // this agent — usually the OTel exporter isn't wired up. Flag
            // with a small warning icon so the all-zero row reads as
            // "not instrumented" rather than "agent did nothing".
            const notInstrumented = b.requests === 0;
            const nameNode = (
              <span className="inline-flex min-w-0 items-center gap-2">
                {account && (
                  <BlueprintIdentity
                    account={account}
                    name={b.agent_name}
                    size={20}
                    className="size-5 shrink-0 rounded-full"
                  />
                )}
                <span className="truncate font-medium text-foreground">{b.agent_name}</span>
              </span>
            );
            return (
              <TableRow key={b.agent_name}>
                <TableCell className="pr-4">
                  <span className="inline-flex items-center gap-1.5">
                    {account ? (
                      <AgentNameLink
                        account={account}
                        agentName={b.agent_name}
                        deployments={deploymentsByAgent?.get(b.agent_name) ?? []}
                      >
                        {nameNode}
                      </AgentNameLink>
                    ) : (
                      nameNode
                    )}
                    {notInstrumented && (
                      <TooltipProvider delayDuration={200}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span
                              className="cursor-help text-faint-foreground"
                              aria-label="Not instrumented"
                            >
                              <TriangleAlert className="size-3.5" />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent side="top" className="max-w-xs">
                            Instrumentation not available — no requests, spend, or token data
                            available.
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    )}
                  </span>
                </TableCell>
                <TableCell>
                  <UsersUsedAvatars userIds={b.users_used ?? []} account={account ?? ""} />
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {formatCompact(b.requests)}
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {formatCost(b.cost_usd)}
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {formatShare(b.cost_usd, denom)}
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {formatCost(b.cost_per_request)}
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {formatCompact(b.tok_per_request)}
                </TableCell>
                <TableCell className="text-right text-foreground">
                  {b.p95_latency_ms > 0 ? formatLatency(b.p95_latency_ms) : "—"}
                </TableCell>
              </TableRow>
            );
          })
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
  // Dedupe by `account/name` so two different publishers of the same agent
  // name don't collapse to a single chip.
  const agents = new Map<string, AgentRef>();
  let cost = 0, reqs = 0, tokens = 0;
  for (const r of rows) {
    cost += r.cost_usd;
    reqs += r.requests;
    tokens += r.tokens;
    if (r.last_seen && (!lastSeen || r.last_seen > lastSeen)) lastSeen = r.last_seen;
    for (const a of r.agents_used) agents.set(`${a.account}/${a.name}`, a);
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

  const { named, unidentified, unattributed } = useMemo(() => {
    const namedRows: UserRow[] = [];
    const unidentifiedRows: UserRow[] = [];
    const unattributedRows: UserRow[] = [];
    for (const u of users) {
      const bucket = classifyUserId(u.user_id, memberIds);
      if (bucket === UNATTRIBUTED_USER_KEY) unattributedRows.push(u);
      else if (bucket === UNIDENTIFIED_USER_KEY) unidentifiedRows.push(u);
      else namedRows.push(u);
    }
    namedRows.sort((a, b) => {
      const diff = userSortValue(a, sortKey) - userSortValue(b, sortKey);
      return asc ? diff : -diff;
    });
    return {
      named: namedRows,
      unidentified: aggregateUsers(unidentifiedRows),
      unattributed: aggregateUsers(unattributedRows),
    };
  }, [users, memberIds, sortKey, asc]);

  const dir = (col: UserSortKey) => sortDirFor(sortKey, asc, col);
  const totalRows = named.length + (unidentified ? 1 : 0) + (unattributed ? 1 : 0);
  // Fall back to a self-computed sum when the caller hasn't wired a stable
  // denominator. The fallback derives from the filtered rows + buckets, so
  // search interactions will re-base the percentages — same caveat as the
  // agents-mode branch.
  const denom = totalCost ?? (
    named.reduce((s, u) => s + u.cost_usd, 0) +
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
            {named.map((u) => (
              <TableRow key={u.user_id}>
                <TableCell className="pr-4">
                  <UserBadge userId={u.user_id} account={account} linkToProfile />
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

function BucketRow({ variant, agg, totalCost, deploymentsByAgent }: BucketRowProps) {
  const isUnidentified = variant === "unidentified";
  const label = isUnidentified
    ? `Unidentified · ${agg.count} ${agg.count === 1 ? "person" : "people"}`
    : "System spend";
  const tooltipText = isUnidentified
    ? "Traces from people who reached an agent through an enabled adapter (Slack, etc.) but aren't linked to a member of this organization."
    : "Traces not associated with any user — typically background jobs, system tasks, or SDK calls that didn't forward a user identifier.";
  return (
    <TableRow>
      <TableCell className="pr-4">
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex items-center gap-2 text-body-sm text-faint-foreground cursor-help">
                {isUnidentified ? (
                  <CircleUserRound className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                ) : (
                  <Server className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                )}
                {label}
                <Info className="size-3 text-faint-foreground" aria-hidden />
              </span>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-[260px]">{tooltipText}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </TableCell>
      <MetricsCells row={agg.metrics} totalCost={totalCost} deploymentsByAgent={deploymentsByAgent} />
    </TableRow>
  );
}
