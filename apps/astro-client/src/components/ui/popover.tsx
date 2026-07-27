import * as React from "react"
import { Popover as PopoverPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"

function Popover({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

function PopoverContent({
  className,
  variant = "tooltip",
  side = "top",
  align = "center",
  sideOffset = 6,
  collisionPadding = 16,
  children,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content> & {
  variant?: "tooltip" | "panel"
}) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        side={side}
        align={align}
        sideOffset={sideOffset}
        collisionPadding={collisionPadding}
        className={cn(
          "z-50 w-fit max-w-[var(--radix-popover-content-available-width)] shadow-md",
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
          variant === "tooltip"
            ? "bg-stone-800 text-stone-50 dark:bg-stone-200 dark:text-stone-900 rounded px-2.5 py-1.5 text-xs text-balance"
            // Panel styles are intentionally complete so tooltip defaults never
            // leak into interactive popover surfaces.
            : "rounded-md border border-border bg-popover p-0 text-popover-foreground dark:bg-popover dark:text-popover-foreground",
          className,
        )}
        {...props}
      >
        {children}
      </PopoverPrimitive.Content>
    </PopoverPrimitive.Portal>
  )
}

export { Popover, PopoverTrigger, PopoverContent }
