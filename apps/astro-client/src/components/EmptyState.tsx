import type { ReactNode } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";

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

export function EmptyState({ title, description, icon, actions, actionLabel, actionTo, variant = "default" }: EmptyStateProps) {
  const resolvedActions: EmptyStateAction[] = actions ?? (actionLabel && actionTo ? [{ label: actionLabel, to: actionTo }] : []);

  if (variant === "card") {
    return (
      <div className="rounded-lg border border-dashed border-border-strong px-6 py-12 text-center">
        {icon && (
          <div className="mx-auto mb-4">
            {icon}
          </div>
        )}
        <p className="text-heading-3 text-foreground mb-2">{title}</p>
        {description && (
          <p className="text-body text-muted-foreground mb-6 max-w-sm mx-auto">{description}</p>
        )}
        {resolvedActions.length > 0 && (
          <div className="flex flex-wrap items-center justify-center gap-3">
            {resolvedActions.map((a) => (
              <Button key={a.to} variant={a.variant ?? "default"} asChild>
                <Link to={a.to}>
                  {a.icon}
                  {a.label}
                </Link>
              </Button>
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <p className="text-lg font-medium">{title}</p>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
      {resolvedActions.length > 0 && (
        <div className="flex flex-wrap items-center justify-center gap-3 mt-2">
          {resolvedActions.map((a) => (
            <Button key={a.to} variant={a.variant ?? "default"} asChild>
              <Link to={a.to}>
                {a.icon}
                {a.label}
              </Link>
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}
