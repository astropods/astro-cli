import { InlineBadge } from "@/components/InlineBadge";
import type { ResolvedIntegration } from "@/lib/api";
import { integrationIconMap } from "@/lib/integrationIcons";
import { SidebarSection } from "./SidebarSection";

export interface RequiredAppsListProps {
  integrations: Array<ResolvedIntegration | string>;
  title?: string;
}

function getIntegrationName(integration: ResolvedIntegration | string): string {
  if (typeof integration === "string") return integration;
  return integration.name || integration.id;
}

function getIntegrationKey(integration: ResolvedIntegration | string): string {
  if (typeof integration === "string") return integration;
  return integration.id || integration.name;
}

function getIntegrationIcon(integration: ResolvedIntegration | string) {
  if (typeof integration === "string") {
    return integrationIconMap[integration];
  }
  return integrationIconMap[integration.id] ?? integrationIconMap[integration.name];
}

export function RequiredAppsList({ integrations, title = "Integrations" }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <SidebarSection title={title}>
      <div className="flex flex-wrap gap-2">
        {integrations.map((integration) => {
          const icon = getIntegrationIcon(integration);
          return (
            <InlineBadge
              key={getIntegrationKey(integration)}
              className="gap-2 rounded-md border-border-strong bg-stone-200 px-3.5 py-1.5 font-sans text-[13px] font-medium normal-case tracking-normal text-foreground dark:border-border-strong dark:bg-muted/30 dark:text-foreground"
            >
              {icon && (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
                  {icon}
                </span>
              )}
              <span>{getIntegrationName(integration)}</span>
            </InlineBadge>
          );
        })}
      </div>
    </SidebarSection>
  );
}
