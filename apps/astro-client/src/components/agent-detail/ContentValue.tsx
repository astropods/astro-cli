import { useMemo, type ReactNode } from "react";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { cn } from "@/lib/utils";
import {
  parseContent,
  type ParsedContent,
} from "@/lib/content-parse";
import { JsonView } from "./JsonView";

export type ContentValueMode = "pretty" | "raw";
export type ContentValueTone = "foreground" | "muted";

interface ContentValueProps {
  content?: unknown;
  parsed?: ParsedContent;
  mode?: ContentValueMode;
  tone?: ContentValueTone;
  emptyFallback?: ReactNode;
  className?: string;
}

export function ContentValue({
  content,
  parsed: parsedProp,
  mode = "pretty",
  tone = "foreground",
  emptyFallback = <span className="text-faint-foreground">—</span>,
  className,
}: ContentValueProps) {
  const parsed = useMemo(
    () => parsedProp ?? parseContent(content),
    [content, parsedProp],
  );
  const textClass =
    tone === "foreground" ? "text-foreground" : "text-muted-foreground";

  if (parsed.isEmpty) return emptyFallback;

  if (mode === "raw") {
    return (
      <pre
        className={cn(
          "m-0 whitespace-pre-wrap break-words py-3 font-mono text-mono-sm leading-relaxed",
          textClass,
          className,
        )}
      >
        {parsed.copyText}
      </pre>
    );
  }

  if (parsed.isJson) {
    return <JsonView value={parsed.json} className={className} />;
  }

  return (
    <div
      className={cn(
        "[&>*:first-child]:mt-0 [&>*:last-child]:mb-0 [&_pre]:whitespace-pre-wrap [&_pre]:break-words",
        textClass,
        className,
      )}
    >
      <StyledMarkdown>{parsed.text}</StyledMarkdown>
    </div>
  );
}
