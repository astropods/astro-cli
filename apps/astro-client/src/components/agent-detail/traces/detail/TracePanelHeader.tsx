import { Check, ChevronDown, ChevronUp, Copy, Link } from "lucide-react";
import { formatTimestamp } from "../trace-utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

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
      {copied ? <Check className="size-3 text-foreground-accent" /> : <Copy className="size-3" />}
    </button>
  );
}

export interface TracePanelTitleProps {
  timestamp: string;
  traceId: string;
  /** When provided, renders a button that copies a shareable link. */
  onShare?: () => void;
  /** Shows the copied confirmation on the share button. */
  shareCopied?: boolean;
}

// Left side of the trace panel header (fed to SidePanel's `title`): the trace
// timestamp with the id row and its copy / share buttons.
export function TracePanelTitle({ timestamp, traceId, onShare, shareCopied }: TracePanelTitleProps) {
  return (
    <div className="min-w-0">
      <h3 className="text-heading-4 text-foreground">{formatTimestamp(timestamp, true)}</h3>
      <div className="mt-1 flex items-center gap-1.5">
        <p className="min-w-0 truncate font-mono text-[11px] text-muted-foreground">{traceId}</p>
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
              <Check className="size-3 text-foreground-accent" />
            ) : (
              <Link className="size-3" />
            )}
          </button>
        )}
      </div>
    </div>
  );
}

export interface TraceNavButtonsProps {
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
}

// Prev/next trace controls (fed to SidePanel's `headerActions`), with a divider
// separating them from the panel's expand/close controls.
export function TraceNavButtons({ canGoPrev, canGoNext, onNavigate }: TraceNavButtonsProps) {
  if (!onNavigate) return null;
  return (
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
      <span aria-hidden className="mx-1 h-4 w-px bg-border" />
    </>
  );
}
