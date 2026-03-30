import { MoreVertical } from "lucide-react";
import { cn } from "@/lib/utils";
import { InlineBadge } from "@/components/InlineBadge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { statusLabel } from "./history/utils";
import type { DeployHistoryStatus, DeploymentHistoryTableRow } from "./history/types";

const STATUS_BADGE_STYLE: Record<DeployHistoryStatus, React.CSSProperties> = {
  active: {
    color: "var(--color-teal-600)",
    background: "color-mix(in oklch, var(--color-teal-600) 12%, transparent)",
  },
  deploying: {
    color: "var(--color-yellow-700)",
    background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)",
  },
  undeploying: {
    color: "var(--color-stone-500)",
    background: "color-mix(in oklch, var(--color-stone-500) 12%, transparent)",
  },
  ready: {
    color: "var(--color-stone-500)",
    background: "color-mix(in oklch, var(--color-stone-500) 12%, transparent)",
  },
  failed: {
    color: "var(--color-red-700)",
    background: "color-mix(in oklch, var(--color-red-700) 12%, transparent)",
  },
  undeployed: {
    color: "var(--color-stone-500)",
    background: "color-mix(in oklch, var(--color-stone-500) 12%, transparent)",
  },
};

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
        isCurrent && "border-l-[3px] border-l-teal-600",
        !isLastRow && !isCurrent && "border-b border-border",
        className,
      )}
      style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding }}
    >
      {/* Name */}
      <div className="min-w-0">
        <div className="font-mono text-body font-medium text-foreground truncate" title={row.rowLabel}>
          {row.rowLabel}
        </div>
      </div>

      {/* Status */}
      <div className="flex justify-end items-center">
        <InlineBadge variant="soft" style={STATUS_BADGE_STYLE[row.status]}>
          {statusLabel(row.status)}
        </InlineBadge>
      </div>

      {/* Duration */}
      <span className="font-mono text-body text-foreground text-right">{row.duration}</span>

      {/* Build */}
      <span className="font-mono text-body text-foreground text-right">{row.build}</span>

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
