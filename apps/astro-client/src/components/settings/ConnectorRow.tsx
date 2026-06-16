import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface ConnectorRowProps {
  icon: ReactNode;
  name: string;
  description: ReactNode;
  action?: ReactNode;
  isLoading?: boolean;
  children?: ReactNode;
}

export function ConnectorRow({ icon, name, description, action, isLoading, children }: ConnectorRowProps) {
  return (
    <div className="border-t border-border first:border-t-0">
      <div className="flex items-center justify-between gap-4 py-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="size-10 shrink-0 flex items-center justify-center rounded-md bg-muted/40">
            {icon}
          </div>
          <div className="min-w-0">
            {isLoading ? (
              <div className="h-4 w-32 rounded animate-pulse bg-muted" />
            ) : (
              <>
                <div className="text-body font-medium text-foreground">{name}</div>
                <div className="text-body-sm text-muted-foreground mt-0.5 truncate">{description}</div>
              </>
            )}
          </div>
        </div>
        {!isLoading && action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </div>
  );
}

export function ConnectorRowList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <ul className={cn("border-t border-border divide-y divide-border", className)}>
      {children}
    </ul>
  );
}

export function ConnectorRowItem({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <li className={cn("flex items-center gap-2.5 px-4 py-2.5 text-body-sm", className)}>
      {children}
    </li>
  );
}
