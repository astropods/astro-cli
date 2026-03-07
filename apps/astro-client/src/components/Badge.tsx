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

export type BadgeSize = "default" | "lg";

export interface BadgeProps {
  variant?: BadgeVariant;
  size?: BadgeSize;
  showDot?: boolean;
  icon?: ReactNode;
  children: ReactNode;
  className?: string;
}

const variantStyles: Record<BadgeVariant, { dot: string; badge: string }> = {
  default: {
    dot: "bg-stone-600 border-[3px] border-stone-300",
    badge: "bg-white/10 text-muted-foreground border border-muted-foreground",
  },
  active: {
    dot: "bg-green-700 border-[3px] border-green-200 dark:bg-green-400 dark:border-green-800",
    badge: "bg-green-50 text-green-700 border border-green-700 dark:bg-green-950 dark:text-green-300 dark:border-green-700",
  },
  pending: {
    dot: "bg-amber-700 border-[3px] border-amber-200 dark:bg-amber-400 dark:border-amber-800",
    badge: "bg-amber-50 text-amber-700 border border-amber-700 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-700",
  },
  inactive: {
    dot: "bg-stone-600 border-[3px] border-stone-300 dark:bg-stone-400 dark:border-stone-700",
    badge: "bg-stone-100 text-stone-600 border border-stone-600 dark:bg-stone-900 dark:text-stone-300 dark:border-stone-600",
  },
  warning: {
    dot: "bg-yellow-700 border-[3px] border-yellow-200 dark:bg-yellow-400 dark:border-yellow-800",
    badge: "bg-yellow-50 text-yellow-700 border border-yellow-700 dark:bg-yellow-950 dark:text-yellow-300 dark:border-yellow-700",
  },
  error: {
    dot: "bg-red-700 border-[3px] border-red-200 dark:bg-red-400 dark:border-red-800",
    badge: "bg-red-50 text-red-700 border border-red-700 dark:bg-red-950 dark:text-red-300 dark:border-red-700",
  },
  info: {
    dot: "bg-blue-700 border-[3px] border-blue-200 dark:bg-blue-400 dark:border-blue-800",
    badge: "bg-blue-50 text-blue-700 border border-blue-700 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-700",
  },
};

const sizeStyles: Record<BadgeSize, { badge: string; dot: string; icon: string }> = {
  default: {
    badge: "gap-1.5 rounded-full py-0.5 pr-2",
    dot: "size-3",
    icon: "size-3.5",
  },
  lg: {
    badge: "gap-2 rounded-lg py-1.5 pr-3",
    dot: "size-3",
    icon: "size-4",
  },
};

export function Badge({ variant = "default", size = "default", showDot = false, icon, children, className }: BadgeProps) {
  const varStyles = variantStyles[variant];
  const sz = sizeStyles[size];
  const hasLeadingElement = showDot || icon;

  return (
    <span
      className={cn(
        "inline-flex items-center font-mono font-normal text-[12px]",
        sz.badge,
        hasLeadingElement ? (size === "lg" ? "pl-2.5" : "pl-1.5") : (size === "lg" ? "pl-3" : "pl-2"),
        varStyles.badge,
        className,
      )}
    >
      {showDot && (
        <span className={cn("shrink-0 rounded-full", sz.dot, varStyles.dot)} />
      )}
      {icon && <span className={cn("shrink-0 [&>svg]:size-full", sz.icon)}>{icon}</span>}
      {children}
    </span>
  );
}
