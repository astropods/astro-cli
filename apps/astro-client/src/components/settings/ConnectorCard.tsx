import type { ReactNode } from "react";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface ConnectorCardProps {
  icon: ReactNode;
  status: ReactNode;
  meta?: ReactNode;
  isLoading?: boolean;
  action?: ReactNode;
  children?: ReactNode;
}

export function ConnectorCard({ icon, status, meta, isLoading, action, children }: ConnectorCardProps) {
  return (
    <Card className="overflow-hidden">
      <div className="flex items-center justify-between gap-4 px-3 py-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="size-5 shrink-0 flex items-center justify-center text-foreground">
            {icon}
          </div>
          <div className="min-w-0">
            {isLoading ? (
              <div className="h-4 w-32 rounded animate-pulse bg-muted" />
            ) : (
              <>
                <div className="text-[13px] font-medium text-foreground truncate">{status}</div>
                {meta && <div className="text-[12px] text-muted-foreground mt-0.5 truncate">{meta}</div>}
              </>
            )}
          </div>
        </div>
        {!isLoading && action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children ? (
        <ul className="border-t border-border divide-y divide-border">{children}</ul>
      ) : null}
    </Card>
  );
}

export function ConnectorCardRow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <li className={cn("flex items-center gap-2.5 px-3 py-2.5 text-[12px] bg-background", className)}>
      {children}
    </li>
  );
}
