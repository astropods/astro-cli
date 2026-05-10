import { ChevronDown, ChevronUp, Maximize2, Minimize2, X } from "lucide-react";
import { formatTimestamp } from "../trace-utils";

export interface TracePanelHeaderProps {
  timestamp: string;
  traceId: string;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
  expanded?: boolean;
  onToggleExpanded?: () => void;
}

export function TracePanelHeader({
  timestamp,
  traceId,
  onClose,
  canGoPrev,
  canGoNext,
  onNavigate,
  expanded,
  onToggleExpanded,
}: TracePanelHeaderProps) {
  return (
    <div className="flex items-center gap-3 border-b border-border px-4 py-3">
      <div className="min-w-0 flex-1">
        <h3 className="text-heading-4 text-foreground">
          {formatTimestamp(timestamp, true)}
        </h3>
        <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground/40">
          {traceId}
        </p>
      </div>

      <div className="flex shrink-0 items-center gap-1">
        {onNavigate && (
          <>
            <button
              type="button"
              onClick={() => onNavigate("prev")}
              disabled={!canGoPrev}
              aria-label="Previous trace"
              className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
            >
              <ChevronUp className="size-4" />
            </button>
            <button
              type="button"
              onClick={() => onNavigate("next")}
              disabled={!canGoNext}
              aria-label="Next trace"
              className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
            >
              <ChevronDown className="size-4" />
            </button>
          </>
        )}
        {onToggleExpanded && (
          <button
            type="button"
            onClick={onToggleExpanded}
            aria-label={expanded ? "Restore panel size" : "Expand panel to full width"}
            className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            {expanded ? (
              <Minimize2 className="size-4" />
            ) : (
              <Maximize2 className="size-4" />
            )}
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          aria-label="Close trace"
          className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
        >
          <X className="size-4" />
        </button>
      </div>
    </div>
  );
}
