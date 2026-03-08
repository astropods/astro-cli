import * as React from "react"

import { cn } from "@/lib/utils"

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
        "file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground border-input flex h-11 w-full min-w-0 rounded-sm border bg-[var(--input-background)] px-3.5 text-body shadow-none transition-[color,box-shadow,border-color] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:border-teal-600 focus-visible:ring-[3px] focus-visible:ring-[var(--input-focus-ring)] dark:focus-visible:border-teal-400",
        "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
        variant === "code" && "font-mono text-mono-md",
        className
      )}
      {...props}
    />
  )
}

export { Input }
