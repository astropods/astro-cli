import { MoreVertical } from "lucide-react";
import { cn } from "@/lib/utils";
import { HistoryStatusBadge, HISTORY_STATUS_FG } from "./HistoryStatusBadge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { DeploymentHistoryTableRow } from "./history/types";


interface DeploymentHistoryRowProps {
  row: DeploymentHistoryTableRow;
  isCompact: boolean;
  gridColumns: string;
  gridGap: number;
  padding: string;
  isCurrent?: boolean;
  isLastRow?: boolean;
  onRollback?: () => void;
  className?: string;
}

export function DeploymentHistoryRow({
  row,
  isCompact,
  gridColumns,
  gridGap,
  padding,
  isCurrent = false,
  isLastRow = false,
  onRollback,
  className,
}: DeploymentHistoryRowProps) {
  return (
    <div
      className={cn(
        "grid items-center",
        !isLastRow && "border-b border-border",
        className,
      )}
      style={{
        gridTemplateColumns: gridColumns,
        gap: gridGap,
        padding,
        ...(isCurrent && { boxShadow: `inset 3px 0 0 ${HISTORY_STATUS_FG[row.status]}` }),
      }}
    >
      {/* Name */}
      <div className="min-w-0">
        <div className="font-mono text-body font-medium text-foreground truncate" title={row.rowLabel}>
          {row.rowLabel}
        </div>
      </div>

      {/* Status */}
      <div className="flex justify-end items-center">
        <HistoryStatusBadge status={row.status} />
      </div>

      {/* Duration */}
      <span className="font-mono text-body text-foreground text-right">{row.duration}</span>

      {/* Deployed on + kebab (non-compact only) */}
      {!isCompact && (
        <>
          <span className="font-mono text-body text-foreground whitespace-nowrap text-right pl-3">
            {row.time}, {row.timeOfDay}
          </span>
          {isCurrent ? (
            <span />
          ) : onRollback ? (
            <div className="relative">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className="bg-transparent border-none cursor-pointer text-foreground flex p-1 rounded hover:bg-muted"
                    aria-label={`Actions for deployment ${row.id}`}
                  >
                    <MoreVertical size={14} />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={onRollback}>
                    Rollback
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ) : (
            <span />
          )}
        </>
      )}
    </div>
  );
}
