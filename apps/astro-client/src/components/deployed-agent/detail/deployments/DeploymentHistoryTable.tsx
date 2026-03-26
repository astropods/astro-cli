import { useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { ErrorPanel } from "@/components/ui/status-panel";
import { InlineBadge } from "@/components/InlineBadge";
import { ActiveContainerAccordion } from "./ActiveContainerAccordion";
import { DeploymentHistoryRow } from "./DeploymentHistoryRow";
import type { DeploymentHistoryTableRow } from "./history/types";

const DEPLOYMENT_GRID_COLUMNS = "minmax(220px, 1fr) 88px 84px 116px 116px 28px";

interface ServiceRow {
  id: string;
  workloadName: string;
  title: string;
  isAgentService: boolean;
  readyText: string;
  uptime: string;
  containers: {
    name: string;
    ready: boolean;
    vars: { key: string; value: string; secret: boolean; source: string }[];
  }[];
  url?: string;
  urls?: { name: string; url: string; type?: string }[];
}

interface DeploymentHistoryTableProps {
  currentRow: DeploymentHistoryTableRow | null;
  pastRows: DeploymentHistoryTableRow[];
  serviceRows: ServiceRow[];
  deploymentId: string;
  isCompact: boolean;
  openContainers: Set<string>;
  onToggleContainer: (id: string) => void;
  onOpenConfigure?: () => void;
  historyLoading: boolean;
  historyError: boolean;
}

export function DeploymentHistoryTable({
  currentRow,
  pastRows,
  serviceRows,
  deploymentId,
  isCompact,
  openContainers,
  onToggleContainer,
  onOpenConfigure,
  historyLoading,
  historyError,
}: DeploymentHistoryTableProps) {
  const [showAllHistory, setShowAllHistory] = useState(false);

  const gridColumns = isCompact
    ? "minmax(0, 1.35fr) minmax(0, 0.78fr) minmax(0, 0.65fr) minmax(0, 0.78fr)"
    : DEPLOYMENT_GRID_COLUMNS;
  const gridGap = isCompact ? 8 : 12;
  const headerPadding = isCompact ? "8px 10px" : "8px 14px";
  const rowPadding = isCompact ? "10px 10px" : "11px 14px";
  const currentRowPadding = isCompact ? "11px 10px" : "12px 14px";
  const gridHeaders = isCompact
    ? ["Deployment", "Status", "Duration", "Build No."]
    : ["Deployment", "Status", "Duration", "Build No.", "Deployed on", ""];

  const hasCollapsedHistory = pastRows.length > 4;
  const visiblePastRows = showAllHistory ? pastRows : pastRows.slice(0, 4);

  return (
    <div className="bg-surface border border-border rounded-[10px] overflow-hidden max-w-full">
      {/* Grid header */}
      <div
        className="grid border-b border-border bg-muted"
        style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding: headerPadding }}
      >
        {gridHeaders.map((h, i) => (
          <span
            key={h}
            className={cn(
              "font-mono text-label tracking-[0.07em] text-faint-foreground whitespace-nowrap min-w-0 truncate",
              i === 0 ? "text-left" : "text-right",
            )}
          >
            {h.toUpperCase()}
          </span>
        ))}
      </div>

      {/* Current deployment */}
      {currentRow ? (
        <>
          <DeploymentHistoryRow
            row={currentRow}
            isCompact={isCompact}
            gridColumns={gridColumns}
            gridGap={gridGap}
            padding={currentRowPadding}
            isCurrent
          />

          {/* Services section */}
          <div className="px-4 py-2 pb-4 border-t border-border bg-muted">
            <div className="font-mono text-label tracking-[0.07em] text-faint-foreground uppercase my-1.5 flex items-center gap-1.5">
              Services
              {serviceRows.length > 0 && (
                <InlineBadge variant="fill" shape="square" className="normal-case size-[18px] p-0 justify-center text-muted-foreground text-[11px]">
                  {serviceRows.length}
                </InlineBadge>
              )}
            </div>
            {serviceRows.length === 0 ? (
              <p className="font-mono text-mono-sm text-faint-foreground m-0 flex items-center gap-2">
                {(currentRow.status === "deploying" || currentRow.status === "undeploying") && (
                  <Loader2
                    size={14}
                    className={cn(
                      "dp-spin",
                      currentRow.status === "deploying" ? "text-yellow-500" : "text-faint-foreground",
                    )}
                  />
                )}
                {currentRow.status === "deploying"
                  ? "Waiting for services to start and logs to stream…"
                  : currentRow.status === "undeploying"
                    ? "Tearing down services and streaming final logs…"
                    : "No service data available"}
              </p>
            ) : (
              serviceRows.map((svc) => (
                <ActiveContainerAccordion
                  key={svc.id}
                  workloadName={svc.workloadName}
                  title={svc.title}
                  isCompact={isCompact}
                  isAgentService={svc.isAgentService}
                  url={svc.url}
                  urls={svc.urls}
                  readyText={svc.readyText}
                  uptime={svc.uptime}
                  deploymentId={deploymentId}
                  containers={svc.containers}
                  deploymentStatus={currentRow.status}
                  isOpen={openContainers.has(svc.id)}
                  onToggle={() => onToggleContainer(svc.id)}
                />
              ))
            )}
          </div>
        </>
      ) : (
        <div className="px-4 py-5 font-mono text-mono-sm text-faint-foreground">
          No active deployment found.
        </div>
      )}

      {/* Past deployments */}
      {(historyLoading || historyError || pastRows.length > 0) && (
        <div className="border-t border-border bg-surface">
          {historyError && (
            <div className="p-3.5">
              <ErrorPanel title="Unable to load deployment history" dismissible>
                Could not load deployment history from the server.
              </ErrorPanel>
            </div>
          )}
          {historyLoading ? (
            <div className="p-3.5 flex items-center gap-2 font-mono text-mono-sm text-faint-foreground">
              <Loader2 size={14} className="dp-spin" />
              Loading deployment history…
            </div>
          ) : pastRows.length === 0 ? null : (
            <>
              {visiblePastRows.map((row, idx) => (
                <DeploymentHistoryRow
                  key={row.id}
                  row={row}
                  isCompact={isCompact}
                  gridColumns={gridColumns}
                  gridGap={gridGap}
                  padding={rowPadding}
                  isLastRow={idx === visiblePastRows.length - 1 && !hasCollapsedHistory}
                  onRollback={onOpenConfigure}
                />
              ))}
              {hasCollapsedHistory && (
                <div className="flex justify-center px-3 py-2 pb-2.5 border-t border-border">
                  <button
                    type="button"
                    onClick={() => setShowAllHistory((prev) => !prev)}
                    className="bg-transparent border-none cursor-pointer font-mono text-mono-sm tracking-[0.04em] text-faint-foreground underline"
                  >
                    {showAllHistory ? "See less" : `See more (${pastRows.length - 4} more)`}
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
