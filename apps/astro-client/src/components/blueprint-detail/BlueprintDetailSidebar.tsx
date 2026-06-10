import { useMemo } from "react";
import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { HoloButton } from "@/components/ui/holo-button";
import { RequiredAppsList } from "./RequiredAppsList";
import { CapabilitiesList } from "./CapabilitiesList";
import { GitHubConnectionPanel } from "./GitHubConnectionPanel";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarDeployedAgents } from "./SidebarDeployedAgents";
import { SidebarRepository } from "./SidebarRepository";
import { SidebarSection } from "./SidebarSection";
import { SidebarStats } from "./SidebarStats";
import { BlueprintCard, type BlueprintCardProps } from "@/components/BlueprintCard";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import { getBlueprintRepository } from "@/lib/blueprint-utils";
import type { Blueprint, AccountPublic, BlueprintAuthor, ResolvedIntegration } from "@/lib/api";

export interface SidebarCardProps {
  agent: Blueprint;
  integrations: ResolvedIntegration[];
  capabilities?: string[];
  authors?: BlueprintAuthor[];
  publishers?: BlueprintAuthor[];
  rating?: number;
  installs?: number;
  recommendedAgents?: BlueprintCardProps[];
  initialAccountData?: AccountPublic;
  canEdit?: boolean;
  githubRepoName?: string;
  githubBranch?: string;
}

export function SidebarCard({
  agent,
  integrations,
  capabilities = [],
  authors = [],
  publishers = [],
  rating,
  installs,
  recommendedAgents = [],
  initialAccountData,
  canEdit,
  githubRepoName,
  githubBranch,
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
  const ownerHandle = agent.account;

  const displayAuthors = publishers.length > 0 ? publishers : authors;

  const repository = getBlueprintRepository(agent);
  const isDraft = agent.versions.length === 0;

  const buildIds = useMemo(
    () => agent.versions.map((v) => v.build_id),
    [agent.versions],
  );

  return (
    <div className="space-y-4">
      <div className="rounded-[4px] border border-border-strong bg-slate-200 p-4 dark:bg-muted/30">
        {isDraft ? (
          <Button size="default" className="h-11 w-full" disabled>
            Deploy this agent
            <ArrowRight className="h-4 w-4" />
          </Button>
        ) : (
          <HoloButton asChild accentHex={agent.avatar_colors?.accent} className="h-11 w-full">
            <Link to={`/deploy/${agent.account}/${agent.name}`}>
              Deploy this agent
              <ArrowRight className="h-4 w-4" />
            </Link>
          </HoloButton>
        )}
      </div>

      {canEdit && (
        <GitHubConnectionPanel account={agent.account} name={agent.name} preConnectedRepo={githubRepoName} preConnectedBranch={githubBranch} />
      )}

      <SidebarDeployedAgents
        account={agent.account}
        blueprintName={agent.name}
        buildIds={buildIds}
      />

      <SidebarStats
        rating={rating}
        installs={installs}
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
        visibility={agent.visibility}
        isDraft={isDraft}
      />

      <SidebarAuthor
        authors={displayAuthors}
        ownerName={ownerName}
        ownerHandle={ownerHandle}
      />

      {repository && <SidebarRepository repository={repository} />}

      {integrations.length > 0 && (
        <RequiredAppsList integrations={integrations} title="Integrations" />
      )}

      {capabilities.length > 0 && (
        <CapabilitiesList capabilities={capabilities} />
      )}

      {recommendedAgents.length > 0 && (
        <SidebarSection title="More blueprints">
          <div className="space-y-2.5">
            {recommendedAgents.map((recommendedAgent) => (
              <BlueprintCard
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

export type BlueprintDetailSidebarProps = SidebarCardProps;

export function BlueprintDetailSidebar(props: BlueprintDetailSidebarProps) {
  return (
    <div className="hidden min-[900px]:block w-[340px] shrink-0 pl-0 pr-8 pt-10 pb-6">
      <SidebarCard {...props} />
    </div>
  );
}
