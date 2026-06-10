"use client";

import { cva, type VariantProps } from "class-variance-authority";
import { forwardRef, type ComponentProps } from "react";

import { cn } from "@/lib/utils";

/** Chat/assistant-ui button styling — isolated from the app-wide Button primitive. */
const chatButtonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        outline:
          "border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
        ghost:
          "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
      },
      size: {
        icon: "size-9",
      },
    },
    defaultVariants: {
      variant: "ghost",
      size: "icon",
    },
  },
);

const ChatButton = forwardRef<
  HTMLButtonElement,
  ComponentProps<"button"> & VariantProps<typeof chatButtonVariants>
>(({ className, variant, size, type = "button", ...props }, ref) => (
  <button
    ref={ref}
    type={type}
    className={cn(chatButtonVariants({ variant, size, className }))}
    {...props}
  />
));
ChatButton.displayName = "ChatButton";

export { ChatButton, chatButtonVariants };
