import { cn } from "@/lib/utils";

interface SpinnerProps {
  size?: number;
  className?: string;
}

export function Spinner({ size = 20, className }: SpinnerProps) {
  return (
    <div
      className={cn("dp-spin", className)}
      style={{
        width: size,
        height: size,
        borderRadius: "50%",
        border: "2px solid var(--border)",
        borderTopColor: "var(--foreground)",
        flexShrink: 0,
      }}
    />
  );
}
