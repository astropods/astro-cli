import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const labelVariants = cva(
  "font-mono uppercase text-muted-foreground",
  {
    variants: {
      size: {
        sm: "text-mono-sm",
        md: "mb-1.5 block text-mono-md tracking-widest text-ink-muted",
      },
    },
    defaultVariants: {
      size: "sm",
    },
  }
)

function Label({
  className,
  size,
  ...props
}: React.ComponentProps<"label"> & VariantProps<typeof labelVariants>) {
  return (
    <label
      data-slot="label"
      className={cn(labelVariants({ size }), className)}
      {...props}
    />
  )
}

export { Label }
