import { forwardRef, useEffect, useMemo, useState, type ComponentPropsWithoutRef, type ReactNode, type Ref } from "react";
import { Link } from "react-router";
import { Info, Server, TriangleAlert, User } from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { UserAvatar } from "@/components/UserAvatar";
import { InlineBadge } from "@/components/InlineBadge";
import { SlackIcon } from "@/components/ui/svgs/slackIcon";
import { nonHumanLabel } from "@/lib/identity-kind";
import { getIntegrationIconUrl } from "@/lib/assets";
import { useResolvedTheme } from "@/lib/theme";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableShowMore,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { formatTimeAgo } from "@/lib/time-format";
import { formatCompact, formatCost, formatLatency } from "@/lib/format-utils";
import type {
  InsightsAgentChip,
  InsightsAgentRow,
  InsightsIdentityRef,
  InsightsPersonRow,
} from "@/lib/api";
import { OverflowPopover } from "./OverflowPopover";
import { useAccountObservabilitySummary } from "@/api/queries/observability";

export type AgentSortKey = "cost_usd" | "requests" | "cost_per_request" | "tok_per_request" | "p95_latency_ms";
export type UserSortKey = "cost_usd" | "requests" | "tokens" | "last_seen";
export type TopSpendersSortDirection = "asc" | "desc";

const DEFAULT_VISIBLE_ROWS = 5;
const PAGE_SIZE_ROWS = 10;
const MAX_VISIBLE_IDENTITIES = 3;
const MAX_VISIBLE_AGENTS = 3;

type TopSpendersPagination = {
  totalRows: number;
  defaultVisibleRows?: number;
  pageSize?: number;
  showLessLabel?: string;
  onShowMore: () => void;
  onShowLess: () => void;
};

type TopSpendersTableProps =
  | {
      mode: "agents";
      rows: InsightsAgentRow[];
      loading: boolean;
      groupLabel?: string;
      panelHeader?: ReactNode;
      sortKey?: AgentSortKey;
      sortDirection?: TopSpendersSortDirection;
      onSort?: (key: AgentSortKey) => void;
      pagination?: TopSpendersPagination;
    }
  | {
      mode: "users";
      rows: InsightsPersonRow[];
      loading: boolean;
      panelHeader?: ReactNode;
      sortKey?: UserSortKey;
      sortDirection?: TopSpendersSortDirection;
      onSort?: (key: UserSortKey) => void;
      pagination?: TopSpendersPagination;
    }
  | {
      // The "models" view self-fetches the account observability summary rather
      // than taking rows, since its data comes from a different endpoint than
      // the agents/people tables.
      mode: "models";
      account: string;
      days: number;
      panelHeader?: ReactNode;
    };

export function TopSpendersTable(props: TopSpendersTableProps) {
  if (props.mode === "users") {
    return <UsersTopSpenders {...props} />;
  }
  if (props.mode === "models") {
    return <ModelsTopSpenders {...props} />;
  }
  return <AgentsTopSpenders {...props} />;
}

function useSort<K extends string>(initial: K) {
  const [sortKey, setSortKey] = useState<K>(initial);
  const [asc, setAsc] = useState(false);

  function handleSort(key: K) {
    if (key === sortKey) {
      setAsc((value) => !value);
      return;
    }
    setSortKey(key);
    setAsc(false);
  }

  return { sortKey, asc, handleSort };
}

function sortDirFor<K extends string>(active: K, asc: boolean, col: K) {
  return active === col ? (asc ? "asc" : "desc") : undefined;
}

// The observability summary window as from/to ISO params for the models view.
function rangeParams(days: number): Record<string, string> {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - days);
  return { from: start.toISOString(), to: end.toISOString() };
}

// Model ids often carry a date/version suffix (e.g. "claude-3-5-sonnet-20241022",
// "gpt-4o-2024-08-06"); show the clean family name, full id in the tooltip.
function modelDisplayName(model: string): string {
  return model.replace(/-(\d{8}|\d{4}-\d{2}-\d{2})$/, "");
}

function RankMarker({ rank }: { rank: number }) {
  return (
    <span className="w-5 shrink-0 text-mono-sm tabular-nums text-faint-foreground" aria-label={`Rank ${rank}`}>
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
        <TableCell key={i} className={i === 0 ? "pl-3 pr-4" : ""}>
          <div className={cn("h-3.5 animate-pulse rounded bg-muted", i === 0 ? "w-[70%]" : "w-1/2")} />
        </TableCell>
      ))}
    </TableRow>
  );
}

// Shared table shell for every Insights view (agents / people / models) so the
// card chrome, header bar, and body wrapper are defined once.
function SpendersTableShell({
  panelHeader,
  head,
  footer,
  children,
}: {
  panelHeader?: ReactNode;
  head: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Table header={panelHeader} containerClassName="bg-card" footer={footer}>
      <TableHeader>
        <TableRow>{head}</TableRow>
      </TableHeader>
      <TableBody>{children}</TableBody>
    </Table>
  );
}

function formatShare(percent: number): string {
  return percent > 0 ? `${percent.toFixed(1)}%` : "-";
}

function formatLastSeen(ts: string | undefined): string {
  if (!ts) return "-";
  return formatTimeAgo(ts);
}

function identityKey(identity: InsightsIdentityRef, index = 0) {
  return `${identity.kind}:${identity.identity_key ?? identity.user_id ?? identity.id ?? identity.label}:${index}`;
}

function maybeWithTooltip(
  children: ReactNode,
  identity: Pick<InsightsIdentityRef, "label" | "tooltip">,
  side: "top" | "right" = "top",
) {
  if (!identity.tooltip) return children;
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
        <TooltipContent side={side} className="max-w-[260px] [text-wrap:initial]">
          {identity.tooltip}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function identityNameTooltip(identity: InsightsIdentityRef): Pick<InsightsIdentityRef, "label" | "tooltip"> {
  return {
    label: identity.label,
    tooltip: identity.label,
  };
}

function IdentityAvatar({
  identity,
  className,
  size = 24,
}: {
  identity: InsightsIdentityRef;
  className?: string;
  size?: number;
}) {
  const baseClassName = cn("size-6 shrink-0 rounded-full", className);
  const resolvedTheme = useResolvedTheme();

  // Dev-tool source rows carry an integration-icon key (e.g. "anthropic",
  // "openai") that resolves to a themed brand logo.
  if (identity.icon) {
    return (
      <img
        src={getIntegrationIconUrl(identity.icon, resolvedTheme)}
        alt=""
        className={cn(baseClassName, "object-contain")}
        loading="lazy"
      />
    );
  }

  if (identity.kind === "agent" && identity.avatar_account && identity.avatar_name) {
    return (
      <BlueprintIdentity
        account={identity.avatar_account}
        name={identity.avatar_name}
        size={size}
        className={baseClassName}
      />
    );
  }

  if (identity.kind === "member" && identity.avatar_handle) {
    return (
      <UserAvatar
        handle={identity.avatar_handle}
        name={identity.label}
        className={baseClassName}
      />
    );
  }

  if (identity.kind === "slack") {
    if (identity.user_details?.avatar_url) {
      return (
        <img
          src={identity.user_details.avatar_url}
          alt=""
          className={cn(baseClassName, "bg-muted object-cover")}
          referrerPolicy="no-referrer"
        />
      );
    }

    return (
      <span
        className={cn(baseClassName, "flex items-center justify-center bg-muted text-muted-foreground")}
        aria-hidden
      >
        {identity.user_details?.is_bot ? (
          <SlackIcon className="size-3" />
        ) : (
          <User className="size-3.5" strokeWidth={1.75} />
        )}
      </span>
    );
  }

  if (identity.kind === "system") {
    return (
      <span
        className={cn(baseClassName, "flex items-center justify-center text-muted-foreground")}
        aria-hidden
      >
        <Server className="size-3.5" strokeWidth={1.75} />
      </span>
    );
  }

  return (
    <span
      className={cn(baseClassName, "flex items-center justify-center bg-muted text-muted-foreground")}
      aria-hidden
    >
      <User className="size-3.5" strokeWidth={1.75} />
    </span>
  );
}

function IdentityLabel({
  identity,
  className,
  icon,
}: {
  identity: InsightsIdentityRef;
  className?: string;
  icon?: ReactNode;
}) {
  const body = (
    <>
      {icon}
      <span className="min-w-0 truncate">{identity.label}</span>
      {identity.kind === "system" && <Info className="size-3 text-muted-foreground" aria-hidden />}
    </>
  );
  const bodyClassName = cn(
    "inline-flex min-w-0 items-center gap-2 text-body text-foreground",
    identity.kind === "unidentified" && "font-mono",
    className,
  );

  let node: ReactNode;
  if (identity.href?.startsWith("/")) {
    node = (
      <Link to={identity.href} className={cn(bodyClassName, "hover:underline")}>
        {body}
      </Link>
    );
  } else if (identity.href) {
    node = (
      <a href={identity.href} rel="noreferrer" className={cn(bodyClassName, "hover:underline")}>
        {body}
      </a>
    );
  } else {
    node = <span className={bodyClassName}>{body}</span>;
  }

  return maybeWithTooltip(node, identity, "right");
}

function IdentityRow({ identity }: { identity: InsightsIdentityRef }) {
  const badge = nonHumanLabel(identity);
  return (
    <div className="flex min-w-0 items-center gap-2">
      <IdentityAvatar identity={identity} className="size-5" size={20} />
      <IdentityLabel identity={identity} />
      {badge && (
        <InlineBadge variant="soft" className="shrink-0 bg-muted text-muted-foreground">
          {badge}
        </InlineBadge>
      )}
    </div>
  );
}

function AgentRowIdentity({ identity }: { identity: InsightsIdentityRef }) {
  return (
    <IdentityLabel
      identity={identity}
      icon={<IdentityAvatar identity={identity} className="size-5" size={20} />}
    />
  );
}

function IdentityAvatarStack({
  identities,
}: {
  identities: InsightsIdentityRef[];
}) {
  if (identities.length === 0) {
    return <span className="text-muted-foreground">-</span>;
  }

  const visible = identities.slice(0, MAX_VISIBLE_IDENTITIES);
  const overflow = identities.length - visible.length;

  return (
    <div className="flex items-center gap-1.5">
      {visible.map((identity, index) => (
        <span key={identityKey(identity, index)}>
          {maybeWithTooltip(
            <span className="inline-flex" aria-label={identity.label}>
              <IdentityAvatar identity={identity} className="size-6" size={24} />
            </span>,
            identityNameTooltip(identity),
          )}
        </span>
      ))}
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={identities.length}
          itemNoun={{ singular: "person", plural: "people" }}
        >
          <ul className="min-h-0 overflow-y-auto">
            {identities.map((identity, index) => (
              <li key={identityKey(identity, index)} className="rounded px-2 py-1.5">
                <IdentityRow identity={identity} />
              </li>
            ))}
          </ul>
        </OverflowPopover>
      )}
    </div>
  );
}

function AgentChipAvatar({ agent }: { agent: InsightsAgentChip }) {
  const resolvedTheme = useResolvedTheme();
  // Dev-tool chips carry an integration-icon key → themed logo (no agent avatar).
  if (agent.icon) {
    return (
      <img
        src={getIntegrationIconUrl(agent.icon, resolvedTheme)}
        alt=""
        className="size-6 shrink-0 rounded-[3px] border-[0.5px] border-border bg-card object-contain p-0.5"
        loading="lazy"
      />
    );
  }
  return (
    <BlueprintIdentity
      account={agent.avatar_account}
      name={agent.avatar_name}
      size={24}
      className="size-6 shrink-0 rounded-[3px] border-[0.5px] border-border object-cover"
    />
  );
}

// Agent chips link to the agent's page; dev-tool chips have no page (empty href).
// forwardRef so TooltipTrigger asChild can attach its ref to the DOM node.
const ChipShell = forwardRef<
  HTMLElement,
  { agent: InsightsAgentChip } & ComponentPropsWithoutRef<"span">
>(function ChipShell({ agent, children, ...rest }, ref) {
  if (agent.href) {
    return (
      <Link ref={ref as Ref<HTMLAnchorElement>} to={agent.href} {...rest}>
        {children}
      </Link>
    );
  }
  return (
    <span ref={ref as Ref<HTMLSpanElement>} {...rest}>
      {children}
    </span>
  );
});

// External (non-deployed) dev-tool chips get a small tag in tooltips / overflow.
function ExternalTag() {
  return (
    <span className="inline-flex items-center rounded border border-current px-1 py-px text-[10px] font-medium uppercase leading-none tracking-wide opacity-70">
      External
    </span>
  );
}

function AgentChips({ agents }: { agents: InsightsAgentChip[] }) {
  if (agents.length === 0) {
    return <span className="text-muted-foreground">-</span>;
  }

  const visible = agents.slice(0, MAX_VISIBLE_AGENTS);
  const overflow = agents.length - visible.length;

  return (
    <div className="flex items-center gap-1.5">
      <span className="sr-only">{agents.map((agent) => agent.label).join(", ")}</span>
      {visible.map((agent) => (
        <TooltipProvider key={agent.key} delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <ChipShell agent={agent} className="inline-flex rounded-[3px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <AgentChipAvatar agent={agent} />
              </ChipShell>
            </TooltipTrigger>
            <TooltipContent side="top" className="flex items-center gap-1.5">
              <span>{agent.label}{agent.is_deleted ? " (deleted)" : ""}</span>
              {!agent.href && <ExternalTag />}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      ))}
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={agents.length}
          itemNoun={{ singular: "agent", plural: "agents" }}
        >
          <ul className="min-h-0 overflow-y-auto">
            {agents.map((agent) => (
              <li key={agent.key}>
                <ChipShell
                  agent={agent}
                  className="flex min-w-0 items-center gap-2 rounded px-2 py-1.5 text-body-sm hover:bg-muted/60"
                >
                  <AgentChipAvatar agent={agent} />
                  <span className="min-w-0 truncate text-foreground">
                    {agent.label}{agent.is_deleted ? " (deleted)" : ""}
                  </span>
                  {!agent.href && <ExternalTag />}
                </ChipShell>
              </li>
            ))}
          </ul>
        </OverflowPopover>
      )}
    </div>
  );
}

function NotInstrumentedMarker() {
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="text-faint-foreground" aria-label="Not instrumented" tabIndex={0}>
            <TriangleAlert className="size-3.5" />
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-[240px] [text-wrap:initial]">
          Instrumentation not available: no requests, spend, or token data available.
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function agentSortValue(row: InsightsAgentRow, key: AgentSortKey): number {
  switch (key) {
    case "cost_usd":
      return row.metrics.cost_usd;
    case "requests":
      return row.metrics.requests;
    case "cost_per_request":
      return row.metrics.cost_per_request;
    case "tok_per_request":
      return row.metrics.tok_per_request;
    case "p95_latency_ms":
      return row.metrics.p95_latency_ms;
  }
}

function userSortValue(row: InsightsPersonRow, key: UserSortKey): number {
  switch (key) {
    case "cost_usd":
      return row.metrics.cost_usd;
    case "requests":
      return row.metrics.requests;
    case "tokens":
      return row.metrics.tokens;
    case "last_seen":
      return row.metrics.last_seen ? new Date(row.metrics.last_seen).getTime() : 0;
  }
}

function useVisibleRows<Row extends { key: string }>(rows: Row[], pagination?: TopSpendersPagination) {
  const [expanded, setExpanded] = useState(false);
  const rowKeySignal = useMemo(() => rows.map((row) => row.key).join("\u0000"), [rows]);

  useEffect(() => {
    setExpanded(false);
  }, [rowKeySignal]);

  if (pagination) {
    const defaultVisibleRows = pagination.defaultVisibleRows ?? DEFAULT_VISIBLE_ROWS;
    const pageSize = pagination.pageSize ?? PAGE_SIZE_ROWS;
    const hiddenCount = Math.max(pagination.totalRows - rows.length, 0);
    return {
      visibleRows: rows,
      hiddenCount,
      expanded: rows.length > defaultVisibleRows,
      setExpanded,
      revealedCount: Math.max(rows.length - defaultVisibleRows, 0),
      showMoreLabel: hiddenCount > 0 ? `Show ${Math.min(pageSize, hiddenCount)} more` : undefined,
      showLessLabel: pagination.showLessLabel,
      onToggle: pagination.onShowMore,
      onShowLess: pagination.onShowLess,
    };
  }
  const visibleRows = expanded ? rows : rows.slice(0, DEFAULT_VISIBLE_ROWS);
  const hiddenCount = Math.max(rows.length - DEFAULT_VISIBLE_ROWS, 0);

  return {
    visibleRows,
    hiddenCount,
    expanded,
    setExpanded,
    revealedCount: expanded ? hiddenCount : 0,
    showMoreLabel: undefined,
    showLessLabel: undefined,
    onToggle: () => setExpanded((value) => !value),
    onShowLess: undefined,
  };
}

function AgentsTopSpenders({
  rows,
  loading,
  groupLabel = "Name",
  panelHeader,
  sortKey: controlledSortKey,
  sortDirection,
  onSort,
  pagination,
}: Extract<TopSpendersTableProps, { mode: "agents" }>) {
  const localSort = useSort<AgentSortKey>("cost_usd");
  const sortKey = controlledSortKey ?? localSort.sortKey;
  const asc = sortDirection ? sortDirection === "asc" : localSort.asc;
  const handleSort = (key: AgentSortKey) => {
    if (onSort) onSort(key);
    else localSort.handleSort(key);
  };
  const sorted = useMemo(
    () => {
      if (onSort) return rows;
      return [...rows].sort((a, b) => {
        const diff = agentSortValue(a, sortKey) - agentSortValue(b, sortKey);
        return asc ? diff : -diff;
      });
    },
    [rows, sortKey, asc, onSort],
  );
  const { visibleRows, hiddenCount, expanded, revealedCount, showMoreLabel, showLessLabel, onToggle, onShowLess } = useVisibleRows(sorted, pagination);
  const dir = (col: AgentSortKey) => sortDirFor(sortKey, asc, col);

  return (
    <SpendersTableShell
      panelHeader={panelHeader}
      footer={
        hiddenCount > 0 || revealedCount > 0 ? (
          <TableShowMore
            hiddenCount={hiddenCount}
            expanded={expanded}
            onToggle={onToggle}
            showMoreLabel={showMoreLabel}
            revealedCount={revealedCount}
            onShowLess={onShowLess}
            showLessLabel={showLessLabel}
          />
        ) : undefined
      }
      head={
        <>
          <TableHead className="pl-3">{groupLabel}</TableHead>
          <TableHead>Used by</TableHead>
          <TableHead sortable sortDirection={dir("requests")} onSort={() => handleSort("requests")} className="text-right">Requests</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Spend</TableHead>
          <TableHead className="text-right">% Total</TableHead>
          <TableHead sortable sortDirection={dir("cost_per_request")} onSort={() => handleSort("cost_per_request")} className="text-right">Spend/Req</TableHead>
          <TableHead sortable sortDirection={dir("tok_per_request")} onSort={() => handleSort("tok_per_request")} className="text-right">Tok/Req</TableHead>
          <TableHead sortable sortDirection={dir("p95_latency_ms")} onSort={() => handleSort("p95_latency_ms")} className="text-right">P95</TableHead>
        </>
      }
    >
      {loading ? (
        Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={8} />)
      ) : sorted.length === 0 ? (
        <TableRow>
          <TableCell colSpan={8} className="py-10 text-center text-body-sm text-faint-foreground">
            No deployment activity in this period
          </TableCell>
        </TableRow>
      ) : (
        visibleRows.map((row, index) => (
          <TableRow key={row.key}>
            <IdentityTableCell rank={index + 1}>
              <span className="inline-flex min-w-0 items-center gap-1.5">
                <AgentRowIdentity identity={row.identity} />
                {row.not_instrumented && <NotInstrumentedMarker />}
              </span>
            </IdentityTableCell>
            <TableCell>
              <IdentityAvatarStack identities={row.used_by} />
            </TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(row.metrics.requests)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCost(row.metrics.cost_usd)}</TableCell>
            <TableCell className="text-right text-foreground">{formatShare(row.metrics.cost_pct)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCost(row.metrics.cost_per_request)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(row.metrics.tok_per_request)}</TableCell>
            <TableCell className="text-right text-foreground">
              {row.metrics.p95_latency_ms > 0 ? formatLatency(row.metrics.p95_latency_ms) : "-"}
            </TableCell>
          </TableRow>
        ))
      )}
    </SpendersTableShell>
  );
}

function UsersTopSpenders({
  rows,
  loading,
  panelHeader,
  sortKey: controlledSortKey,
  sortDirection,
  onSort,
  pagination,
}: Extract<TopSpendersTableProps, { mode: "users" }>) {
  const localSort = useSort<UserSortKey>("cost_usd");
  const sortKey = controlledSortKey ?? localSort.sortKey;
  const asc = sortDirection ? sortDirection === "asc" : localSort.asc;
  const handleSort = (key: UserSortKey) => {
    if (onSort) onSort(key);
    else localSort.handleSort(key);
  };
  const sorted = useMemo(
    () => {
      if (onSort) return rows;
      return [...rows].sort((a, b) => {
        if (a.identity.kind === "system" && b.identity.kind !== "system") return 1;
        if (b.identity.kind === "system" && a.identity.kind !== "system") return -1;
        const diff = userSortValue(a, sortKey) - userSortValue(b, sortKey);
        return asc ? diff : -diff;
      });
    },
    [rows, sortKey, asc, onSort],
  );
  const { visibleRows, hiddenCount, expanded, revealedCount, showMoreLabel, showLessLabel, onToggle, onShowLess } = useVisibleRows(sorted, pagination);
  const dir = (col: UserSortKey) => sortDirFor(sortKey, asc, col);

  return (
    <SpendersTableShell
      panelHeader={panelHeader}
      footer={
        hiddenCount > 0 || revealedCount > 0 ? (
          <TableShowMore
            hiddenCount={hiddenCount}
            expanded={expanded}
            onToggle={onToggle}
            showMoreLabel={showMoreLabel}
            revealedCount={revealedCount}
            onShowLess={onShowLess}
            showLessLabel={showLessLabel}
          />
        ) : undefined
      }
      head={
        <>
          <TableHead className="pl-3">Name</TableHead>
          <TableHead>Agents Used</TableHead>
          <TableHead sortable sortDirection={dir("cost_usd")} onSort={() => handleSort("cost_usd")} className="text-right">Spend</TableHead>
          <TableHead className="text-right">% Total</TableHead>
          <TableHead sortable sortDirection={dir("requests")} onSort={() => handleSort("requests")} className="text-right">Requests</TableHead>
          <TableHead sortable sortDirection={dir("tokens")} onSort={() => handleSort("tokens")} className="text-right">Tokens</TableHead>
          <TableHead sortable sortDirection={dir("last_seen")} onSort={() => handleSort("last_seen")} className="text-right">Last Used</TableHead>
        </>
      }
    >
      {loading ? (
        Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} columns={7} />)
      ) : sorted.length === 0 ? (
        <TableRow>
          <TableCell colSpan={7} className="py-10 text-center text-body-sm text-faint-foreground">
            No activity from people in this period
          </TableCell>
        </TableRow>
      ) : (
        visibleRows.map((row, index) => (
          <TableRow key={row.key}>
            <IdentityTableCell rank={index + 1}>
              <IdentityRow identity={row.identity} />
            </IdentityTableCell>
            <TableCell>
              <AgentChips agents={row.agents_used} />
            </TableCell>
            <TableCell className="text-right text-foreground">{formatCost(row.metrics.cost_usd)}</TableCell>
            <TableCell className="text-right text-foreground">{formatShare(row.metrics.cost_pct)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(row.metrics.requests)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(row.metrics.tokens)}</TableCell>
            <TableCell className="text-right text-foreground">{formatLastSeen(row.metrics.last_seen)}</TableCell>
          </TableRow>
        ))
      )}
    </SpendersTableShell>
  );
}

// Per-model cost / tokens / requests / tail latency for the account. Self-fetches
// the observability summary (a different endpoint than the agents/people rows).
function ModelsTopSpenders({ account, days, panelHeader }: Extract<TopSpendersTableProps, { mode: "models" }>) {
  const params = useMemo(() => rangeParams(days), [days]);
  const { data, isLoading } = useAccountObservabilitySummary(account, params, { window: String(days) });
  const models = useMemo(
    () => [...(data?.cost_by_model ?? [])].sort((a, b) => b.cost_usd - a.cost_usd),
    [data?.cost_by_model],
  );

  return (
    <SpendersTableShell
      panelHeader={panelHeader}
      head={
        <>
          <TableHead className="pl-3">Model</TableHead>
          <TableHead className="text-right">Requests</TableHead>
          <TableHead className="text-right">Spend</TableHead>
          <TableHead className="text-right">% Total</TableHead>
          <TableHead className="text-right">Tokens</TableHead>
          <TableHead className="pr-4 text-right">p95</TableHead>
        </>
      }
    >
      {models.length === 0 ? (
        <TableRow>
          <TableCell colSpan={6} className="py-10 text-center text-body-sm text-faint-foreground">
            {isLoading ? "Loading model usage..." : "No model usage in this period"}
          </TableCell>
        </TableRow>
      ) : (
        models.map((m) => (
          <TableRow key={m.model}>
            <TableCell className="pl-3">
              <div className="max-w-[22rem] truncate text-foreground" title={m.model}>{modelDisplayName(m.model)}</div>
            </TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(m.requests)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCost(m.cost_usd)}</TableCell>
            <TableCell className="text-right text-foreground">{formatShare(m.cost_pct)}</TableCell>
            <TableCell className="text-right text-foreground">{formatCompact(m.total_tokens)}</TableCell>
            <TableCell className="pr-4 text-right text-foreground">
              {m.p95_latency_ms > 0 ? formatLatency(m.p95_latency_ms) : "-"}
            </TableCell>
          </TableRow>
        ))
      )}
    </SpendersTableShell>
  );
}
