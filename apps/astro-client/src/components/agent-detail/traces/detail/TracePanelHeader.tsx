import { Check, ChevronDown, ChevronUp, Copy, Link, Maximize2, Minimize2, X } from "lucide-react";
import { formatTimestamp } from "../trace-utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

export interface TracePanelHeaderProps {
  timestamp: string;
  traceId: string;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
  expanded?: boolean;
  onToggleExpanded?: () => void;
  /** When provided, the header renders a button that copies a shareable link. */
  onShare?: () => void;
  /** Shows the copied confirmation on the share button. */
  shareCopied?: boolean;
}

// The copy-id and copy-link buttons live next to the trace id (not with the
// panel controls) and render at 12px so the id row stays compact.
const ID_BUTTON_CLASS =
  "shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground";

function CopyIdButton({ traceId }: { traceId: string }) {
  const { copy, copied } = useCopyToClipboard();
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        void copy(traceId);
      }}
      title={copied ? "Copied!" : "Copy trace ID"}
      aria-label="Copy trace ID"
      className={ID_BUTTON_CLASS}
    >
      {copied ? <Check className="size-3 text-primary" /> : <Copy className="size-3" />}
    </button>
  );
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
  onShare,
  shareCopied,
}: TracePanelHeaderProps) {
  return (
    <div className="flex items-center gap-3 border-b border-border px-4 py-3">
      <div className="min-w-0 flex-1">
        <h3 className="text-heading-4 text-foreground">
          {formatTimestamp(timestamp, true)}
        </h3>
        <div className="mt-1 flex items-center gap-1.5">
          <p className="min-w-0 truncate font-mono text-[11px] text-muted-foreground">
            {traceId}
          </p>
          <CopyIdButton traceId={traceId} />
          {onShare && (
            <button
              type="button"
              onClick={onShare}
              title="Copy link to this trace"
              aria-label="Copy link to this trace"
              className={ID_BUTTON_CLASS}
            >
              {shareCopied ? (
                <Check className="size-3 text-primary" />
              ) : (
                <Link className="size-3" />
              )}
            </button>
          )}
        </div>
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
            {/* Light divider separating trace navigation from panel controls. */}
            <span aria-hidden className="mx-1 h-4 w-px bg-border" />
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
