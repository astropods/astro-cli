import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface EvalTabCardProps {
  className?: string;
  children: ReactNode;
}

export function EvalTabCard({ className, children }: EvalTabCardProps) {
  return (
    <div
      className={cn(
        "flex min-w-0 flex-col overflow-hidden rounded-lg border border-border",
        className,
      )}
    >
      {children}
    </div>
  );
}

export interface EvalTabCardHeaderProps {
  label: string;
  description?: ReactNode;
  className?: string;
  /** Right-aligned actions (filter chips, toggles, etc.). */
  children?: ReactNode;
}

export function EvalTabCardHeader({
  label,
  description,
  className,
  children,
}: EvalTabCardHeaderProps) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border bg-card px-5 py-3 dark:bg-surface",
        className,
      )}
    >
      <div className="flex min-w-0 max-w-full flex-col gap-0.5">
        <div className="flex min-w-0 items-baseline gap-2">
          <span className="flex-none text-body font-semibold text-foreground">
            {label}
          </span>
        </div>
        {description && (
          <div className="text-body-sm text-muted-foreground">{description}</div>
        )}
      </div>
      {children && (
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-3">
          {children}
        </div>
      )}
    </div>
  );
}

export interface EvalTabCardBodyProps {
  className?: string;
  children: ReactNode;
}

export function EvalTabCardBody({ className, children }: EvalTabCardBodyProps) {
  return <div className={cn("flex min-h-0 flex-1", className)}>{children}</div>;
}
