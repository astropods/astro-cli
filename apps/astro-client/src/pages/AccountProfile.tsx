import { useParams } from "react-router";
import { useAccount } from "../api/queries/accounts";
import { useDeployments } from "../api/queries/deployments";
import { useAuth } from "../lib/auth";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { AgentCard } from "../components/AgentCard";
import { useAccountAgents } from "../api/queries/agents";
import { getAgentDescription } from "../lib/agent-utils";
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

  const { data: agentsData } = useAccountAgents(data?.name ?? "", !!data);
  const accountAgents = agentsData?.agents ?? [];

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
        <UserAvatar handle={data.name} name={data.name} avatarVersion={data.avatar_version} className="size-16" />
        <div>
          {data.owner?.first_name && (
            <h1 className="text-2xl font-bold">
              {[data.owner.first_name, data.owner.last_name].filter(Boolean).join(" ")}
            </h1>
          )}
          <p className="text-muted-foreground">@{data.name}</p>
        </div>
      </div>

      {isMember && (
        <div className="mt-8">
          <h3 className="flex items-center gap-1.5 text-xl font-semibold">
            Installed agents
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
            <p className="text-muted-foreground mt-3">No agents installed</p>
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
                  updatedAt={formatDate(deployment.created_at)}
                />
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-8">
        <h3 className="text-xl font-semibold">Agent templates</h3>
        <p className="text-sm text-muted-foreground mt-0.5">
          Agents published by this account
        </p>
        {accountAgents.length === 0 ? (
          <p className="text-muted-foreground mt-3">No agent templates published</p>
        ) : (
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
            {accountAgents.map((agent) => (
              <AgentCard
                key={agent.name}
                slug={`${data.name}/${agent.name}`}
                account={data.name}
                name={agent.name}
                description={getAgentDescription(agent)}
                visibility={agent.visibility}
                lifetimeMessages={agent.metrics?.lifetime_messages}
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
