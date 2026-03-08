import * as React from "react"

import { cn } from "@/lib/utils"

/** Base visual styles shared by all input-like elements. */
export const inputBase =
  "border-input rounded-sm border bg-[var(--input-background)] px-3.5 text-body shadow-none transition-[color,box-shadow,border-color] outline-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"

/** Focus ring for native focusable elements (input, textarea, select). */
export const inputFocusVisible =
  "focus-visible:border-teal-600 focus-visible:ring-[3px] focus-visible:ring-[var(--input-focus-ring)] dark:focus-visible:border-teal-400"

/** Focus ring for container elements that wrap a bare <input>. */
export const inputFocusWithin =
  "focus-within:border-teal-600 focus-within:ring-[3px] focus-within:ring-[var(--input-focus-ring)] dark:focus-within:border-teal-400"

/** Validation ring for aria-invalid elements. */
export const inputInvalid =
  "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive"

function Input({
  className,
  type,
  variant = "default",
  ...props
}: React.ComponentProps<"input"> & {
  variant?: "default" | "code"
}) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground flex h-11 w-full min-w-0 file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium",
        inputBase,
        inputFocusVisible,
        inputInvalid,
        variant === "code" && "font-mono text-mono-md",
        className
      )}
      {...props}
    />
  )
}

export { Input }
