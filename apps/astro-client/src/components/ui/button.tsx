import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Slot } from "radix-ui"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-[calc(var(--radius-sm)+2px)] text-sm transition-all disabled:pointer-events-none disabled:opacity-35 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground font-semibold tracking-[-0.01em] hover:bg-indigo-800 active:bg-indigo-900 dark:hover:bg-indigo-500 dark:active:bg-indigo-400",
        destructive:
          "bg-destructive text-white font-semibold tracking-[-0.01em] hover:bg-red-600 active:bg-red-800 dark:bg-red-700 dark:hover:bg-red-900 dark:active:bg-red-950 focus-visible:ring-destructive/20 dark:focus-visible:ring-destructive/40",
        outline:
          "border border-border bg-transparent text-foreground font-normal px-3.5 hover:bg-slate-300 hover:border-border-strong data-[active]:bg-accent data-[active]:text-accent-foreground dark:border-input dark:hover:bg-slate-800 dark:hover:border-slate-700",
        ghost:
          "text-foreground hover:bg-slate-100 hover:text-accent-foreground active:bg-slate-200 dark:hover:bg-slate-800 dark:active:bg-slate-900",
        link: "text-primary dark:text-indigo-400 underline decoration-primary/40 dark:decoration-indigo-400/40 underline-offset-4 hover:decoration-primary dark:hover:decoration-indigo-400",
      },
      size: {
        default: "h-10 px-5 has-[>svg]:px-4",
        xs: "h-6 gap-1 px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-8 gap-1.5 px-3 text-xs has-[>svg]:px-2.5",
        lg: "h-12 px-6 has-[>svg]:px-5",
        icon: "size-8",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-7",
        "icon-lg": "size-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
