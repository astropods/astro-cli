import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { PermissionsPreview } from "./PermissionsPreview";
import { SidebarAbout } from "./SidebarAbout";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarStats } from "./SidebarStats";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import type { Agent, AccountPublic } from "@/lib/api";

export interface SidebarCardProps {
  agent: Agent;
  description: string;
  integrations: string[];
  permissions: string[];
  initialAccountData?: AccountPublic;
}

export function SidebarCard({
  agent,
  description,
  integrations,
  permissions,
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
    <div className="rounded-lg border border-stone-400 bg-stone-200 p-5 dark:border-border dark:bg-background">
      {/* Install CTA */}
      <Button asChild size="default" className="w-full gap-2 text-stone-100">
        <Link to={`/deploy/${agent.account}/${agent.name}`}>
          Install Agent
          <ArrowRight className="h-4 w-4" />
        </Link>
      </Button>

      <SidebarAbout description={description} />

      <SidebarAuthor
        name={ownerName}
        handle={agent.account}
        profilePictureUrl={owner?.profile_picture_url}
      />

      <SidebarStats
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
      />

      {/* Required Apps */}
      {integrations.length > 0 && (
        <>
          <div className="my-5 h-px bg-stone-400 dark:bg-border" />
          <RequiredAppsList integrations={integrations} />
        </>
      )}

      {/* Permissions */}
      {permissions.length > 0 && (
        <>
          <div className="my-5 h-px bg-stone-400 dark:bg-border" />
          <PermissionsPreview permissions={permissions} />
        </>
      )}
    </div>
  );
}

export type AgentDetailSidebarProps = SidebarCardProps;

export function AgentDetailSidebar(props: AgentDetailSidebarProps) {
  return (
    <div className="hidden min-[900px]:block w-[340px] shrink-0 pl-0 pr-8 pt-10 pb-6">
      <div>
        <SidebarCard {...props} />
      </div>
    </div>
  );
}
