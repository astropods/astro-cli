import { MoreVertical } from "lucide-react";
import { cn } from "@/lib/utils";
import { StatusIndicator } from "@/components/StatusIndicator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { statusVariant, statusLabel } from "./history/utils";
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
  const isSpinning = row.status === "deploying" || row.status === "undeploying";

  return (
    <div
      className={cn(
        "grid items-center",
        isCurrent && "border-l-[3px] border-l-teal-600",
        !isLastRow && !isCurrent && "border-b border-border",
        className,
      )}
      style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding }}
    >
      {/* Name */}
      <div className="min-w-0">
        <div className="font-sans text-body font-medium text-foreground truncate" title={row.rowLabel}>
          {row.rowLabel}
        </div>
      </div>

      {/* Status */}
      <div className="flex justify-end items-center">
        <StatusIndicator
          variant={statusVariant(row.status)}
          spinner={isSpinning}
        >
          {statusLabel(row.status)}
        </StatusIndicator>
      </div>

      {/* Duration */}
      <span className="font-sans text-body text-foreground text-right">{row.duration}</span>

      {/* Build */}
      <span className="font-sans text-body text-foreground text-right">{row.build}</span>

      {/* Deployed on + kebab (non-compact only) */}
      {!isCompact && (
        <>
          <span className="font-sans text-body text-foreground whitespace-nowrap text-right pl-3">
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
