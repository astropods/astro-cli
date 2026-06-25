import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { Check, ChevronRight, Copy } from "lucide-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { parseContent } from "@/lib/content-parse";
import { ContentValue } from "@/components/agent-detail/ContentValue";

export interface ContentSectionProps {
  label: string;
  content: unknown;
  icon?: ReactNode;
  mode?: "pretty" | "raw";
  defaultOpen?: boolean;
  emptyText?: string;
}

export function ContentSection({
  label,
  content,
  icon,
  mode = "pretty",
  defaultOpen = true,
  emptyText = "No content.",
}: ContentSectionProps) {
  const [open, setOpen] = useState(defaultOpen);
  const { copy, copied } = useCopyToClipboard();
  const parsed = useMemo(() => parseContent(content), [content]);

  return (
    <section className="overflow-hidden rounded-md border border-border/70">
      <div
        className={cn(
          "flex items-center gap-2 transition-colors hover:bg-muted/40",
          open && "bg-muted/30",
        )}
      >
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center gap-2 px-4 py-2.5 text-left"
        >
          <ChevronRight
            className={cn(
              "size-3.5 text-muted-foreground transition-transform",
              open && "rotate-90",
            )}
          />
          {icon && <span className="flex flex-none items-center justify-center">{icon}</span>}
          <span className="truncate text-body-sm font-medium text-foreground">{label}</span>
        </button>
        {!parsed.isEmpty && (
          <button
            type="button"
            onClick={() => void copy(parsed.copyText)}
            className="mr-2 flex items-center gap-1 rounded px-1.5 py-0.5 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
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
      </div>

      <div
        className="grid transition-[grid-template-rows] duration-200 ease-out"
        style={{ gridTemplateRows: open ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          <div
            className={cn(
              "border-t border-border/70",
              parsed.isJson && !parsed.isEmpty && mode !== "raw" ? "p-3" : "px-4 py-3",
            )}
          >
            <ContentValue
              parsed={parsed}
              mode={mode}
              className={!parsed.isJson ? "[&_pre]:rounded-sm" : undefined}
              emptyFallback={
                <p className="text-body-sm text-muted-foreground">
                  {emptyText}
                </p>
              }
            />
          </div>
        </div>
      </div>
    </section>
  );
}
