import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { integrationIconMap } from "@/lib/integrationIcons";
import { InlineBadge } from "@/components/InlineBadge";

export interface RequiredAppsListProps {
  integrations: ResolvedIntegration[];
  title?: string;
}

export function RequiredAppsList({ integrations, title = "Connected apps" }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <div className="space-y-3">
      <h4 className="text-[13px] font-semibold text-foreground">{title}</h4>
      <div className="flex flex-wrap gap-2">
        {integrations.map((integration) => {
          const name = integration.name;
          const key = integration.id;
          const icon: ReactNode = integrationIconMap[name];
          return (
            <InlineBadge
              key={name}
              className="gap-2 rounded-md border-border-strong bg-stone-200 px-3.5 py-2 font-sans text-[13px] font-medium normal-case tracking-normal text-foreground dark:border-border-strong dark:bg-muted/30 dark:text-foreground"
            >
              {icon ? (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:size-full [&>svg]:text-muted-foreground">
                  {icon}
                </span>
              ) : (
                <Puzzle className="h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <span>
                {formatIntegrationLabel(name)}
              </span>
            </InlineBadge>
          );
        })}
      </div>
    </div>
  );
}
