import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";
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
import type { AccountBlueprintsSummaryResponse } from "@/lib/api";
import { formatModelName } from "./model-colors";

type Blueprint = AccountBlueprintsSummaryResponse["blueprints"][number];
type SortKey = "cost_usd" | "requests" | "cost_per_request" | "tok_per_request" | "p95_latency_ms";

function SortIcon({ active, asc }: { active: boolean; asc: boolean }) {
  if (!active) return <span className="ml-1 text-faint-foreground opacity-40">↕</span>;
  return <span className="ml-1 text-foreground">{asc ? "↑" : "↓"}</span>;
}

interface HeaderCellProps {
  label: string;
  sortKey?: SortKey;
  currentSort: SortKey;
  asc: boolean;
  onSort: (k: SortKey) => void;
  align?: "left" | "right";
}

function SortableHead({ label, sortKey, currentSort, asc, onSort, align = "right" }: HeaderCellProps) {
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

function GhostRow() {
  return (
    <TableRow>
      {Array.from({ length: 6 }).map((_, i) => (
        <TableCell key={i} className={i === 0 ? "pr-4" : ""}>
          <div className={cn("h-3.5 animate-pulse rounded bg-muted", i === 0 ? "w-[70%]" : "w-1/2")} />
        </TableCell>
      ))}
    </TableRow>
  );
}

interface TopSpendersTableProps {
  blueprints: Blueprint[];
  loading: boolean;
  groupLabel?: string;
}

export function TopSpendersTable({ blueprints, loading, groupLabel = "Agent" }: TopSpendersTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>("cost_usd");
  const [asc, setAsc] = useState(false);

  function handleSort(key: SortKey) {
    if (key === sortKey) {
      setAsc((v) => !v);
    } else {
      setSortKey(key);
      setAsc(false);
    }
  }

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
            Array.from({ length: 4 }).map((_, i) => <GhostRow key={i} />)
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
