import { useState } from "react";
import { X, Copy, Check } from "lucide-react";
import { QueueListIcon, ChevronUpIcon, ChevronDownIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { InlineBadge } from "@/components/InlineBadge";
import type { TraceRow, TraceStatus } from "./MonitorTab";

const PANEL_SHELL_CLASS = "flex h-full w-[420px] flex-col border-l border-border bg-surface dark:bg-background";
const PANEL_HEADER_CLASS = "flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5";

const STATUS_STYLE: Record<TraceStatus, { label: string; badgeStyle: React.CSSProperties }> = {
  success: {
    label: "Success",
    badgeStyle: { color: "var(--color-teal-600)", background: "color-mix(in oklch, var(--color-teal-600) 12%, transparent)" },
  },
  error: {
    label: "Error",
    badgeStyle: { color: "var(--color-red-700)", background: "color-mix(in oklch, var(--color-red-700) 12%, transparent)" },
  },
  timeout: {
    label: "Timeout",
    badgeStyle: { color: "var(--color-yellow-700)", background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)" },
  },
};

function formatLatencyMs(ms: number): string {
  if (!Number.isFinite(ms)) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

interface TraceDetailPanelProps {
  trace: TraceRow;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
  fullPage?: boolean;
}

export function TraceDetailPanel({ trace, onClose, canGoPrev, canGoNext, onNavigate, fullPage = false }: TraceDetailPanelProps) {
  const [tab, setTab] = useState<"input" | "output">("input");
  const [copied, setCopied] = useState(false);
  const st = STATUS_STYLE[trace.status];
  const activeContent = tab === "input" ? (trace.input ?? "") : (trace.output ?? "");

  const handleCopy = () => {
    if (!activeContent) return;
    void navigator.clipboard.writeText(activeContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };

  return (
    <div className={fullPage ? "flex flex-1 flex-col overflow-hidden" : PANEL_SHELL_CLASS}>
      {/* Header — matches ConfigurePanel pattern */}
      <div className={PANEL_HEADER_CLASS}>
        <QueueListIcon className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-heading-4 font-semibold text-foreground">Traces</span>
        <Button
          variant="ghost"
          size="icon"
          className="size-7 shrink-0"
          disabled={!canGoPrev}
          onClick={() => onNavigate?.("prev")}
          aria-label="Previous trace"
        >
          <ChevronUpIcon className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-7 shrink-0"
          disabled={!canGoNext}
          onClick={() => onNavigate?.("next")}
          aria-label="Next trace"
        >
          <ChevronDownIcon className="size-4" />
        </Button>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>

      {/* Metadata strip — two rows */}
      <div className="flex shrink-0 flex-col gap-1 border-b border-border px-5 py-3">
        <div className="flex items-center justify-between gap-2">
          <span className="font-mono text-[13px] text-foreground">{trace.time}</span>
          <InlineBadge variant="soft" style={st.badgeStyle}>{st.label}</InlineBadge>
        </div>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[12px] text-muted-foreground">{formatLatencyMs(trace.latency)}</span>
          <span className="text-muted-foreground text-[11px]">·</span>
          <span className="font-mono text-[12px] text-muted-foreground">{trace.tokens > 0 ? `${trace.tokens.toLocaleString()} tokens` : "—"}</span>
        </div>
      </div>

      {/* Tab bar */}
      {/* Tab bar */}
      <div className="flex shrink-0 border-b border-border px-5">
        {(["input", "output"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            style={{
              fontFamily: "var(--font-sans), sans-serif",
              fontSize: 13,
              fontWeight: tab === t ? 600 : 400,
              color: tab === t ? "var(--foreground)" : "var(--faint-foreground)",
              background: "none",
              border: "none",
              borderBottom: tab === t ? "2px solid var(--color-teal-600)" : "2px solid transparent",
              padding: "10px 0",
              marginRight: 16,
              cursor: "pointer",
              transition: "color 0.15s",
            }}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="relative flex-1 overflow-y-auto px-5 py-4">
        {activeContent && (
          <button
            type="button"
            onClick={handleCopy}
            className="absolute right-4 top-4 text-muted-foreground hover:text-foreground transition-colors"
            style={{ background: "none", border: "none", padding: 2, display: "flex", cursor: "pointer" }}
            aria-label={`Copy ${tab}`}
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
          </button>
        )}
        {tab === "input" ? (
          trace.input ? (
            <StyledMarkdown className="text-[13px]">{trace.input}</StyledMarkdown>
          ) : (
            <span className="font-sans text-[13px] text-muted-foreground">—</span>
          )
        ) : trace.output ? (
          <StyledMarkdown className="text-[13px]">{trace.output}</StyledMarkdown>
        ) : (
          <span className="font-mono text-[13px] text-[var(--color-coral-600)]">
            Trace did not complete — no output recorded
          </span>
        )}
      </div>
    </div>
  );
}
