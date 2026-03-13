import { InlineBadge } from "@/components/InlineBadge";
import type { ResolvedIntegration } from "@/lib/api";
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

export function RequiredAppsList({ integrations, title = "Integrations" }: RequiredAppsListProps) {
  if (integrations.length === 0) return null;

  return (
    <SidebarSection title={title}>
      <div className="flex flex-wrap gap-2">
        {integrations.map((integration) => (
          <InlineBadge
            key={getIntegrationKey(integration)}
            className="rounded-md border-border-strong bg-stone-200 px-3.5 py-2 font-sans text-[13px] font-medium normal-case tracking-normal text-foreground dark:border-border-strong dark:bg-muted/30 dark:text-foreground"
          >
            <span>{getIntegrationName(integration)}</span>
          </InlineBadge>
        ))}
      </div>
    </SidebarSection>
  );
}
