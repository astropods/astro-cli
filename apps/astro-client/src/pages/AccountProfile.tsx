import { useParams } from "react-router";
import { useAccount } from "../api/queries/accounts";
import { useDeployments } from "../api/queries/deployments";
import { useAuth } from "../lib/auth";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { BlueprintCard } from "../components/BlueprintCard";
import { useAccountBlueprints } from "../api/queries/blueprints";
import { getBlueprintDescription } from "../lib/blueprint-utils";
import { mapDeploymentStatus, formatDate } from "../lib/deployment-utils";
import { deploymentPath } from "../lib/routes";
import { ShieldCheck } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

function AccountProfileContent() {
  const { account } = useParams<{ account: string }>();
  const { data, isLoading } = useAccount(account ?? "");
  const { isAuthenticated, accounts } = useAuth();

  const isMember = isAuthenticated && accounts.some((a) => a.name === data?.name);

  const { data: deploymentsData } = useDeployments(
    data?.name ?? "",
    isMember,
  );

  const { data: blueprintsData } = useAccountBlueprints(data?.name ?? "", { enabled: !!data });
  const accountBlueprints = blueprintsData?.agents ?? [];

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  const deployments = deploymentsData?.deployments ?? [];

  return (
    <div className="flex flex-1 flex-col p-6 md:p-8">
      <div className="flex items-center gap-4">
        <UserAvatar handle={data.name} name={data.display_name || data.name} className="size-16" />
        <div>
          {data.display_name && (
            <h1 className="text-2xl font-bold">
              {data.display_name}
            </h1>
          )}
          <p className="text-muted-foreground">@{data.name}</p>
        </div>
      </div>

      {isMember && (
        <div className="mt-8">
          <h3 className="flex items-center gap-1.5 text-xl font-semibold">
            Deployed agents
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <ShieldCheck className="size-4 text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>
                  Only visible to members with access
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </h3>
          {deployments.length === 0 ? (
            <p className="text-muted-foreground mt-3">No agents deployed</p>
          ) : (
            <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
              {deployments.map((deployment) => (
                <DeployedAgentCard
                  key={deployment.id}
                  name={deployment.name}
                  displayName={deployment.display_name}
                  deploymentId={deployment.id}
                  account={data.name}
                  href={deploymentPath(data.name, deployment.id)}
                  status={mapDeploymentStatus(deployment)}
                  requests={0}
                  lastActive="—"
                  installedAt={formatDate(deployment.created_at)}
                  updatedAt={formatDate(deployment.updated_at || deployment.created_at)}
                />
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-8">
        <h3 className="text-xl font-semibold">Agent blueprints</h3>
        <p className="text-sm text-muted-foreground mt-0.5">
          Agents published by this account
        </p>
        {accountBlueprints.length === 0 ? (
          <p className="text-muted-foreground mt-3">No agent blueprints published</p>
        ) : (
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
            {accountBlueprints.map((agent) => (
              <BlueprintCard
                key={agent.name}
                slug={`${data.name}/${agent.name}`}
                account={data.name}
                name={agent.name}
                description={getBlueprintDescription(agent)}
                visibility={agent.visibility}
                avatarUrl={agent.avatar_url}
                deployCount={agent.metrics?.deploy_count}

                onArchive={isMember ? () => {} : undefined}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default function AccountProfile() {
  return <AccountProfileContent />;
}
