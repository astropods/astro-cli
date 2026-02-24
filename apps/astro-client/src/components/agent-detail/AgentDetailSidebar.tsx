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
    <div className="rounded-lg border border-border bg-stone-100 p-5">
      {/* Install CTA */}
      <Button asChild size="lg" className="w-full gap-2">
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
          <div className="my-5 h-px bg-border" />
          <RequiredAppsList integrations={integrations} />
        </>
      )}

      {/* Permissions */}
      {permissions.length > 0 && (
        <>
          <div className="my-5 h-px bg-border" />
          <PermissionsPreview permissions={permissions} />
        </>
      )}
    </div>
  );
}

export type AgentDetailSidebarProps = SidebarCardProps;

export function AgentDetailSidebar(props: AgentDetailSidebarProps) {
  return (
    <div className="hidden lg:block w-[340px] shrink-0 p-6">
      <div className="sticky top-[57px]">
        <SidebarCard {...props} />
      </div>
    </div>
  );
}
