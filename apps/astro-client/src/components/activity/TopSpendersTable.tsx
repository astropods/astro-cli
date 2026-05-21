import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import { formatTimeAgo } from "@/lib/time-format";
import { Card } from "@/components/ui/card";
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
import { formatModelName } from "./model-colors";
import { AgentsUsedChips } from "./AgentsUsedChips";
import { UserBadge } from "@/components/UserBadge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Info } from "lucide-react";
import { useAccountMembers } from "@/api/queries/accounts";
import { classifyUserId, UNATTRIBUTED_USER_KEY, UNAUTHORIZED_USER_KEY } from "./user-classification";

type AgentRow = AccountBlueprintsSummaryResponse["blueprints"][number];
type UserRow = AccountUsersSummaryResponse["users"][number];

type AgentSortKey = "cost_usd" | "requests" | "cost_per_request" | "tok_per_request" | "p95_latency_ms";
type UserSortKey = "cost_usd" | "requests" | "tokens" | "last_seen";

function SortIcon({ active, asc }: { active: boolean; asc: boolean }) {
  if (!active) return <span className="ml-1 text-faint-foreground opacity-40">↕</span>;
  return <span className="ml-1 text-foreground">{asc ? "↑" : "↓"}</span>;
}

interface SortableHeadProps<K extends string> {
  label: string;
  sortKey?: K;
  currentSort: K;
  asc: boolean;
  onSort: (k: K) => void;
  align?: "left" | "right";
}

function SortableHead<K extends string>({
  label,
  sortKey,
  currentSort,
  asc,
  onSort,
  align = "right",
}: SortableHeadProps<K>) {
  const isActive = sortKey !== undefined && currentSort === sortKey;
  return (
    <TableHead
      className={cn(
        "font-mono text-label uppercase tracking-[0.07em] text-faint-foreground",
        align === "right" ? "text-right" : "text-left",
        sortKey && "cursor-pointer select-none hover:text-foreground transition-colors",
      )}
      onClick={sortKey ? () => onSort(sortKey) : undefined}
    >
      {label}
      {sortKey && <SortIcon active={isActive} asc={asc} />}
    </TableHead>
  );
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
    }
  | {
      mode: "users";
      users: UserRow[];
      account: string;
      loading: boolean;
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
}: Extract<TopSpendersTableProps, { mode: "agents" }>) {
  const { sortKey, asc, handleSort } = useSort<AgentSortKey>("cost_usd");

  const sorted = useMemo(
    () => [...blueprints].sort((a, b) => {
      const diff = a[sortKey] - b[sortKey];
      return asc ? diff : -diff;
    }),
    [blueprints, sortKey, asc],
  );

  const sp = { currentSort: sortKey, asc, onSort: handleSort };

  return (
    <Card className="overflow-hidden dark:bg-surface">
      <div className="px-5 py-4">
        <h3 className="text-heading-4 text-foreground">Top Spenders</h3>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="font-mono text-label uppercase tracking-[0.07em] text-left text-faint-foreground">
              {groupLabel}
            </TableHead>
            <SortableHead label="Requests" sortKey="requests" {...sp} />
            <SortableHead label="Spend" sortKey="cost_usd" {...sp} />
            <SortableHead label="Spend/Req" sortKey="cost_per_request" {...sp} />
            <SortableHead label="Tok/Req" sortKey="tok_per_request" {...sp} />
            <SortableHead label="P95" sortKey="p95_latency_ms" {...sp} />
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={6} />)
          ) : sorted.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="py-10 text-center text-body-sm text-faint-foreground">
                No agent activity in this period
              </TableCell>
            </TableRow>
          ) : (
            sorted.map((b) => (
              <TableRow key={b.agent_name}>
                <TableCell className="pr-4">
                  <span className="text-body font-medium text-foreground">{b.agent_name}</span>
                  {b.top_model && (
                    <span className="ml-2 text-mono-sm text-faint-foreground">{formatModelName(b.top_model)}</span>
                  )}
                </TableCell>
                <TableCell className="text-right font-mono text-body-sm text-muted-foreground">
                  {formatCompact(b.requests)}
                </TableCell>
                <TableCell className="text-right font-mono text-body font-medium text-foreground">
                  {formatCost(b.cost_usd)}
                </TableCell>
                <TableCell className="text-right font-mono text-body-sm text-muted-foreground">
                  {formatCost(b.cost_per_request)}
                </TableCell>
                <TableCell className="text-right font-mono text-body-sm text-muted-foreground">
                  {formatCompact(b.tok_per_request)}
                </TableCell>
                <TableCell className="text-right font-mono text-body-sm text-muted-foreground">
                  {b.p95_latency_ms > 0 ? formatLatency(b.p95_latency_ms) : "—"}
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </Card>
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

function MetricsCells({ row }: { row: UserMetrics }) {
  return (
    <>
      <TableCell><AgentsUsedChips agents={row.agents_used} /></TableCell>
      <TableCell className="text-right font-mono text-body font-medium text-foreground">{formatCost(row.cost_usd)}</TableCell>
      <TableCell className="text-right font-mono text-body-sm text-muted-foreground">{formatCompact(row.requests)}</TableCell>
      <TableCell className="text-right font-mono text-body-sm text-muted-foreground">{formatCompact(row.tokens)}</TableCell>
      <TableCell className="text-right font-mono text-body-sm text-muted-foreground">{formatLastSeen(row.last_seen)}</TableCell>
    </>
  );
}

function UsersTopSpenders({
  users,
  account,
  loading,
}: Extract<TopSpendersTableProps, { mode: "users" }>) {
  const { sortKey, asc, handleSort } = useSort<UserSortKey>("cost_usd");

  // Members feeds classification — without it every named user falls into
  // Unauthorized. Treat its load as part of the table's loading state so the
  // skeleton stays up until classification can be trusted.
  const { data: membersData, isLoading: membersLoading } = useAccountMembers(account);
  const memberIds = useMemo(
    () => new Set(membersData?.members.map((m) => m.user_id) ?? []),
    [membersData],
  );
  const isLoading = loading || membersLoading;

  const { named, unauthorized, unattributed } = useMemo(() => {
    const namedRows: UserRow[] = [];
    const unauthorizedRows: UserRow[] = [];
    const unattributedRows: UserRow[] = [];
    for (const u of users) {
      const bucket = classifyUserId(u.user_id, memberIds);
      if (bucket === UNATTRIBUTED_USER_KEY) unattributedRows.push(u);
      else if (bucket === UNAUTHORIZED_USER_KEY) unauthorizedRows.push(u);
      else namedRows.push(u);
    }
    namedRows.sort((a, b) => {
      const diff = userSortValue(a, sortKey) - userSortValue(b, sortKey);
      return asc ? diff : -diff;
    });
    return {
      named: namedRows,
      unauthorized: aggregateUsers(unauthorizedRows),
      unattributed: aggregateUsers(unattributedRows),
    };
  }, [users, memberIds, sortKey, asc]);

  const sp = { currentSort: sortKey, asc, onSort: handleSort };
  const totalRows = named.length + (unauthorized ? 1 : 0) + (unattributed ? 1 : 0);

  return (
    <Card className="overflow-hidden dark:bg-surface">
      <div className="px-5 py-4">
        <h3 className="text-heading-4 text-foreground">Top Spenders</h3>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="font-mono text-label uppercase tracking-[0.07em] text-left text-faint-foreground">
              User
            </TableHead>
            <TableHead className="font-mono text-label uppercase tracking-[0.07em] text-left text-faint-foreground">
              Agents Used
            </TableHead>
            <SortableHead label="Spend" sortKey="cost_usd" {...sp} />
            <SortableHead label="Requests" sortKey="requests" {...sp} />
            <SortableHead label="Tokens" sortKey="tokens" {...sp} />
            <SortableHead label="Last Used" sortKey="last_seen" {...sp} />
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={6} />)
          ) : totalRows === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="py-10 text-center text-body-sm text-faint-foreground">
                No user activity in this period
              </TableCell>
            </TableRow>
          ) : (
            <>
              {named.map((u) => (
                <TableRow key={u.user_id}>
                  <TableCell className="pr-4">
                    <UserBadge userId={u.user_id} account={account} />
                  </TableCell>
                  <MetricsCells row={u} />
                </TableRow>
              ))}
              {unauthorized && <BucketRow variant="unauthorized" agg={unauthorized} />}
              {unattributed && <BucketRow variant="unattributed" agg={unattributed} />}
            </>
          )}
        </TableBody>
      </Table>
    </Card>
  );
}

interface BucketRowProps {
  variant: "unauthorized" | "unattributed";
  agg: Aggregate;
}

function BucketRow({ variant, agg }: BucketRowProps) {
  const isUnauthorized = variant === "unauthorized";
  const label = isUnauthorized
    ? `Unauthorized${agg.count > 1 ? ` · ${agg.count} users` : ""}`
    : "Unattributed";
  const tooltip = isUnauthorized
    ? "Traces from users who reached an agent through an enabled adapter (Slack, Discord, etc.) but haven't authorized it."
    : "Traces not associated with any user — typically background jobs, system tasks, or SDK calls that didn't forward a user identifier.";
  const dotClass = isUnauthorized
    ? "size-5 rounded-full border border-[var(--warning)] bg-[color-mix(in_srgb,var(--warning)_20%,transparent)]"
    : "size-5 rounded-full border border-dashed border-border";

  return (
    <TableRow>
      <TableCell className="pr-4">
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex items-center gap-2 font-mono text-mono-sm text-faint-foreground cursor-help">
                <span className={dotClass} aria-hidden />
                {label}
                <Info className="size-3 text-faint-foreground" aria-hidden />
              </span>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-xs">{tooltip}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </TableCell>
      <MetricsCells row={agg.metrics} />
    </TableRow>
  );
}
