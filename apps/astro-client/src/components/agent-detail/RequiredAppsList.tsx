import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { integrationIconMap } from "@/lib/integrationIcons";
import { SidebarSection } from "./SidebarSection";
import type { ResolvedIntegration } from "@/lib/api";

export interface RequiredAppsListProps {
  integrations: ResolvedIntegration[];
}

export function RequiredAppsList({ integrations }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <SidebarSection title="Integrations">
      <div className="flex flex-col gap-2">
        {integrations.map((integration) => {
          const icon: ReactNode = integrationIconMap[integration.id];
          return (
            <div
              key={integration.id}
              className="flex items-center gap-2.5"
            >
              {icon ? (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:size-full">
                  {icon}
                </span>
              ) : (
                <Puzzle className="h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <span className="text-[13px] text-foreground">{integration.name}</span>
            </div>
          );
        })}
      </div>
    </SidebarSection>
  );
}
