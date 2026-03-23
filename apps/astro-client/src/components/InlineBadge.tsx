import { cn } from "@/lib/utils";

export interface InlineBadgeProps {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
}

export function InlineBadge({ children, className, style }: InlineBadgeProps) {
  return (
    <span
      style={style}
      className={cn(
        "inline-flex items-center font-mono text-mono-sm uppercase px-2.5 py-1 rounded-full",
        "text-muted-foreground bg-stone-200 border border-border-strong",
        "dark:bg-teal-900/40 dark:border-teal-300/20 dark:text-teal-300",
        className,
      )}
    >
      {children}
    </span>
  );
}
