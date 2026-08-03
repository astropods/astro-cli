import { useEffect, useState, type ReactNode } from "react";
import { Maximize2, Minimize2, X } from "lucide-react";
import { useIsMobile } from "@/hooks/use-compact-layout";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

interface SidePanelProps {
  /** Header content on the left (title / identity). Inline mode only. */
  title?: ReactNode;
  /** Panel-specific header controls, before the built-in expand/close. Inline mode only. */
  headerActions?: ReactNode;
  onClose?: () => void;
  closeLabel?: string;
  /** When provided, renders an expand/restore button and toggles it via `expanded`. Inline mode only. */
  onToggleExpanded?: () => void;
  expanded?: boolean;
  /** Elevation: page-level panels sit on "surface"; lifted tiles use "card". Inline mode only. */
  background?: "surface" | "card";
  /** Bottom divider under the header. Off for panels whose header flows into tabs. Inline mode only. */
  headerBorder?: boolean;
  /**
   * Docked mode. When set, the shell owns the open/close transition: it slides the
   * panel in and out on desktop, and on small screens it presents the same content
   * as a bottom sheet. `children` bring their own header. Leave unset for the inline
   * shell (a framed header row plus body) used by detail panels.
   */
  open?: boolean;
  ariaLabel?: string;
  className?: string;
  headerClassName?: string;
  children: ReactNode;
}

// Shared shell for right-side panels. Inline mode (default) owns the container
// chrome and a header row with expand/close, leaving the body to the caller
// (traces, datasets, pods). Docked mode (`open` set) owns the open/close
// animation and the small-screen bottom sheet, wrapping content that supplies
// its own header (chat).
export function SidePanel({
  title,
  headerActions,
  onClose,
  closeLabel = "Close panel",
  onToggleExpanded,
  expanded,
  background = "surface",
  headerBorder = true,
  open,
  ariaLabel,
  className,
  headerClassName,
  children,
}: SidePanelProps) {
  const isMobile = useIsMobile();
  // Two-phase mount so the docked panel animates on both open and close: mount
  // collapsed, expand on the next frame; on close, collapse then unmount after
  // the transition (which also keeps the content's queries idle while closed).
  const [mounted, setMounted] = useState(false);
  const [entered, setEntered] = useState(false);
  useEffect(() => {
    if (open === undefined) return;
    if (open) {
      setMounted(true);
      const raf = requestAnimationFrame(() => setEntered(true));
      return () => cancelAnimationFrame(raf);
    }
    setEntered(false);
    const timer = setTimeout(() => setMounted(false), 300);
    return () => clearTimeout(timer);
  }, [open]);

  if (open !== undefined) {
    if (isMobile) {
      return (
        <Sheet open={open} onOpenChange={(next) => { if (!next) onClose?.(); }}>
          <SheetContent
            side="bottom"
            showCloseButton={false}
            className="h-[min(86dvh,760px)] max-h-[calc(100dvh-0.75rem)] gap-0 overflow-hidden rounded-t-2xl border-border bg-surface p-0 shadow-2xl"
          >
            <SheetTitle className="sr-only">{ariaLabel}</SheetTitle>
            {children}
          </SheetContent>
        </Sheet>
      );
    }
    if (!mounted) return null;
    return (
      <aside
        aria-hidden={!open}
        className={cn(
          "flex shrink-0 flex-col overflow-hidden transition-[width] duration-300 ease-out motion-reduce:transition-none",
          entered ? "w-[368px]" : "w-0",
        )}
      >
        <div
          className={cn(
            "m-3.5 flex w-[340px] min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border bg-surface transition-[transform,opacity] duration-300 ease-out motion-reduce:transition-none",
            entered ? "translate-x-0 opacity-100" : "translate-x-3 opacity-0",
          )}
        >
          {children}
        </div>
      </aside>
    );
  }

  return (
    <div
      role="dialog"
      aria-label={ariaLabel}
      className={cn(
        "flex h-full w-full flex-col overflow-hidden rounded-md border border-border",
        background === "card" ? "bg-card" : "bg-surface",
        className,
      )}
    >
      <div className={cn("flex items-center gap-3 px-4 py-3", headerBorder && "border-b border-border", headerClassName)}>
        <div className="min-w-0 flex-1">{title}</div>
        <div className="flex shrink-0 items-center gap-1">
          {headerActions}
          {onToggleExpanded && (
            <button
              type="button"
              onClick={onToggleExpanded}
              aria-label={expanded ? "Restore panel size" : "Expand panel to full width"}
              className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {expanded ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
            </button>
          )}
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              aria-label={closeLabel}
              className="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              <X className="size-4" />
            </button>
          )}
        </div>
      </div>
      {children}
    </div>
  );
}
