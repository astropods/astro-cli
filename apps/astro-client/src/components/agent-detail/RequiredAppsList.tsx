import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { integrationIconMap } from "@/lib/integrationIcons";

export interface RequiredAppsListProps {
  integrations: string[];
}

export function RequiredAppsList({ integrations }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <div>
      <span className="text-xs text-stone-400 mb-3 block">Required Apps</span>
      <div className="flex flex-col gap-2">
        {integrations.map((name) => {
          const icon: ReactNode = integrationIconMap[name];
          return (
            <div
              key={name}
              className="flex items-center gap-3 rounded-lg border border-border px-3 py-2.5"
            >
              {icon ? (
                <span className="flex h-5 w-5 shrink-0 items-center justify-center [&>svg]:size-full">
                  {icon}
                </span>
              ) : (
                <Puzzle className="h-5 w-5 shrink-0 text-stone-400" />
              )}
              <span className="text-sm font-medium text-foreground">{name}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
