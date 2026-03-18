import { InlineBadge } from "@/components/InlineBadge";
import type { ResolvedIntegration } from "@/lib/api";
import { getIntegrationIcon } from "@/lib/integrationIcons";
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

function isKnown(integration: ResolvedIntegration | string): boolean {
  if (typeof integration === "string") return false;
  return integration.known;
}

export function RequiredAppsList({ integrations, title = "Integrations" }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <SidebarSection title={title}>
      <div className="flex flex-wrap gap-2">
        {integrations.map((integration) => {
          const key = getIntegrationKey(integration);
          const known = isKnown(integration);
          return (
            <InlineBadge
              key={key}
              className="gap-1.5 rounded-full border-border-strong bg-stone-200 px-2.5 py-1 font-sans text-[12px] font-medium normal-case tracking-normal text-foreground dark:border-border-strong dark:bg-muted/30 dark:text-foreground"
            >
              {known && (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
                  {getIntegrationIcon(key)}
                </span>
              )}
              <span className="whitespace-nowrap">{getIntegrationName(integration)}</span>
            </InlineBadge>
          );
        })}
      </div>
    </SidebarSection>
  );
}
