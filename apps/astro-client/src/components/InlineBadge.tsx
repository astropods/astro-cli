import { cn } from "@/lib/utils";

export interface InlineBadgeProps {
  children: React.ReactNode;
  className?: string;
  variant?: "default" | "soft";
  style?: React.CSSProperties;
}

export function InlineBadge({ children, className, variant = "default", style }: InlineBadgeProps) {
  return (
    <span
      style={style}
      className={cn(
        variant === "soft"
          ? "inline-flex items-center font-mono text-[11px] normal-case px-2 py-0.5 rounded-full border border-transparent leading-none"
          : "inline-flex items-center font-mono text-mono-sm uppercase px-2.5 py-1 rounded-full text-muted-foreground bg-stone-200 border border-border-strong dark:bg-teal-900/40 dark:border-teal-300/20 dark:text-teal-300",
        className,
      )}
    >
      {children}
    </span>
  );
}
