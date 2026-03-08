import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { integrationIconMap } from "@/lib/integrationIcons";
import { SidebarSection } from "./SidebarSection";

export interface RequiredAppsListProps {
  integrations: string[];
}

export function RequiredAppsList({ integrations }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <SidebarSection title="Required Apps">
      <div className="flex flex-col gap-2">
        {integrations.map((name) => {
          const icon: ReactNode = integrationIconMap[name];
          return (
            <div
              key={name}
              className="flex items-center gap-3 rounded-lg border border-stone-400 bg-stone-50 px-3 py-2 dark:border-border dark:bg-background"
            >
              {icon ? (
                <span className="flex h-5 w-5 shrink-0 items-center justify-center [&>svg]:size-full">
                  {icon}
                </span>
              ) : (
                <Puzzle className="h-5 w-5 shrink-0 text-muted-foreground" />
              )}
              <span className="text-[13px] font-medium text-foreground">{name}</span>
            </div>
          );
        })}
      </div>
    </SidebarSection>
  );
}
