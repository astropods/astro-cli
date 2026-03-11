import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const labelVariants = cva(
  "font-mono uppercase text-muted-foreground",
  {
    variants: {
      size: {
        sm: "text-mono-sm",
        md: "mb-1 block text-[13px] font-sans normal-case font-semibold text-foreground",
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
