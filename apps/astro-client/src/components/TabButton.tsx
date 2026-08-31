import type { ButtonHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

type TabButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "type"> & {
  active: boolean;
  padding?: "default" | "compact";
};

export function TabButton({
  active,
  padding = "default",
  className,
  children,
  ...props
}: TabButtonProps) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex cursor-pointer items-center border-b-2 text-body transition-colors",
        padding === "compact" ? "pb-2" : "pb-3",
        active
          ? "border-primary font-semibold text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}
