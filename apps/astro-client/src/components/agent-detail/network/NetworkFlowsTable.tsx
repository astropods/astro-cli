import { useState } from "react";
import { ChevronDown, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { NetworkDirection, NetworkFlow } from "@/lib/api";
import { formatCompactNumber } from "../charts/chart-utils";
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

  return (
    <div className="overflow-clip rounded-lg border border-border/60 bg-card dark:bg-surface">
      <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
        <p className="text-mono-sm text-muted-foreground">{PEER_HEADER[direction]}s</p>
        <span className="text-mono-sm text-muted-foreground">
          {flows.length} {flows.length === 1 ? "peer" : "peers"}
        </span>
      </div>

      {loading ? (
        <div className="flex h-[200px] items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : flows.length === 0 ? (
        <div className="flex h-[200px] items-center justify-center">
          <p className="text-body-sm text-muted-foreground">No traffic recorded.</p>
        </div>
      ) : (
        <TooltipProvider delayDuration={150}>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[600px] border-collapse">
              <thead>
                <tr className="border-b border-border/60">
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">
                    {PEER_HEADER[direction]}
                  </th>
                  <th className="px-4 py-2 text-right text-mono-sm font-normal text-muted-foreground">Requests</th>
                  {showStatus && (
                    <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Responses</th>
                  )}
                </tr>
              </thead>
              <tbody>
                {visible.map((flow) => (
                  <tr
                    key={flow.peer}
                    className="border-b border-border/30 transition-colors hover:bg-black/2 dark:hover:bg-white/3"
                  >
                    <td className="max-w-[280px] truncate px-4 py-2.5 font-mono text-body-sm text-foreground">
                      {flow.peer}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-right font-mono text-body-sm text-foreground">
                      {formatCompactNumber(flow.request_count)}
                    </td>
                    {showStatus && (
                      <td className="px-4 py-2.5">
                        <ResponsesCell flow={flow} />
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {hiddenCount > 0 && (
            <button
              onClick={() => setExpanded((e) => !e)}
              className="flex w-full items-center justify-center gap-1.5 py-3 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <ChevronDown className={cn("size-3.5 transition-transform", expanded && "rotate-180")} />
              {expanded ? "Show less" : `Show ${hiddenCount} more`}
            </button>
          )}
        </TooltipProvider>
      )}
    </div>
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
        <span className="-mx-4 -my-2.5 flex cursor-default items-center gap-2 px-4 py-2.5">
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
