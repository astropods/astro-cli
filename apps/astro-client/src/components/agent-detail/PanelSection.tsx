import type { ReactNode } from "react";

interface PanelSectionProps {
  title: string;
  description?: string;
  emptyState?: string;
  isEmpty?: boolean;
  children?: ReactNode;
}

export function PanelSection({ title, description, emptyState, isEmpty, children }: PanelSectionProps) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h3 className="text-[0.9375rem] font-medium leading-tight tracking-wide text-foreground/80 dark:text-white/80">{title}</h3>
        {description && (
          <p className="text-body-sm tracking-wide text-muted-foreground dark:text-white/30">{description}</p>
        )}
      </div>
      {isEmpty ? (
        <p className="text-body-sm text-muted-foreground/50 dark:text-white/20">{emptyState ?? "None"}</p>
      ) : (
        children
      )}
    </section>
  );
}
