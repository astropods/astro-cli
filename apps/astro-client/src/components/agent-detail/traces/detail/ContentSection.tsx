import type {
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
  ReactNode,
} from "react";
import { useMemo, useRef, useState } from "react";
import { Check, ChevronRight, Copy } from "lucide-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { parseContent } from "@/lib/content-parse";
import { ContentValue } from "@/components/agent-detail/ContentValue";
import { ResizeCornerHandle } from "@/components/ui/resize-corner-handle";

const RESIZE_MIN_HEIGHT = 128;
const RESIZE_KEYBOARD_STEP = 24;

export interface ContentSectionProps {
  label: string;
  content: unknown;
  icon?: ReactNode;
  mode?: "pretty" | "raw";
  defaultOpen?: boolean;
  emptyText?: string;
  contentClassName?: string;
  resizableContent?: boolean;
}

export function ContentSection({
  label,
  content,
  icon,
  mode = "pretty",
  defaultOpen = true,
  emptyText = "No content.",
  contentClassName,
  resizableContent = false,
}: ContentSectionProps) {
  const [open, setOpen] = useState(defaultOpen);
  const [contentHeight, setContentHeight] = useState<number | null>(
    resizableContent ? RESIZE_MIN_HEIGHT : null,
  );
  const contentRef = useRef<HTMLDivElement>(null);
  const { copy, copied } = useCopyToClipboard();
  const parsed = useMemo(() => parseContent(content), [content]);
  const resizeMaxHeight =
    typeof window === "undefined" ? 720 : Math.max(192, window.innerHeight * 0.7);
  const resizeMinHeight = RESIZE_MIN_HEIGHT;
  const resizeMaxHeightClamped = Math.max(resizeMinHeight, resizeMaxHeight);

  function clampContentHeight(height: number) {
    return Math.max(
      resizeMinHeight,
      Math.min(resizeMaxHeightClamped, height),
    );
  }

  function handleResizeStart(event: ReactPointerEvent<HTMLButtonElement>) {
    const node = contentRef.current;
    if (!node) return;

    event.preventDefault();

    const startY = event.clientY;
    const startHeight = node.getBoundingClientRect().height;
    const ownerWindow = node.ownerDocument.defaultView ?? window;

    setContentHeight(startHeight);

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const nextHeight = clampContentHeight(
        startHeight + moveEvent.clientY - startY,
      );
      setContentHeight(nextHeight);
    };

    const stopResize = () => {
      ownerWindow.removeEventListener("pointermove", handlePointerMove);
      ownerWindow.removeEventListener("pointerup", stopResize);
      ownerWindow.removeEventListener("pointercancel", stopResize);
    };

    ownerWindow.addEventListener("pointermove", handlePointerMove);
    ownerWindow.addEventListener("pointerup", stopResize);
    ownerWindow.addEventListener("pointercancel", stopResize);
  }

  function handleResizeKeyDown(event: ReactKeyboardEvent<HTMLButtonElement>) {
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;

    const node = contentRef.current;
    if (!node) return;

    event.preventDefault();
    const direction = event.key === "ArrowDown" ? 1 : -1;
    const measuredHeight = node.getBoundingClientRect().height;
    setContentHeight((current) =>
      clampContentHeight(
        (current ?? measuredHeight) + direction * RESIZE_KEYBOARD_STEP,
      ),
    );
  }

  return (
    <section className="overflow-hidden rounded-md border border-border/70">
      <div
        className={cn(
          "flex items-center gap-2 transition-colors hover:bg-muted/45 dark:hover:bg-foreground/5",
          open && "bg-muted/40 dark:bg-foreground/5",
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
                <Check className="size-3 text-foreground-accent" />
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
              resizableContent && "relative",
            )}
          >
            <div
              ref={contentRef}
              className={cn(
                resizableContent && "min-h-32 overflow-y-auto",
                parsed.isJson && !parsed.isEmpty && mode !== "raw" ? "p-3" : "px-4 py-3",
                contentClassName,
              )}
              style={
                resizableContent && contentHeight != null
                  ? { height: contentHeight, maxHeight: "70dvh" }
                  : undefined
              }
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
            {resizableContent && (
              <button
                type="button"
                aria-label={`Resize ${label} content`}
                onPointerDown={handleResizeStart}
                onKeyDown={handleResizeKeyDown}
                className="absolute bottom-0 right-0 z-10 size-3 cursor-ns-resize touch-none"
              >
                <ResizeCornerHandle className="absolute bottom-px right-px" />
              </button>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
