import { cn } from "@/lib/utils";

export interface InlineBadgeProps {
  children: React.ReactNode;
  className?: string;
}

export function InlineBadge({ children, className }: InlineBadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center text-mono-sm font-mono uppercase px-2 py-0.5",
        "text-teal-800 border border-teal-800",
        "dark:text-teal-300 dark:border-teal-300/30",
        className,
      )}
    >
      {children}
    </span>
  );
}
