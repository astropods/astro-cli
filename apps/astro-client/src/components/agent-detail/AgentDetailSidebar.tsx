import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { RequiredAppsList } from "./RequiredAppsList";
import { PermissionsPreview } from "./PermissionsPreview";
import { formatDate } from "@/lib/utils";
import { useAccount } from "@/api/queries";
import type { Agent, AccountPublic } from "@/lib/api";

export interface AgentDetailSidebarProps {
  agent: Agent;
  description: string;
  integrations: string[];
  permissions: string[];
  initialAccountData?: AccountPublic;
}

export function AgentDetailSidebar({
  agent,
  description,
  integrations,
  permissions,
  initialAccountData,
}: AgentDetailSidebarProps) {
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
  const accountInitial = ownerName.charAt(0).toUpperCase();

  return (
    <div className="hidden lg:block w-[340px] shrink-0 p-6">
      <div className="sticky top-[57px]">
        <div className="rounded-lg border border-border bg-stone-100 p-5">
          {/* Install CTA */}
          <Button asChild size="lg" className="w-full gap-2">
            <Link to={`/deploy/${agent.account}/${agent.name}`}>
              Install Agent
              <ArrowRight className="h-4 w-4" />
            </Link>
          </Button>

          {/* About */}
          {description && (
            <div className="pt-5 mt-5 border-t border-border">
              <span className="text-xs text-muted-foreground mb-2 block">About</span>
              <p className="text-sm text-foreground leading-snug">
                {description}
              </p>
            </div>
          )}

          {/* Author block */}
          <div className="pt-5 mt-5 border-t border-border">
            <span className="text-xs text-muted-foreground mb-3 block">Created by</span>
            <div className="flex items-center gap-3">
              {owner?.profile_picture_url ? (
                <img
                  src={owner.profile_picture_url}
                  alt={ownerName}
                  className="h-10 w-10 shrink-0 rounded-full object-cover"
                />
              ) : (
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-stone-200 text-sm font-semibold text-muted-foreground">
                  {accountInitial}
                </div>
              )}
              <div className="flex flex-col">
                <span className="text-sm font-medium text-foreground">
                  {ownerName}
                </span>
                <span className="text-xs text-muted-foreground">
                  @{agent.account}
                </span>
              </div>
            </div>
          </div>

          {/* Stats */}
          {(version || updatedAt) && (
            <>
              <div className="my-5 h-px bg-border" />
              <div className="grid grid-cols-2 gap-3">
                {version && (
                  <div className="flex flex-col gap-0.5">
                    <span className="text-xs text-muted-foreground">Version</span>
                    <span className="text-sm font-medium text-foreground">
                      {latestVersion?.version ? `v${version}` : version}
                    </span>
                  </div>
                )}
                {updatedAt && (
                  <div className="flex flex-col gap-0.5">
                    <span className="text-xs text-muted-foreground">Updated</span>
                    <span className="text-sm font-medium text-foreground">
                      {updatedAt}
                    </span>
                  </div>
                )}
              </div>
            </>
          )}

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
      </div>
    </div>
  );
}
