import { useState } from "react";
import type { MetaFunction } from "react-router";
import { useParams } from "react-router";
import type { Route } from "./+types/AccountProfile";
import { createServerApi } from "@/lib/api.server";
import { useAccount, useAccountOrgs, useAccountMembers } from "@/api/queries/accounts";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useHeartedBlueprints } from "@/api/queries/hearts";
import { ProfileEditSidebar } from "./ProfileEditSidebar";
import { ProfileViewSidebar } from "./ProfileViewSidebar";
import { ProfileLayout } from "./ProfileLayout";
import type { HeartSort } from "./HeartsTab";

export const meta: MetaFunction = ({ params }) => [
  { title: `${params.account} | Astro` },
];

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const accountName = params.account ?? "";
  if (!accountName) return { account: null, orgs: null, members: null, blueprints: null, deployments: null, hearted: null };
  const account = await api.getAccount(accountName).catch(() => null);
  const members = account?.type === "organization"
    ? await api.getAccountMembers(accountName).catch(() => null)
    : null;
  return { account, orgs: null, members, blueprints: null, deployments: null, hearted: null };
}

export default function AccountProfile({ loaderData }: Route.ComponentProps) {
  const { account } = useParams<{ account: string }>();
  const { data } = useAccount(account ?? "", {
    initialData: loaderData.account ?? undefined,
  });
  const { isAuthenticated, accounts, user } = useAuth();

  const isOrg = data?.type === "organization";
  const isSelf = isAuthenticated && accounts.some((a) => a.id === data?.id);

  const { data: orgsData } = useAccountOrgs(account ?? "", {
    enabled: !isOrg,
    initialData: loaderData.orgs ?? undefined,
  });
  const { data: membersData } = useAccountMembers(account ?? "", {
    enabled: isOrg,
    initialData: loaderData.members ?? undefined,
  });

  const viewerMember = isAuthenticated && isOrg
    ? membersData?.members.find((m) => m.user_id === user?.id)
    : undefined;
  const isOrgMember = !!viewerMember;
  const isOrgAdmin = viewerMember?.role === "admin" || viewerMember?.role === "owner";
  const isOwnerOrAdmin = isSelf || isOrgAdmin;
  const canViewDeployments = isSelf || isOrgMember;

  const { data: deploymentsData, isLoading: isDeploymentsLoading } = useDeployments(data?.name ?? "", canViewDeployments, {
    initialData: loaderData.deployments ?? undefined,
  });
  const { data: blueprintsData, isLoading: isBlueprintsLoading } = useAccountBlueprints(data?.name ?? "", {
    enabled: !!data,
    initialData: loaderData.blueprints ?? undefined,
  });
  const { data: heartsData } = useHeartedBlueprints(data?.name ?? "", undefined, {
    enabled: !!data && !isOrg,
    initialData: loaderData.hearted ?? undefined,
  });

  const rawBlueprints = blueprintsData?.agents ?? [];
  const rawDeployments = deploymentsData?.deployments ?? [];
  const orgs = orgsData?.orgs ?? [];
  const members = membersData?.members ?? [];

  const [heartSearch, setHeartSearch] = useState("");
  const [heartSort, setHeartSort] = useState<HeartSort>("popular");

  const heartsTabCount =
    !isOrg && heartsData && heartsData.items.length > 0
      ? `${heartsData.items.length}${heartsData.next_cursor ? "+" : ""}`
      : null;

  if (!data) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  return (
    <ProfileLayout
      data={data}
      isAdmin={isOwnerOrAdmin}
      canViewDeployments={canViewDeployments}
      rawBlueprints={rawBlueprints}
      rawDeployments={rawDeployments}
      isBlueprintsLoading={isBlueprintsLoading}
      isDeploymentsLoading={isDeploymentsLoading}
      renderViewSidebar={({ blueprintCount, deploymentCount, isInternalView, onEditOpen, isBlueprintsLoading, isDeploymentsLoading }) => (
        <ProfileViewSidebar
          data={data}
          variant={isOrg ? "org" : "personal"}
          isAdmin={isOwnerOrAdmin}
          isInternalView={isInternalView}
          blueprintCount={blueprintCount}
          deploymentCount={deploymentCount}
          isBlueprintsLoading={isBlueprintsLoading}
          isDeploymentsLoading={isDeploymentsLoading}
          orgs={orgs}
          members={members}
          onEditOpen={onEditOpen}
        />
      )}
      renderEditSidebar={({ onClose }) => (
        <ProfileEditSidebar data={data} onClose={onClose} variant={isOrg ? "org" : "personal"} />
      )}
      hearts={!isOrg ? {
        isOwner: isSelf,
        search: heartSearch,
        onSearchChange: setHeartSearch,
        sort: heartSort,
        onSortChange: setHeartSort,
        tabCount: heartsTabCount,
      } : undefined}
    />
  );
}
