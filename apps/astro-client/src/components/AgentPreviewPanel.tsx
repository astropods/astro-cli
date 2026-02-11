import {
  Paperclip,
  ChevronDown,
  ArrowUp,
  EllipsisVertical,
} from "lucide-react";
import { Button } from "@/components/ui/button";

export interface AgentPreviewPanelProps {
  suggestedPrompts: string[];
  className?: string;
}

export function AgentPreviewPanel({
  suggestedPrompts,
  className,
}: AgentPreviewPanelProps) {
  return (
    <aside
      className={`hidden lg:flex w-[400px] shrink-0 flex-col border-l border-border bg-muted/50 ml-auto ${className ?? ""}`}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <div className="flex items-start gap-2">
          <svg
            className="size-4 text-muted-foreground mt-0.5"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <polygon points="5 3 19 12 5 21 5 3" />
          </svg>
          <div>
            <p className="text-sm font-medium leading-tight">Agent Preview</p>
            <p className="text-xs text-muted-foreground">
              Try the agent before hiring
            </p>
          </div>
        </div>
        <button className="text-muted-foreground hover:text-foreground">
          <EllipsisVertical className="size-4" />
        </button>
      </div>

      {/* Empty state + input */}
      <div className="flex flex-1 flex-col items-center justify-center px-4">
        <div className="mb-3 flex size-14 items-center justify-center rounded-xl bg-primary/10">
          <svg
            className="size-7 text-primary"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.5}
          >
            <path d="M12 2L2 7l10 5 10-5-10-5z" />
            <path d="M2 17l10 5 10-5" />
            <path d="M2 12l10 5 10-5" />
          </svg>
        </div>
        <p className="text-sm font-semibold mb-6">Ask me anything</p>

        {/* Combined input card */}
        <div className="w-full rounded-lg bg-background border border-border">
          <textarea
            rows={2}
            placeholder="Ask about incidents, connectors, or recent issues"
            className="w-full resize-none bg-transparent px-4 pt-3 pb-2 text-sm outline-none placeholder:text-muted-foreground"
          />
          <div className="flex items-center gap-2 px-3 pb-3">
            <button className="text-muted-foreground hover:text-foreground">
              <Paperclip className="size-4" />
            </button>
            <button className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground">
              Auto
              <ChevronDown className="size-3" />
            </button>
            <div className="ml-auto">
              <Button size="icon-sm" className="rounded-full">
                <ArrowUp className="size-3.5" />
              </Button>
            </div>
          </div>
        </div>

        {/* Suggested prompts */}
        {suggestedPrompts.length > 0 && (
          <div className="flex flex-wrap justify-center gap-1.5 mt-3">
            {suggestedPrompts.map((prompt) => (
              <button
                key={prompt}
                className="rounded-full border border-border bg-background px-3 py-1 text-xs hover:bg-accent transition-colors"
              >
                {prompt}
              </button>
            ))}
          </div>
        )}

        {/* Disclaimer */}
        <p className="text-xs text-muted-foreground text-center mt-3">
          This simulation uses real agent logic with fake data. No external
          systems are affected.
        </p>
      </div>
    </aside>
  );
}
