import { useState } from "react";
import { Copy, Check, ArrowUpRight, ArrowDownLeft, ChevronRight } from "lucide-react";
import { QueueListIcon, ChevronUpIcon, ChevronDownIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { InlineBadge } from "@/components/InlineBadge";
import type { TraceRow } from "./MonitorTab";
import { TRACE_STATUS_STYLE, formatLatencyMs } from "./MonitorTab";

const PANEL_SHELL_CLASS = "flex h-full w-[420px] flex-col border-l border-border bg-surface dark:bg-background";
const PANEL_HEADER_CLASS = "flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5";

interface TraceDetailPanelV2Props {
  trace: TraceRow;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
  fullPage?: boolean;
}

function SectionAccordion({
  label,
  icon,
  content,
  emptyMessage,
  defaultOpen = true,
}: {
  label: string;
  icon: React.ReactNode;
  content: string | undefined;
  emptyMessage: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!content) return;
    void navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };

  return (
    <div className="border border-border rounded-md overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2.5 px-4 py-2 text-left bg-muted/40 hover:bg-muted transition-colors cursor-pointer"
      >
        <ChevronRight
          className="size-3.5 text-muted-foreground shrink-0 transition-transform duration-200"
          style={{ transform: open ? "rotate(90deg)" : "rotate(0deg)" }}
        />
        <span className="flex items-center gap-2 text-[13px] font-semibold text-foreground">
          {icon}
          {label}
        </span>
        <span className="flex-1" />
        {content && (
          <Button
            variant="ghost"
            size="icon"
            className="size-6 text-muted-foreground shrink-0"
            onClick={handleCopy}
            aria-label={`Copy ${label.toLowerCase()}`}
          >
            {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
          </Button>
        )}
      </button>
      <div
        className="grid transition-[grid-template-rows] duration-200 ease-out"
        style={{ gridTemplateRows: open ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          <div className="border-t border-border px-4 py-3 [&>*:first-child]:mt-0">
            {content ? (
              <StyledMarkdown className="text-[13px] [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">{content}</StyledMarkdown>
            ) : (
              <span className="text-[13px] text-muted-foreground">{emptyMessage}</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export function TraceDetailPanelV2({ trace, onClose, canGoPrev, canGoNext, onNavigate, fullPage = false }: TraceDetailPanelV2Props) {
  const st = TRACE_STATUS_STYLE[trace.status];

  return (
    <div className={fullPage ? "flex flex-1 flex-col overflow-hidden" : PANEL_SHELL_CLASS}>
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
          <XMarkIcon className="size-4" />
        </Button>
      </div>

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

      <div className="flex-1 overflow-y-auto px-5 py-4 flex flex-col gap-3">
        <SectionAccordion
          label="Input"
          icon={<ArrowUpRight className="size-3.5 text-muted-foreground" />}
          content={trace.input}
          emptyMessage="—"
        />
        <SectionAccordion
          label="Output"
          icon={<ArrowDownLeft className="size-3.5 text-muted-foreground" />}
          content={trace.output}
          emptyMessage="Trace did not complete — no output recorded"
        />
      </div>
    </div>
  );
}
