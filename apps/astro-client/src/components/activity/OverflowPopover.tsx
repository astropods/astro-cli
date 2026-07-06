import { useState, type ReactNode } from "react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";

// Shared popover shell for "+N" overflow chips in Insights table cells.
// Owns trigger styling, popover chrome, and the count label; callers render
// the scrollable list body.

interface OverflowPopoverProps {
  /** The number of items hidden behind the chip — displayed as `+N`. */
  overflow: number;
  /** Total count rendered as the popover header (e.g. "8 people"). */
  total: number;
  /** Singular / plural noun used to build the header label and aria text. */
  itemNoun: { singular: string; plural: string };
  /** List body — typically a `<ul>` element with `<li>` children. */
  children: ReactNode;
  trigger?: "click" | "hover";
  className?: string;
}

export function OverflowPopover({
  overflow,
  total,
  itemNoun,
  children,
  trigger = "click",
  className,
}: OverflowPopoverProps) {
  const noun = total === 1 ? itemNoun.singular : itemNoun.plural;
  const hover = trigger === "hover";
  const [open, setOpen] = useState(false);

  const hoverProps = hover
    ? { onMouseEnter: () => setOpen(true), onMouseLeave: () => setOpen(false) }
    : {};

  return (
    <PopoverPrimitive.Root
      open={hover ? open : undefined}
      onOpenChange={hover ? setOpen : undefined}
    >
      <PopoverPrimitive.Trigger asChild>
        <button
          type="button"
          className="cursor-pointer rounded font-mono text-mono-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          aria-label={`Show ${total} ${noun}`}
          {...hoverProps}
        >
          +{overflow}
        </button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          side="right"
          align="start"
          sideOffset={8}
          collisionPadding={16}
          onOpenAutoFocus={hover ? (e) => e.preventDefault() : undefined}
          onCloseAutoFocus={hover ? (e) => e.preventDefault() : undefined}
          className={cn(
            "z-50 flex max-h-[min(400px,var(--radix-popover-content-available-height))] w-64 max-w-xs flex-col rounded-md border border-border bg-popover p-2 shadow-md",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            hover && "data-[state=closed]:pointer-events-none",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
            "data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
            className,
          )}
          {...hoverProps}
        >
          <p className="shrink-0 px-2 pb-1.5 text-mono-sm text-faint-foreground">
            {total} {noun}
          </p>
          {children}
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
