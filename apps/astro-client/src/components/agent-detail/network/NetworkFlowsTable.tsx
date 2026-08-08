import { useState } from "react";
import { ChevronDown, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { getIntegrationIconUrl } from "@/lib/assets";
import { useResolvedTheme } from "@/lib/theme";
import type { NetworkDirection, NetworkFlow } from "@/lib/api";
import { iconIdForPeer } from "./destination-groups";
import { formatCompactNumber } from "../charts/chart-utils";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

const DEFAULT_VISIBLE = 10;

const PEER_HEADER: Record<NetworkDirection, string> = {
  inbound: "Route",
  outbound: "Destination",
  database: "Database",
};

export interface NetworkFlowsTableProps {
  flows: NetworkFlow[];
  direction: NetworkDirection;
  loading?: boolean;
}

export function NetworkFlowsTable({ flows, direction, loading }: NetworkFlowsTableProps) {
  const [expanded, setExpanded] = useState(false);
  const visible = expanded ? flows : flows.slice(0, DEFAULT_VISIBLE);
  const hiddenCount = flows.length - DEFAULT_VISIBLE;
  const showStatus = direction !== "database";
  const columnCount = showStatus ? 3 : 2;

  return (
    <TooltipProvider delayDuration={150}>
      <Table
        className="min-w-[600px] bg-card"
        containerClassName="rounded-lg"
      >
        <TableHeader className="bg-card">
          <TableRow>
            <TableHead>
              <span className="inline-flex items-baseline gap-2">
                <span>{PEER_HEADER[direction]}</span>
                <span className="text-faint-foreground">{flows.length}</span>
              </span>
            </TableHead>
            <TableHead className="text-right">Requests</TableHead>
            {showStatus && <TableHead>Responses</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-10 text-center">
                <Loader2 className="mx-auto size-5 animate-spin text-muted-foreground" />
              </TableCell>
            </TableRow>
          ) : flows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-10 text-center text-body-sm text-muted-foreground">
                No traffic recorded.
              </TableCell>
            </TableRow>
          ) : (
            visible.map((flow) => (
              <TableRow key={flow.peer}>
                <TableCell className="max-w-[280px] font-mono text-body-sm text-foreground">
                  <PeerCell flow={flow} direction={direction} />
                </TableCell>
                <TableCell className="whitespace-nowrap text-right font-mono text-body-sm text-foreground">
                  {formatCompactNumber(flow.request_count)}
                </TableCell>
                {showStatus && (
                  <TableCell>
                    <ResponsesCell flow={flow} />
                  </TableCell>
                )}
              </TableRow>
            ))
          )}
        </TableBody>
        {hiddenCount > 0 && (
          <TableFooter>
            <TableRow>
              <TableCell colSpan={columnCount} className="p-0">
                <button
                  onClick={() => setExpanded((e) => !e)}
                  className="flex w-full items-center justify-center gap-1.5 py-3 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
                >
                  <ChevronDown className={cn("size-3.5 transition-transform", expanded && "rotate-180")} />
                  {expanded ? "Show less" : `Show ${hiddenCount} more`}
                </button>
              </TableCell>
            </TableRow>
          </TableFooter>
        )}
      </Table>
    </TooltipProvider>
  );
}

/** The icon slot collapses when nothing resolves, so route rows stay flush left. */
function PeerCell({ flow, direction }: { flow: NetworkFlow; direction: NetworkDirection }) {
  const resolvedTheme = useResolvedTheme();
  const iconId = iconIdForPeer(flow.peer, direction, flow.registrable_domain);

  return (
    <span className="flex items-center gap-2">
      {iconId && (
        <img
          src={getIntegrationIconUrl(iconId, resolvedTheme)}
          alt=""
          className="size-4 shrink-0 object-contain dark:brightness-150"
          loading="lazy"
        />
      )}
      <span className="min-w-0 truncate">{flow.peer}</span>
    </span>
  );
}

function formatSuccessPct(ok: number, total: number): string {
  if (total === 0) return "—";
  const pct = (ok / total) * 100;
  if (pct === 100) return "100%";
  // Avoid rounding 99.97% up to "100%" when errors actually exist.
  if (pct >= 99.95) return "99.9%";
  return `${pct.toFixed(1)}%`;
}

function ResponsesCell({ flow }: { flow: NetworkFlow }) {
  const codes = flow.status_codes ?? {};
  const errors4xx = codes["4xx"] ?? 0;
  const errors5xx = codes["5xx"] ?? 0;
  // Everything not flagged as an error rolls into "ok" — covers 2xx, 3xx,
  // and any non-numeric "other" classes the handler passes through.
  const okCount = Math.max(0, flow.request_count - errors4xx - errors5xx);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="-mx-4 -my-2 flex cursor-default items-center gap-2 px-4 py-2">
          <span className="flex h-1.5 w-24 overflow-hidden rounded-full bg-muted">
            {okCount > 0 && (
              <span style={{ flexGrow: okCount, background: "var(--success)" }} />
            )}
            {errors4xx > 0 && (
              <span style={{ flexGrow: errors4xx, background: "var(--warning)" }} />
            )}
            {errors5xx > 0 && (
              <span style={{ flexGrow: errors5xx, background: "var(--error)" }} />
            )}
          </span>
          <span className="font-mono text-body-sm text-foreground tabular-nums">
            {formatSuccessPct(okCount, flow.request_count)} ok
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <div className="flex flex-col gap-0.5 font-mono">
          <span>{formatCompactNumber(okCount)} ok</span>
          {errors4xx > 0 && <span>{formatCompactNumber(errors4xx)} client errors (4xx)</span>}
          {errors5xx > 0 && <span>{formatCompactNumber(errors5xx)} server errors (5xx)</span>}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
