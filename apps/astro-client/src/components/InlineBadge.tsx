import { cn } from "@/lib/utils";

export interface InlineBadgeProps {
  children: React.ReactNode;
  className?: string;
}

export function InlineBadge({ children, className }: InlineBadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center font-mono text-[10px] tracking-wide px-2.5 py-0.5 rounded-full",
        "text-muted-foreground bg-stone-200 border border-border-strong",
        "dark:bg-teal-900/40 dark:border-teal-300/20 dark:text-teal-300",
        className,
      )}
    >
      {children}
    </span>
  );
}
