import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface ConnectorRowProps {
  icon: ReactNode;
  name: string;
  description?: ReactNode;
  action?: ReactNode;
  isLoading?: boolean;
  children?: ReactNode;
  stackActionOnMobile?: boolean;
}

export function ConnectorRow({
  icon,
  name,
  description,
  action,
  isLoading,
  children,
  stackActionOnMobile = false,
}: ConnectorRowProps) {
  return (
    <div className="border-t border-border first:border-t-0">
      <div
        className={cn(
          "grid min-h-20 items-center gap-x-3.5 gap-y-0.5 px-4 py-4 sm:flex sm:gap-3.5 sm:px-5",
          stackActionOnMobile
            ? "grid-cols-[auto_minmax(0,1fr)]"
            : "grid-cols-[auto_minmax(0,1fr)_auto]",
        )}
      >
        <div className="row-span-2 flex size-10 shrink-0 items-center justify-center rounded-md border border-border bg-background">
          {icon}
        </div>
        <div className="contents sm:mr-auto sm:block sm:min-w-0">
          {isLoading ? (
            <div className="col-start-2 row-span-2 h-4 w-32 rounded animate-pulse bg-muted" />
          ) : (
            <>
              <div className="col-start-2 row-start-1 text-body font-medium text-foreground">
                {name}
              </div>
              {description ? (
                <div
                  className={cn(
                    "col-start-2 row-start-2 text-body-sm leading-5 text-muted-foreground",
                    !stackActionOnMobile && "col-end-4",
                  )}
                >
                  {description}
                </div>
              ) : null}
            </>
          )}
        </div>
        {!isLoading && action ? (
          <div
            className={cn(
              "shrink-0",
              stackActionOnMobile
                ? "col-span-2 row-start-3 mt-3 w-full sm:mt-0 sm:w-auto [&>*]:w-full sm:[&>*]:w-auto"
                : "col-start-3 row-start-1 self-start sm:self-center",
            )}
          >
            {action}
          </div>
        ) : null}
      </div>
      {children}
    </div>
  );
}

export function ConnectorRowList({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <ul
      className={cn(
        "mx-4 mb-4 overflow-hidden rounded-sm border border-border bg-popover divide-y divide-border sm:mx-5",
        className,
      )}
    >
      {children}
    </ul>
  );
}

export function ConnectorRowItem({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <li className={cn("flex min-h-10 items-center gap-2.5 px-3 py-2.5 text-body-sm", className)}>
      {children}
    </li>
  );
}
