import { cn } from "@/lib/utils";

export type StatusIndicatorVariant = "success" | "pending" | "muted" | "warning" | "error";

export interface StatusIndicatorProps {
  variant?: StatusIndicatorVariant;
  pulse?: boolean;
  children: React.ReactNode;
  className?: string;
}

const variantStyles: Record<StatusIndicatorVariant, { dot: string; text: string }> = {
  success: {
    dot: "bg-teal-500 dark:bg-teal-300",
    text: "text-teal-500 dark:text-teal-300",
  },
  pending: {
    dot: "bg-teal-700 dark:bg-teal-600",
    text: "text-teal-700 dark:text-teal-600",
  },
  muted: {
    dot: "bg-stone-600 dark:bg-stone-400",
    text: "text-stone-600 dark:text-stone-400",
  },
  warning: {
    dot: "bg-yellow-500 dark:bg-yellow-400",
    text: "text-yellow-500 dark:text-yellow-400",
  },
  error: {
    dot: "bg-coral-600 dark:bg-coral-400",
    text: "text-coral-600 dark:text-coral-400",
  },
};

export function StatusIndicator({
  variant = "muted",
  pulse = false,
  children,
  className,
}: StatusIndicatorProps) {
  const styles = variantStyles[variant];

  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <span
        className={cn(
          "size-2 shrink-0 rounded-full",
          styles.dot,
          pulse && "animate-pulse",
        )}
      />
      <span
        className={cn(
          "text-mono-md font-mono uppercase",
          styles.text,
        )}
      >
        {children}
      </span>
    </span>
  );
}
