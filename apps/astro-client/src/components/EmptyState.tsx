import type { ReactNode } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export interface EmptyStateAction {
  label: string;
  to: string;
  icon?: ReactNode;
  variant?: "default" | "outline";
}

export interface EmptyStateProps {
  title: string;
  description?: string;
  icon?: ReactNode;
  actions?: EmptyStateAction[];
  /** @deprecated use actions[] */
  actionLabel?: string;
  /** @deprecated use actions[] */
  actionTo?: string;
  variant?: "default" | "card";
}

function EmptyStateActions({
  actions,
  className,
}: {
  actions: EmptyStateAction[];
  className?: string;
}) {
  if (actions.length === 0) return null;
  return (
    <div className={cn("flex flex-wrap items-center justify-center gap-3", className)}>
      {actions.map((a) => (
        <Button key={a.label} variant={a.variant ?? "default"} asChild>
          <Link to={a.to}>
            {a.icon}
            {a.label}
          </Link>
        </Button>
      ))}
    </div>
  );
}

export function EmptyState({
  title,
  description,
  icon,
  actions,
  actionLabel,
  actionTo,
  variant = "default",
}: EmptyStateProps) {
  const resolvedActions: EmptyStateAction[] =
    actions ?? (actionLabel && actionTo ? [{ label: actionLabel, to: actionTo }] : []);

  if (variant === "card") {
    return (
      <div className="rounded-lg border border-dashed border-border-strong px-6 py-12 text-center">
        {icon && <div className="mx-auto mb-4">{icon}</div>}
        <p className="text-heading-3 text-foreground mb-2">{title}</p>
        {description && (
          <p className="text-body text-muted-foreground mb-6 max-w-sm mx-auto">
            {description}
          </p>
        )}
        <EmptyStateActions actions={resolvedActions} />
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <p className="text-lg font-medium">{title}</p>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
      <EmptyStateActions actions={resolvedActions} className="mt-2" />
    </div>
  );
}
