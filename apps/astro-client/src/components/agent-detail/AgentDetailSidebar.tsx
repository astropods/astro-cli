import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { PermissionsPreview } from "./PermissionsPreview";
import { SidebarAuthor } from "./SidebarAuthor";
import { SidebarStats } from "./SidebarStats";
import { SidebarSection } from "./SidebarSection";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import type { Agent, AccountPublic, AgentCardAuthor, ResolvedIntegration } from "@/lib/api";

export interface SidebarCardProps {
  agent: Agent;
  description: string;
  integrations: string[];
  permissions: string[];
  rating?: number;
  installs?: number;
  teammateInstallCount?: number;
  teammateInitials?: string[];
  initialAccountData?: AccountPublic;
}

export function SidebarCard({
  agent,
  integrations,
  permissions,
  rating,
  installs,
  teammateInstallCount,
  teammateInitials,
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
  const teammateBadges = teammateInitials?.slice(0, 3) ?? [];
  const teammateLabel = teammateInstallCount ?? teammateBadges.length;
  const teammateBadgeClasses = [
    "bg-indigo-600 text-white",
    "bg-teal-600 text-white",
    "bg-amber-700 text-white",
  ];

  return (
    <div className="space-y-4">
      <div className="rounded-md border border-border-strong bg-stone-200 p-4 dark:bg-muted/30">
        <Button asChild size="default" className="h-11 w-full gap-2 rounded-md px-4 font-mono text-[14px] font-semibold text-stone-100">
          <Link to={`/deploy/${agent.account}/${agent.name}`}>
            Hire this agent
            <ArrowRight className="h-4 w-4" />
          </Link>
        </Button>
        {teammateLabel > 0 && (
          <div className="mt-3.5 flex items-center gap-2.5 text-[13px] text-muted-foreground">
            <div className="flex -space-x-1.5">
              {teammateBadges.map((label, idx) => (
                <span
                  key={`${label}-${idx}`}
                  className={`inline-flex h-5 w-5 items-center justify-center rounded-full border border-surface text-[9px] font-mono ${teammateBadgeClasses[idx % teammateBadgeClasses.length]}`}
                >
                  {label.slice(0, 2).toUpperCase()}
                </span>
              ))}
            </div>
            <span>
              {teammateLabel} of your teammates installed this
            </span>
          </div>
        )}
      </div>

      <SidebarAuthor
        authors={authors}
        ownerName={ownerName}
        ownerHandle={agent.account}
        ownerProfilePictureUrl={owner?.profile_picture_url}
      />

      <SidebarStats
        rating={rating}
        installs={installs}
        version={version}
        isSemver={!!latestVersion?.version}
        updatedAt={updatedAt ?? undefined}
      />

      {(integrations.length > 0 || permissions.length > 0) && (
        <SidebarSection title="What it needs">
          <div className="space-y-5">
            {integrations.length > 0 && (
              <RequiredAppsList integrations={integrations} title="Connected apps" />
            )}
            {integrations.length > 0 && permissions.length > 0 && (
              <div className="h-px bg-border-strong/90" />
            )}
            {permissions.length > 0 && (
              <PermissionsPreview permissions={permissions} title="Permissions" />
            )}
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
