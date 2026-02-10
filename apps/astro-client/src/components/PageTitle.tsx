import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface PageTitleProps {
  title: string;
  subtitle?: string;
  actions?: ReactNode;
  className?: string;
}

export function PageTitle({ title, subtitle, actions, className }: PageTitleProps) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-9">{title}</h1>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      {subtitle && (
        <p className="text-sm font-medium text-muted-foreground">{subtitle}</p>
      )}
    </div>
  );
}
