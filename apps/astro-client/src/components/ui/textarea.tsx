import * as React from "react"

import { cn } from "@/lib/utils"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground border-input w-full min-w-0 min-h-[80px] rounded-sm border bg-[var(--input-background)] px-3.5 py-2.5 text-body shadow-none transition-[color,box-shadow,border-color] outline-none resize-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:border-teal-600 focus-visible:ring-[3px] focus-visible:ring-[var(--input-focus-ring)] dark:focus-visible:border-teal-400",
        "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
