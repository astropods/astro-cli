import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { CapabilitiesList } from "./CapabilitiesList";
import { SidebarAbout } from "./SidebarAbout";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarStats } from "./SidebarStats";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import type { Agent, AccountPublic, AgentCardAuthor, ResolvedIntegration } from "@/lib/api";

export interface SidebarCardProps {
  agent: Agent;
  description: string;
  integrations: ResolvedIntegration[];
  capabilities?: string[];
  authors?: AgentCardAuthor[];
  initialAccountData?: AccountPublic;
}

export function SidebarCard({
  agent,
  description,
  integrations,
  capabilities = [],
  authors = [],
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
    <div className="rounded-lg border border-border-strong bg-stone-200 p-5 dark:bg-background">
      {/* Install CTA */}
      <Button asChild size="default" className="w-full gap-2 text-stone-100">
        <Link to={`/deploy/${agent.account}/${agent.name}`}>
          Install Agent
          <ArrowRight className="h-4 w-4" />
        </Link>
      </Button>

      <SidebarAbout description={description} />

      <SidebarAuthor
        authors={authors}
        ownerName={ownerName}
        ownerHandle={agent.account}
        ownerProfilePictureUrl={owner?.profile_picture_url}
      />

      <SidebarStats
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
      />

      {/* Integrations */}
      {integrations.length > 0 && (
        <>
          <div className="my-5 h-px bg-border-strong" />
          <RequiredAppsList integrations={integrations} />
        </>
      )}

      {/* Capabilities */}
      {capabilities.length > 0 && (
        <>
          <div className="my-5 h-px bg-border-strong" />
          <CapabilitiesList capabilities={capabilities} />
        </>
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
