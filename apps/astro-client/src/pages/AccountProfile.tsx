import { useParams } from "react-router";
import { useAccount } from "../api/queries/accounts";
import { useDeployments } from "../api/queries/deployments";
import { useAuth } from "../lib/auth";
import { DeployedAgentCard } from "../components/DeployedAgentCard";
import { mapDeploymentStatus, formatDate } from "../lib/deployment-utils";

function AccountProfileContent() {
  const { account } = useParams<{ account: string }>();
  const { data, isLoading } = useAccount(account ?? "");
  const { isAuthenticated, accounts } = useAuth();

  const isOrg = data?.type === "organization";
  const isMember = isAuthenticated && accounts.some((a) => a.name === data?.name);

  const { data: deploymentsData } = useDeployments(
    data?.name ?? "",
    isOrg && isMember,
  );

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
      <h1 className="text-2xl font-bold">{data.name}</h1>
      <p className="text-muted-foreground mt-1">
        {isOrg ? "Organization profile" : "Personal account profile"}
      </p>

      {isOrg && isMember && (
        <div className="mt-8">
          <h2 className="text-lg font-semibold">Installed agents</h2>
          {deployments.length === 0 ? (
            <p className="text-muted-foreground mt-3">No agents installed</p>
          ) : (
            <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
              {deployments.map((deployment) => (
                <DeployedAgentCard
                  key={deployment.name}
                  name={deployment.name}
                  account={data.name}
                  href={`/${data.name}/${deployment.name}`}
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
    </div>
  );
}

export default function AccountProfile() {
  return <AccountProfileContent />;
}
