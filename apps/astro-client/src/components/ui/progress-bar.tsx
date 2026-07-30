import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export type ProgressBarTone =
  | "primary"
  | "success"
  | "warning"
  | "destructive"
  | "muted";

export type ProgressBarSize = "xs" | "sm";

const TONE_CLASS: Record<ProgressBarTone, string> = {
  primary: "bg-primary",
  success: "bg-success",
  warning: "bg-warning",
  destructive: "bg-destructive",
  muted: "bg-muted-foreground",
};

const SIZE_CLASS: Record<ProgressBarSize, string> = {
  xs: "h-1",
  sm: "h-1.5",
};

export interface ProgressBarProps
  extends Omit<ComponentProps<"div">, "children"> {
  value: number;
  max?: number;
  tone?: ProgressBarTone;
  size?: ProgressBarSize;
  indicatorClassName?: string;
}

/** A determinate progress track. Callers own its width and accessible label. */
export function ProgressBar({
  value,
  max = 100,
  tone = "primary",
  size = "sm",
  className,
  indicatorClassName,
  ...props
}: ProgressBarProps) {
  const normalizedMax = Number.isFinite(max) && max > 0 ? max : 100;
  const normalizedValue = Number.isFinite(value)
    ? Math.min(normalizedMax, Math.max(0, value))
    : 0;
  const percentage = (normalizedValue / normalizedMax) * 100;

  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={normalizedMax}
      aria-valuenow={normalizedValue}
      className={cn(
        "w-full overflow-hidden rounded-full bg-border",
        SIZE_CLASS[size],
        className,
      )}
      {...props}
    >
      <div
        className={cn(
          "h-full rounded-full",
          TONE_CLASS[tone],
          indicatorClassName,
        )}
        style={{ width: `${percentage}%` }}
      />
    </div>
  );
}
