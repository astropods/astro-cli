import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { CapabilitiesList } from "./CapabilitiesList";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarStats } from "./SidebarStats";
import { SidebarSection } from "./SidebarSection";
import { AgentCard, type AgentCardProps } from "@/components/AgentCard";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import type { Agent, AccountPublic, AgentCardAuthor, ResolvedIntegration } from "@/lib/api";

export interface SidebarCardProps {
  agent: Agent;
  integrations: ResolvedIntegration[];
  capabilities?: string[];
  authors?: AgentCardAuthor[];
  rating?: number;
  installs?: number;
  recommendedAgents?: AgentCardProps[];
  initialAccountData?: AccountPublic;
}

export function SidebarCard({
  agent,
  integrations,
  capabilities = [],
  authors = [],
  rating,
  installs,
  recommendedAgents = [],
  initialAccountData,
}: SidebarCardProps) {
  const latestVersion = agent.versions[0];
  const version = latestVersion?.version ?? latestVersion?.build_id?.slice(0, 8);
  const updatedAt = latestVersion?.published_at
    ? formatDate(latestVersion.published_at)
    : null;

  const { data: accountData } = useAccount(agent.account, {
    initialData: initialAccountData,
  });
  const owner = accountData?.owner;
  const ownerName = owner?.first_name && owner?.last_name
    ? `${owner.first_name} ${owner.last_name}`
    : agent.account;

  return (
    <div className="space-y-4">
      <div className="rounded-md border border-border-strong bg-stone-200 p-4 dark:bg-muted/30">
        <Button asChild size="default" className="h-11 w-full">
          <Link to={`/deploy/${agent.account}/${agent.name}`}>
            Deploy this agent
            <ArrowRight className="h-4 w-4" />
          </Link>
        </Button>
      </div>

      <SidebarAuthor
        authors={authors}
        ownerName={ownerName}
        ownerHandle={agent.account}
        ownerId={accountData?.id}
        ownerProfilePictureUrl={owner?.profile_picture_url}
      />

      <SidebarStats
        rating={rating}
        installs={installs}
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
      />

      {integrations.length > 0 && (
        <RequiredAppsList integrations={integrations} title="Integrations" />
      )}

      {capabilities.length > 0 && (
        <CapabilitiesList capabilities={capabilities} />
      )}

      {recommendedAgents.length > 0 && (
        <SidebarSection title="Often used together">
          <div className="space-y-2.5">
            {recommendedAgents.map((recommendedAgent) => (
              <AgentCard
                key={recommendedAgent.slug}
                {...recommendedAgent}
                variant="oftenUsedTogether"
              />
            ))}
          </div>
        </SidebarSection>
      )}

    </div>
  );
}

export type AgentDetailSidebarProps = SidebarCardProps;

export function AgentDetailSidebar(props: AgentDetailSidebarProps) {
  return (
    <div className="hidden min-[900px]:block w-[340px] shrink-0 pl-0 pr-8 pt-10 pb-6">
      <SidebarCard {...props} />
    </div>
  );
}
