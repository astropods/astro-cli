import { cn } from "@/lib/utils";

export interface InlineBadgeProps {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
  variant?: "default" | "soft" | "fill";
  shape?: "pill" | "square";
}

export function InlineBadge({ children, className, style, variant = "default", shape = "pill" }: InlineBadgeProps) {
  return (
    <span
      style={style}
      className={cn(
        "inline-flex items-center font-mono text-mono-sm uppercase px-2.5 py-1",
        shape === "pill" ? "rounded-full" : "rounded",
        variant === "soft"
          ? "text-[11px] normal-case px-2 py-0.5 border border-transparent leading-none"
          : variant === "fill"
          ? "text-foreground bg-accent border-none"
          : "text-foreground border border-border-strong",
        className,
      )}
    >
      {children}
    </span>
  );
}
