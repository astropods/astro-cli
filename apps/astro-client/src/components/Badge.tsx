import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type BadgeVariant =
  | "default"
  | "active"
  | "pending"
  | "inactive"
  | "warning"
  | "error"
  | "info";

export interface BadgeProps {
  variant?: BadgeVariant;
  showDot?: boolean;
  children: ReactNode;
  className?: string;
}

const variantStyles: Record<BadgeVariant, { dot: string; badge: string }> = {
  default: {
    dot: "bg-gray-600 border-[3px] border-gray-300",
    badge: "bg-white/10 text-foreground border border-border",
  },
  active: {
    dot: "bg-green-700 border-[3px] border-green-200",
    badge: "bg-green-50 text-green-700 border border-green-700",
  },
  pending: {
    dot: "bg-amber-700 border-[3px] border-amber-200",
    badge: "bg-amber-50 text-amber-700 border border-amber-700",
  },
  inactive: {
    dot: "bg-gray-600 border-[3px] border-gray-300",
    badge: "bg-gray-100 text-gray-600 border border-gray-600",
  },
  warning: {
    dot: "bg-yellow-700 border-[3px] border-yellow-200",
    badge: "bg-yellow-50 text-yellow-700 border border-yellow-700",
  },
  error: {
    dot: "bg-red-700 border-[3px] border-red-200",
    badge: "bg-red-50 text-red-700 border border-red-700",
  },
  info: {
    dot: "bg-blue-700 border-[3px] border-blue-200",
    badge: "bg-blue-50 text-blue-700 border border-blue-700",
  },
};

export function Badge({ variant = "default", showDot = false, children, className }: BadgeProps) {
  const styles = variantStyles[variant];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full py-0.5 pr-2 text-xs font-medium",
        showDot ? "pl-1.5" : "pl-2",
        styles.badge,
        className,
      )}
    >
      {showDot && (
        <span className={cn("size-3 shrink-0 rounded-full", styles.dot)} />
      )}
      {children}
    </span>
  );
}
