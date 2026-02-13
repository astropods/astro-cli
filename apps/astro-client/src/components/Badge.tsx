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
    dot: "bg-stone-600 border-[3px] border-stone-300",
    badge: "bg-stone-100 text-stone-600 border border-stone-600",
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

const sizeStyles: Record<BadgeSize, { badge: string; dot: string; icon: string }> = {
  default: {
    badge: "gap-1.5 rounded-full py-0.5 pr-2 text-xs",
    dot: "size-3",
    icon: "size-3.5",
  },
  lg: {
    badge: "gap-2 rounded-lg py-1.5 pr-3 text-sm",
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
        "inline-flex items-center font-medium",
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
