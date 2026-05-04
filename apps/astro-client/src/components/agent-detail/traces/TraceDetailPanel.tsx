import { useState } from "react";
import { X, ChevronUp, ChevronDown, ChevronRight, Copy, Check } from "lucide-react";
import { cn } from "@/lib/utils";

import { StyledMarkdown } from "@/components/StyledMarkdown";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { TraceEntry } from "@/lib/api";
import { STATUS_CONFIG, normalizeStatus, formatTimestamp, formatLatency, formatCost } from "./trace-utils";

/**
 * Normalize trace content into a markdown-friendly string.
 * Objects and JSON strings get wrapped in a fenced code block;
 * plain text / markdown passes through as-is.
 */
function formatContent(value: unknown): string {
  if (value == null) return "";

  // Already an object — pretty-print as JSON code block
  if (typeof value === "object") {
    try {
      return "```json\n" + JSON.stringify(value, null, 2) + "\n```";
    } catch {
      return String(value);
    }
  }

  const str = String(value);
  if (!str) return "";

  // Try to detect JSON strings and format them
  const trimmed = str.trim();
  if (
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"))
  ) {
    try {
      const parsed = JSON.parse(trimmed);
      return "```json\n" + JSON.stringify(parsed, null, 2) + "\n```";
    } catch {
      // Not valid JSON, fall through
    }
  }

  return str;
}

// ---------------------------------------------------------------------------
// Collapsible section
// ---------------------------------------------------------------------------

function ContentSection({
  label,
  content,
  defaultOpen = true,
}: {
  label: string;
  content: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const { copy, copied } = useCopyToClipboard();
  const text = formatContent(content);

  return (
    <section className="overflow-hidden rounded-md border border-border/40">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-4 py-2.5 text-left transition-colors hover:bg-white/3"
      >
        <ChevronRight
          className={cn(
            "size-3.5 text-muted-foreground transition-transform",
            open && "rotate-90",
          )}
        />
        <span className="text-body-sm font-medium text-foreground">{label}</span>
        {text && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              void copy(text);
            }}
            className="ml-auto flex items-center gap-1 rounded px-1.5 py-0.5 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            {copied ? (
              <>
                <Check className="size-3 text-primary" />
                Copied
              </>
            ) : (
              <>
                <Copy className="size-3" />
                Copy
              </>
            )}
          </button>
        )}
      </button>

      <div
        className="grid transition-[grid-template-rows] duration-200 ease-out"
        style={{ gridTemplateRows: open ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          {text ? (
            <div className="border-t border-border/40 px-4 py-3 [&_pre]:whitespace-pre-wrap [&_pre]:break-words [&_pre]:rounded-sm [&>div>*:first-child]:mt-0 [&>div>*:last-child]:mb-0">
              <StyledMarkdown>{text}</StyledMarkdown>
            </div>
          ) : (
            <div className="border-t border-border/40 px-4 py-3">
              <p className="text-body-sm text-muted-foreground">No content.</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------

export interface TraceDetailPanelProps {
  trace: TraceEntry;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
}

export function TraceDetailPanel({
  trace,
  onClose,
  canGoPrev,
  canGoNext,
  onNavigate,
}: TraceDetailPanelProps) {
  const status = normalizeStatus(trace.status);
  const cfg = STATUS_CONFIG[status];

  return (
    <div
      role="dialog"
      aria-label="Trace details"
      className="flex h-full w-full flex-col overflow-hidden rounded-md border border-border bg-surface"
    >
      {/* Header */}
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <div className="min-w-0 flex-1">
          <h3 className="text-heading-4 text-foreground">
            {formatTimestamp(trace.timestamp, true)}
          </h3>
          <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground/40">
            {trace.trace_id}
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {onNavigate && (
            <>
              <button
                onClick={() => onNavigate("prev")}
                disabled={!canGoPrev}
                aria-label="Previous trace"
                className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
              >
                <ChevronUp className="size-4" />
              </button>
              <button
                onClick={() => onNavigate("next")}
                disabled={!canGoNext}
                aria-label="Next trace"
                className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
              >
                <ChevronDown className="size-4" />
              </button>
            </>
          )}
          <button
            onClick={onClose}
            aria-label="Close trace"
            className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        </div>
      </div>

      {/* Metadata */}
      <div className="border-b border-border px-4 py-3">
        <div className="grid grid-cols-3 gap-3">
          <div className="flex flex-col items-start gap-1">
            <span className="text-mono-sm text-muted-foreground/60">Status</span>
            <span
              className="inline-flex items-center gap-[5px] rounded border pl-[6px] pr-[10px] py-1 font-mono text-label font-normal tracking-[0.06em]"
              style={{ background: cfg.bg, borderColor: cfg.bdr, color: cfg.fg }}
            >
              {cfg.label}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-mono-sm text-muted-foreground/60">Latency</span>
            <span className="font-mono text-body-sm text-foreground">{formatLatency(trace.latency_ms)}</span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-mono-sm text-muted-foreground/60">Cost</span>
            <span className="font-mono text-body-sm text-foreground">{formatCost(trace.total_cost)}</span>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="flex flex-col gap-3">
          <ContentSection label="Input" content={trace.input} />
          <ContentSection label="Output" content={trace.output} />
        </div>
      </div>
    </div>
  );
}
