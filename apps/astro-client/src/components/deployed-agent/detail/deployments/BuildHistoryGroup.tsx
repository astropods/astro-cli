import { useState } from "react";
import { ChevronUpIcon, ChevronDownIcon } from "@heroicons/react/24/outline";
import { MoreVertical } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { InlineBadge } from "@/components/InlineBadge";
import { cn } from "@/lib/utils";
import { statusLabel } from "./history/utils";
import type { DeploymentHistoryTableRow } from "./history/types";

const UNDEPLOYED_BADGE_STYLE: React.CSSProperties = {
  color: "var(--color-stone-500)",
  background: "color-mix(in oklch, var(--color-stone-500) 12%, transparent)",
};

const DEPLOY_TYPE_BADGE: Record<"initial" | "config", React.CSSProperties> = {
  initial: {
    color: "var(--color-teal-700)",
    background: "color-mix(in oklch, var(--color-teal-700) 10%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-teal-700) 22%, transparent)",
  },
  config: {
    color: "var(--color-blue-600)",
    background: "color-mix(in oklch, var(--color-blue-600) 10%, transparent)",
    borderColor: "color-mix(in oklch, var(--color-blue-600) 22%, transparent)",
  },
};

export interface BuildHistoryGroupProps {
  rows: DeploymentHistoryTableRow[]; // all rows for this build_id, newest first
  isCompact: boolean;
  gridColumns: string;
  gridGap: number;
  padding: string;
  subRowPadding: string;
  isLastGroup: boolean;
  onRollback?: (revision: number, buildId: string) => void;
}

export function BuildHistoryGroup({
  rows,
  isCompact,
  gridColumns,
  gridGap,
  padding,
  subRowPadding,
  isLastGroup,
  onRollback,
}: BuildHistoryGroupProps) {
  const [open, setOpen] = useState(false);

  const headerRow = rows[0];
  const oldestRow = rows[rows.length - 1];

  return (
    <>
      {/* Header row */}
      <div
        className={cn(
          "grid items-center transition-[box-shadow] duration-700",
          !isLastGroup || open ? "border-b border-border" : "",
        )}
        style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding }}
      >
        {/* Name + expand chevron */}
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          className="min-w-0 flex items-center gap-1.5 bg-transparent border-none cursor-pointer text-left p-0 group"
        >
          {open
            ? <ChevronDownIcon className="h-3 w-3 shrink-0 text-faint-foreground" />
            : <ChevronUpIcon className="h-3 w-3 shrink-0 text-faint-foreground rotate-90" />}
          <span className="font-mono text-body font-medium text-foreground truncate group-hover:text-foreground" title={headerRow.rowLabel}>
            {headerRow.rowLabel}
          </span>
        </button>

        {/* Status */}
        <div className="flex justify-end items-center">
          <InlineBadge variant="soft" style={UNDEPLOYED_BADGE_STYLE}>
            {statusLabel("undeployed")}
          </InlineBadge>
        </div>

        {/* Duration */}
        <span className="font-mono text-body text-foreground text-right">{headerRow.duration}</span>

        {/* Deployed on (non-compact only) — shows the initial deploy date */}
        {!isCompact && (
          <>
            <span className="font-mono text-body text-foreground whitespace-nowrap text-right pl-3">
              {oldestRow.time}, {oldestRow.timeOfDay}
            </span>
            <span />
          </>
        )}
      </div>

      {/* Sub-rows */}
      {open && rows.map((row, idx) => {
        const isOldest = idx === rows.length - 1;
        const deployType: "initial" | "config" = isOldest ? "initial" : "config";
        const isLastSubRow = idx === rows.length - 1;

        return (
          <div
            key={row.id}
            className={cn(
              "grid items-center bg-muted/40 border-l-2 border-l-border",
              isLastSubRow && isLastGroup ? "" : "border-b border-border",
            )}
            style={{ gridTemplateColumns: gridColumns, gap: gridGap, padding: subRowPadding }}
          >
            {/* Name */}
            <div className="min-w-0 pl-5 flex items-center gap-1.5">
              <InlineBadge variant="soft" shape="square" style={DEPLOY_TYPE_BADGE[deployType]}>
                {deployType === "initial" ? "Initial deploy" : "Config change"}
              </InlineBadge>
              {row.source.revision !== undefined && (
                <span className="font-mono text-mono-sm text-faint-foreground">
                  #{row.source.revision}
                </span>
              )}
            </div>

            {/* Status — empty, inherits group's undeployed */}
            <span />

            {/* Duration */}
            <span className="font-mono text-body text-foreground text-right">{row.duration}</span>

            {/* Deployed on + kebab (non-compact only) */}
            {!isCompact && (
              <>
                <span className="font-mono text-body text-foreground whitespace-nowrap text-right pl-3">
                  {row.time}, {row.timeOfDay}
                </span>
                <div className="flex justify-end">
                  {onRollback && row.source.revision ? (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <button
                          type="button"
                          className="bg-transparent border-none cursor-pointer text-foreground flex p-1 rounded hover:bg-muted"
                        >
                          <MoreVertical size={14} />
                        </button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => onRollback(row.source.revision!, row.source.build_id ?? '')}>
                          Rollback
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  ) : (
                    <span />
                  )}
                </div>
              </>
            )}
          </div>
        );
      })}
    </>
  );
}
