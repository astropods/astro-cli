import { useState, useMemo, useRef } from "react";
import { Loader2 } from "lucide-react";
import { ChevronUpIcon, ChevronDownIcon, ChevronRightIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { ErrorPanel } from "@/components/ui/status-panel";
import { InlineBadge } from "@/components/InlineBadge";
import { ActiveContainerAccordion } from "./ActiveContainerAccordion";
import { DeploymentHistoryRow } from "./DeploymentHistoryRow";
import { BuildHistoryGroup } from "./BuildHistoryGroup";
import type { DeploymentHistoryTableRow, ServiceRow, DeployHistoryStatus } from "./history/types";

const DEPLOYMENT_GRID_COLUMNS = "minmax(180px, 1fr) 88px 84px 185px 28px";

interface DeploymentHistoryTableProps {
  currentRow: DeploymentHistoryTableRow | null;
  pastRows: DeploymentHistoryTableRow[];
  serviceRows: ServiceRow[];
  deploymentId: string;
  isCompact: boolean;
  openContainers: Set<string>;
  onToggleContainer: (id: string) => void;
  onRollback?: (revision: number, buildId: string) => void;
  historyLoading: boolean;
  historyError: boolean;
  isPausing?: boolean;
  isResuming?: boolean;
  isRestarting?: boolean;
  isGloballyRestarting?: boolean;
  onPodRestartStateChange?: (isRestarting: boolean) => void;
}

export function DeploymentHistoryTable({
  currentRow,
  pastRows,
  serviceRows,
  deploymentId,
  isCompact,
  openContainers,
  onToggleContainer,
  onRollback,
  historyLoading,
  historyError,
  isPausing = false,
  isResuming = false,
  isRestarting = false,
  isGloballyRestarting = false,
  onPodRestartStateChange,
}: DeploymentHistoryTableProps) {
  // Track how many pods are locally restarting so the status row reflects it
  const restartingPodCountRef = useRef(0);
  const [isPodRestarting, setIsPodRestarting] = useState(false);
  type OverrideStatus = "restarting" | "pausing" | "resuming";
  const statusOverride: OverrideStatus | null = (isRestarting || isPodRestarting) ? "restarting" : isPausing ? "pausing" : isResuming ? "resuming" : null;
  // effectiveCurrentRow drives the status row display (shows restarting/pausing/resuming)
  const effectiveCurrentRow = currentRow && statusOverride
    ? { ...currentRow, status: statusOverride }
    : currentRow;
  const accordionDeploymentStatus: DeployHistoryStatus = isPausing ? "pausing" : isResuming ? "resuming" : (currentRow?.status ?? "active");
  const [showAllHistory, setShowAllHistory] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(true);

  const gridColumns = isCompact
    ? "minmax(0, 1.35fr) minmax(0, 0.78fr) minmax(0, 0.65fr)"
    : DEPLOYMENT_GRID_COLUMNS;
  const gridGap = isCompact ? 8 : 12;
  const headerPadding = isCompact ? "12px 10px" : "12px 14px";
  const rowPadding = isCompact ? "10px 10px" : "11px 14px";
  const currentRowPadding = isCompact ? "11px 10px" : "12px 14px";
  const gridHeaders = isCompact
    ? ["Deployment", "Status", "Duration"]
    : ["Deployment", "Status", "Duration", "Deployed on", ""];
  const [currentOpen, setCurrentOpen] = useState(true);
  const hasCollapsedHistory = pastRows.length > 4;
  const visiblePastRows = showAllHistory ? pastRows : pastRows.slice(0, 4);

  // Group past rows by build_id for collapsible history display
  const buildGroups = useMemo(() => {
    const map = new Map<string, DeploymentHistoryTableRow[]>();
    for (const row of visiblePastRows) {
      const bid = row.source.build_id || 'unknown';
      if (!map.has(bid)) map.set(bid, []);
      map.get(bid)!.push(row);
    }
    return Array.from(map.values());
  }, [visiblePastRows]);

  const totalUniqueBuildCount = useMemo(() => {
    return new Set(pastRows.map(r => r.source.build_id || 'unknown')).size;
  }, [pastRows]);

  return (
    <div className="flex flex-col gap-3 max-w-full">

    {/* ── CURRENT DEPLOYMENT CARD ── */}
    <div className="bg-surface border border-border rounded-[10px] overflow-hidden">
      {/* Column headers double as the collapse toggle */}
      <button
        type="button"
        onClick={() => setCurrentOpen((o) => !o)}
        className="w-full grid border-b border-border bg-muted hover:bg-muted/60 transition-colors"
        style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding: headerPadding }}
      >
        <span className="flex items-center gap-2 font-mono text-label tracking-[0.07em] text-faint-foreground whitespace-nowrap min-w-0 truncate text-left">
          {currentOpen
            ? <ChevronDownIcon className="h-3.5 w-3.5 shrink-0" />
            : <ChevronRightIcon className="h-3.5 w-3.5 shrink-0" />}
          CURRENT DEPLOYMENT
        </span>
        {gridHeaders.slice(1).map((h, i) => (
          <span
            key={h}
            className={cn(
              "font-mono text-label tracking-[0.07em] text-faint-foreground whitespace-nowrap min-w-0 truncate text-right",
              i === 2 && "pl-3",
            )}
          >
            {h.toUpperCase()}
          </span>
        ))}
      </button>

      {/* Current deployment */}
      {currentOpen && (effectiveCurrentRow ? (
        <>
          <DeploymentHistoryRow
            row={effectiveCurrentRow}
            isCompact={isCompact}
            gridColumns={gridColumns}
            gridGap={gridGap}
            padding={currentRowPadding}
            isCurrent
          />

          {/* Services section */}
          <div className="px-4 py-2 pb-4 border-t border-border bg-muted">
            <div className="font-mono text-label tracking-[0.07em] text-faint-foreground my-1.5 flex items-center gap-1.5">
              Services
              {serviceRows.length > 0 && (
                <InlineBadge variant="fill" shape="square" className="normal-case size-[18px] p-0 justify-center text-muted-foreground text-[11px]">
                  {serviceRows.length}
                </InlineBadge>
              )}
            </div>
            {serviceRows.length === 0 ? (
              <p className="font-sans text-sm text-faint-foreground m-0 flex items-center gap-2">
                {(effectiveCurrentRow.status === "deploying" || effectiveCurrentRow.status === "undeploying" || effectiveCurrentRow.status === "restarting" || effectiveCurrentRow.status === "pausing" || effectiveCurrentRow.status === "resuming") && (
                  <Loader2
                    size={14}
                    className={cn(
                      "dp-spin",
                      effectiveCurrentRow.status === "deploying" ? "text-yellow-500"
                      : effectiveCurrentRow.status === "restarting" ? "text-yellow-500"
                      : effectiveCurrentRow.status === "pausing" ? "text-coral-600"
                      : effectiveCurrentRow.status === "resuming" ? "text-primary"
                      : "text-faint-foreground",
                    )}
                  />
                )}
                {effectiveCurrentRow.status === "deploying"
                  ? "Waiting for services to start and logs to stream…"
                  : effectiveCurrentRow.status === "undeploying"
                    ? "Tearing down services and streaming final logs…"
                    : effectiveCurrentRow.status === "restarting"
                      ? "Restarting service pods…"
                      : effectiveCurrentRow.status === "pausing"
                        ? "Pausing deployment…"
                        : effectiveCurrentRow.status === "resuming"
                          ? "Resuming deployment…"
                          : "No service data available"}
              </p>
            ) : (
              serviceRows.map((svc) => (
                <ActiveContainerAccordion
                  key={svc.id}
                  workloadName={svc.workloadName}
                  podName={svc.podName}
                  title={svc.title}
                  isCompact={isCompact}

                  urls={svc.urls}
                  readyText={svc.readyText}
                  uptime={svc.uptime}
                  deploymentId={deploymentId}
                  containers={svc.containers}
                  deploymentStatus={accordionDeploymentStatus}
                  isOpen={openContainers.has(svc.id)}
                  onToggle={() => onToggleContainer(svc.id)}
                  isGloballyRestarting={isGloballyRestarting}
                  onPodRestartStateChange={(restarting) => {
                    restartingPodCountRef.current = Math.max(0, restartingPodCountRef.current + (restarting ? 1 : -1));
                    const anyRestarting = restartingPodCountRef.current > 0;
                    setIsPodRestarting(anyRestarting);
                    onPodRestartStateChange?.(anyRestarting);
                  }}
                />
              ))
            )}
          </div>
        </>
      ) : (
        <div className="px-4 py-5 font-mono text-mono-sm text-faint-foreground">
          No active deployment found.
        </div>
      ))}
    </div>

    {/* ── HISTORY CARD ── */}
    {(historyLoading || historyError || pastRows.length > 0) && (
      <div className="bg-surface border border-border rounded-[10px] overflow-hidden">
        {/* Column headers double as the collapse toggle */}
        <button
          type="button"
          onClick={() => setHistoryOpen((o) => !o)}
          className="w-full grid border-b border-border bg-muted hover:bg-muted/60 transition-colors"
          style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding: headerPadding }}
        >
          <span className="flex items-center gap-2 font-mono text-label tracking-[0.07em] text-faint-foreground text-left min-w-0">
            {historyOpen
              ? <ChevronDownIcon className="h-3.5 w-3.5 shrink-0" />
              : <ChevronRightIcon className="h-3.5 w-3.5 shrink-0" />}
            <span>HISTORY</span>
            {pastRows.length > 0 && (
              <span className="flex items-center gap-1.5 normal-case tracking-normal ml-3">
                <span>Configs</span>
                <InlineBadge variant="fill" shape="square" className="normal-case size-[18px] p-0 justify-center text-muted-foreground text-[11px]">
                  {pastRows.length}
                </InlineBadge>
                <span>·</span>
                <span>Builds</span>
                <InlineBadge variant="fill" shape="square" className="normal-case size-[18px] p-0 justify-center text-muted-foreground text-[11px]">
                  {totalUniqueBuildCount}
                </InlineBadge>
              </span>
            )}
          </span>
          {gridHeaders.slice(1).map((h, i) => (
            <span
              key={h}
              className={cn(
                "font-mono text-label tracking-[0.07em] text-faint-foreground whitespace-nowrap min-w-0 truncate text-right",
                i === 2 && "pl-3",
              )}
            >
              {h.toUpperCase()}
            </span>
          ))}
        </button>

        {historyOpen && (
          <>
            {historyError && (
              <div className="p-3.5 pt-0">
                <ErrorPanel title="Unable to load deployment history" dismissible>
                  Could not load deployment history from the server.
                </ErrorPanel>
              </div>
            )}
            {historyLoading ? (
              <div className="px-3.5 pb-3.5 flex items-center gap-2 font-mono text-mono-sm text-faint-foreground">
                <Loader2 size={14} className="dp-spin" />
                Loading deployment history…
              </div>
            ) : pastRows.length === 0 ? null : (
              <>
                {buildGroups.map((groupRows, groupIdx) => (
                  <BuildHistoryGroup
                    key={groupRows[0].id}
                    rows={groupRows}
                    isCompact={isCompact}
                    gridColumns={gridColumns}
                    gridGap={gridGap}
                    padding={rowPadding}
                    subRowPadding={isCompact ? "8px 10px" : "9px 14px"}
                    isLastGroup={groupIdx === buildGroups.length - 1 && !hasCollapsedHistory}
                    onRollback={onRollback}
                  />
                ))}
                {hasCollapsedHistory && (
                  <div className="flex justify-center px-3 pt-2 pb-0.5">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="font-medium"
                      onClick={() => setShowAllHistory((prev) => !prev)}
                    >
                      {showAllHistory ? (
                        <>Show less <ChevronUpIcon className="h-3.5 w-3.5" /></>
                      ) : (
                        <>Show {pastRows.length - 4} more <ChevronDownIcon className="h-3.5 w-3.5" /></>
                      )}
                    </Button>
                  </div>
                )}
              </>
            )}
          </>
        )}
      </div>
    )}

    </div>
  );
}
