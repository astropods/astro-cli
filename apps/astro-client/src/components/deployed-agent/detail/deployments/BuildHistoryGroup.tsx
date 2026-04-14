import { useState } from "react";
import { ChevronUpIcon, ChevronDownIcon } from "@heroicons/react/24/outline";
import { MoreVertical } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { HistoryStatusBadge } from "./HistoryStatusBadge";
import { Tag } from "@/components/Tag";
import { cn } from "@/lib/utils";
import type { DeploymentHistoryTableRow } from "./history/types";


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
          <HistoryStatusBadge status="undeployed" />
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
            <div className="min-w-0 pl-4 flex items-center gap-1.5">
              {row.source.revision !== undefined && (
                <span className="font-mono text-mono-sm text-faint-foreground w-8 shrink-0 text-right">
                  #{row.source.revision}
                </span>
              )}
              <Tag color={deployType === "initial" ? "teal" : "blue"}>
                {deployType === "initial" ? "Initial deploy" : "Config change"}
              </Tag>
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
