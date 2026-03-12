import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { integrationIconMap } from "@/lib/integrationIcons";

export interface RequiredAppsListProps {
  integrations: string[];
  title?: string;
}

export function RequiredAppsList({ integrations, title = "Connected apps" }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <div className="space-y-2">
      <h4 className="text-[13px] font-medium text-foreground">{title}</h4>
      <div className="flex flex-wrap gap-2">
        {integrations.map((name) => {
          const icon: ReactNode = integrationIconMap[name];
          return (
            <div
              key={name}
              className="inline-flex items-center gap-2 rounded-md border border-border-strong bg-background px-3 py-1.5"
            >
              {icon ? (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:size-full">
                  {icon}
                </span>
              ) : (
                <Puzzle className="h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <span className="text-[12px] font-medium text-foreground">{name}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
