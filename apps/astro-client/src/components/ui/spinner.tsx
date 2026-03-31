import { cn } from "@/lib/utils";

interface SpinnerProps {
  size?: number;
  /** Delay in ms before the spinner becomes visible. Uses a CSS animation so
   *  it renders immediately (no layout shift) but stays transparent until the
   *  delay elapses — fast loads never see it, slow loads get a graceful reveal. */
  delay?: number;
  className?: string;
}

export function Spinner({ size = 20, delay, className }: SpinnerProps) {
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
        ...(delay != null && {
          opacity: 0,
          animation: `dp-spin 1.2s linear infinite, dp-fadein 0s ${delay}ms forwards`,
        }),
      }}
    />
  );
}
