"use client";

import { type VariantProps } from "class-variance-authority";
import { forwardRef, type ComponentProps } from "react";

import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type ChatButtonVariantProps = VariantProps<typeof buttonVariants> & {
  className?: string;
};

/** Assistant-ui defaults, backed by the app-wide Button primitive styles. */
function chatButtonVariants({
  variant = "ghost",
  size = "icon",
  className,
}: ChatButtonVariantProps = {}) {
  return cn(buttonVariants({ variant, size }), className);
}

const ChatButton = forwardRef<
  HTMLButtonElement,
  ComponentProps<"button"> & VariantProps<typeof buttonVariants>
>(({ className, variant, size, type = "button", ...props }, ref) => (
  <button
    ref={ref}
    type={type}
    className={chatButtonVariants({ variant, size, className })}
    {...props}
  />
));
ChatButton.displayName = "ChatButton";

export { ChatButton, chatButtonVariants };
