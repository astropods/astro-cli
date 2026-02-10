import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { IntegrationBadge } from "./IntegrationBadge";

export interface IntegrationIconStackItem {
  name: string;
  icon: ReactNode;
}

export interface IntegrationIconStackProps {
  integrations: IntegrationIconStackItem[];
  max?: number;
  className?: string;
}

export function IntegrationIconStack({
  integrations,
  max = 3,
  className,
}: IntegrationIconStackProps) {
  if (integrations.length === 0) return null;

  const visible = integrations.slice(0, max);
  const overflowCount = integrations.length - max;

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div className="flex items-center">
        {visible.map((integration, i) => (
          <IntegrationBadge
            key={integration.name}
            name={integration.name}
            icon={integration.icon}
            className={i > 0 ? "-ml-1" : undefined}
            style={{ zIndex: i }}
          />
        ))}
      </div>
      {overflowCount > 0 && (
        <span className="text-xs text-muted-foreground">
          +{overflowCount}
        </span>
      )}
    </div>
  );
}
