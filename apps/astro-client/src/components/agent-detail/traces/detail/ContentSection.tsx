import { useMemo, useState } from "react";
import { Check, ChevronRight, Copy } from "lucide-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { JsonView } from "./JsonView";

interface ParsedContent {
  /** Parsed JSON (object/array/primitive). Null when not JSON. */
  json: unknown;
  /** Plain text fallback when not JSON. */
  text: string;
  /** Plain-text representation for the copy button (always populated). */
  copyText: string;
  isJson: boolean;
  isEmpty: boolean;
}

/**
 * Decide whether the trace content is JSON or plain text.
 * Objects / arrays passed in directly are treated as JSON.
 * Strings get a JSON.parse attempt when they look JSON-shaped.
 */
function parseContent(value: unknown): ParsedContent {
  if (value == null) {
    return { json: null, text: "", copyText: "", isJson: false, isEmpty: true };
  }

  if (typeof value === "object") {
    return {
      json: value,
      text: "",
      copyText: safeStringify(value),
      isJson: true,
      isEmpty: false,
    };
  }

  const str = String(value);
  if (!str) {
    return { json: null, text: "", copyText: "", isJson: false, isEmpty: true };
  }

  const trimmed = str.trim();
  if (
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"))
  ) {
    try {
      const parsed = JSON.parse(trimmed);
      return {
        json: parsed,
        text: "",
        copyText: JSON.stringify(parsed, null, 2),
        isJson: true,
        isEmpty: false,
      };
    } catch {
      // fall through to plain text
    }
  }

  return { json: null, text: str, copyText: str, isJson: false, isEmpty: false };
}

function safeStringify(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

export interface ContentSectionProps {
  label: string;
  content: unknown;
  defaultOpen?: boolean;
  emptyText?: string;
}

export function ContentSection({
  label,
  content,
  defaultOpen = true,
  emptyText = "No content.",
}: ContentSectionProps) {
  const [open, setOpen] = useState(defaultOpen);
  const { copy, copied } = useCopyToClipboard();
  const parsed = useMemo(() => parseContent(content), [content]);

  return (
    <section className="overflow-hidden rounded-md border border-border/40">
      <div className="flex items-center gap-2 transition-colors hover:bg-muted/40">
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="flex flex-1 items-center gap-2 px-4 py-2.5 text-left"
        >
          <ChevronRight
            className={cn(
              "size-3.5 text-muted-foreground transition-transform",
              open && "rotate-90",
            )}
          />
          <span className="text-body-sm font-medium text-foreground">{label}</span>
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
          {parsed.isEmpty ? (
            <div className="border-t border-border/40 px-4 py-3">
              <p className="text-body-sm text-muted-foreground">{emptyText}</p>
            </div>
          ) : parsed.isJson ? (
            <div className="border-t border-border/40 p-3">
              <JsonView value={parsed.json} />
            </div>
          ) : (
            <div className="border-t border-border/40 px-4 py-3 [&_pre]:whitespace-pre-wrap [&_pre]:break-words [&_pre]:rounded-sm [&>div>*:first-child]:mt-0 [&>div>*:last-child]:mb-0">
              <StyledMarkdown>{parsed.text}</StyledMarkdown>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
