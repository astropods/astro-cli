import * as React from "react"
import { CheckIcon, ChevronDownIcon, XIcon } from "lucide-react"
import { Select as SelectPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"
import { inputBase, inputFocusVisible, inputInvalid } from "./input"

function Select({
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Root>) {
  return <SelectPrimitive.Root data-slot="select" {...props} />
}

function SelectGroup({
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Group>) {
  return <SelectPrimitive.Group data-slot="select-group" {...props} />
}

function SelectValue({
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Value>) {
  return <SelectPrimitive.Value data-slot="select-value" {...props} />
}

function SelectTrigger({
  className,
  children,
  icon,
  onClear,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Trigger> & {
  icon?: React.ReactElement
  onClear?: () => void
}) {
  const trigger = (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      className={cn(
        "data-[placeholder]:text-muted-foreground text-foreground flex h-11 w-full items-center justify-between hover:border-border-strong [&>span]:line-clamp-1",
        inputBase,
        inputFocusVisible,
        inputInvalid,
        className,
        onClear && "pr-3.5 [&>span]:max-w-[calc(100%-2.5rem)]"
      )}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        {icon ?? <ChevronDownIcon className="size-4 opacity-50" />}
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  )

  if (!onClear) {
    return trigger
  }

  const triggerLabel = props["aria-label"]
  const clearLabel = triggerLabel ? `Clear ${triggerLabel}` : "Clear selection"

  return (
    <div className="inline-flex w-full items-center">
      {trigger}
      <button
        type="button"
        aria-label={clearLabel}
        onClick={onClear}
        className="-ml-12 rounded-sm p-0.5 text-faint-foreground/60 transition-colors hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        <XIcon aria-hidden className="size-3" />
      </button>
    </div>
  )
}

function SelectContent({
  className,
  children,
  footer,
  position = "popper",
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Content> & { footer?: React.ReactNode }) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        data-slot="select-content"
        className={cn(
          "bg-popover text-popover-foreground data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 relative z-50 max-h-96 min-w-[8rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-md border shadow-md",
          position === "popper" &&
            "data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1",
          className
        )}
        position={position}
        {...props}
      >
        <SelectPrimitive.Viewport
          className={cn(
            "max-w-[calc(100vw-2rem)] p-1",
            position === "popper" &&
              "h-[var(--radix-select-trigger-height)] w-full min-w-[min(var(--radix-select-trigger-width),calc(100vw-2rem))]"
          )}
        >
          {children}
        </SelectPrimitive.Viewport>
        {footer}
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  )
}

function SelectLabel({
  className,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Label>) {
  return (
    <SelectPrimitive.Label
      data-slot="select-label"
      className={cn("px-2 py-1.5 text-sm font-medium", className)}
      {...props}
    />
  )
}

function SelectItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        "focus:bg-accent focus:text-accent-foreground data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-2 pl-8 text-sm outline-hidden select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        className
      )}
      {...props}
    >
      <span className="pointer-events-none absolute left-2 flex size-3.5 items-center justify-center text-foreground-accent">
        <SelectPrimitive.ItemIndicator>
          <CheckIcon className="size-4" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  )
}

export {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
}
