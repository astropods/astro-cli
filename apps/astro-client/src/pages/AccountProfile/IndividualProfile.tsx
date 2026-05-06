import { useState } from "react";
import { useParams } from "react-router";
import type { Route } from "./+types/AccountProfile";
import { useAccount, useAccountOrgs } from "@/api/queries/accounts";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { BlueprintCard } from "@/components/BlueprintCard";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import { PageContainer } from "@/components/PageLayout";
import { GradientGridWash } from "@/components/GradientGridWash";
import { ProfileEditSidebar } from "./ProfileEditSidebar";
import { ProfileViewSidebar } from "./ProfileViewSidebar";

interface IndividualProfileProps {
  loaderData: Route.ComponentProps["loaderData"];
}

export function IndividualProfile({ loaderData }: IndividualProfileProps) {
  const { account } = useParams<{ account: string }>();
  const { data } = useAccount(account ?? "", {
    initialData: loaderData.account ?? undefined,
  });
  const { isAuthenticated, accounts } = useAuth();
  const { data: orgsData } = useAccountOrgs(account ?? "", {
    initialData: loaderData.orgs ?? undefined,
  });
  const [editOpen, setEditOpen] = useState(false);

  const isOwner = isAuthenticated && accounts.some((a) => a.name === data?.name);
  const orgs = orgsData?.orgs ?? [];

  const { data: deploymentsData } = useDeployments(data?.name ?? "", isOwner, {
    initialData: loaderData.deployments ?? undefined,
  });
  const { data: blueprintsData } = useAccountBlueprints(data?.name ?? "", {
    enabled: !!data,
    initialData: loaderData.blueprints ?? undefined,
  });

  if (!data) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  const rawBlueprints = blueprintsData?.agents ?? [];
  const deployments = deploymentsData?.deployments ?? [];
  const visibleBlueprints = isOwner
    ? rawBlueprints
    : rawBlueprints.filter((bp) => bp.visibility === "public");

  return (
    <PageContainer
      className="flex min-h-0"
      outerClassName="bg-background"
      outerChildren={
        <GradientGridWash
          colors={data.avatar_colors ?? undefined}
          opacity={0.4}
          className="absolute left-0 top-0 h-[700px] w-[calc((100%-min(100%,1500px))/2+20rem)] [mask-image:radial-gradient(ellipse_120%_150%_at_0%_0%,black_0%,transparent_80%)]"
        />
      }
    >
      <aside className="w-72 shrink-0 border-r border-border overflow-hidden">
        {editOpen ? (
          <ProfileEditSidebar data={data} onClose={() => setEditOpen(false)} />
        ) : (
          <ProfileViewSidebar
            data={data}
            isOwner={isOwner}
            blueprintCount={visibleBlueprints.length}
            deploymentCount={deployments.length}
            orgs={orgs}
            onEditOpen={() => setEditOpen(true)}
          />
        )}
      </aside>

      <main className="relative flex flex-1 min-w-0 flex-col gap-5 px-8 py-7">
        <h2 className="text-heading-2 text-foreground">
          {isOwner ? "My Blueprints" : "Blueprints"}
        </h2>
        {visibleBlueprints.length === 0 ? (
          <p className="text-body text-muted-foreground">No blueprints published yet.</p>
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {visibleBlueprints.map((agent) => (
              <BlueprintCard
                key={agent.name}
                slug={`${data.name}/${agent.name}`}
                account={data.name}
                name={agent.name}
                description={getBlueprintDescription(agent)}
                visibility={agent.visibility}
                avatarColors={agent.avatar_colors}
                deployCount={agent.metrics?.deploy_count}
                onArchive={isOwner ? () => {} : undefined}
              />
            ))}
          </div>
        )}
      </main>
    </PageContainer>
  );
}
