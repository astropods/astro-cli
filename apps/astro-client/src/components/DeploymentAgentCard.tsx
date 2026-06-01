import { DeployedAgentCard } from "@/components/DeployedAgentCard";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { getMessagingEndpoint } from "@/lib/deployment-utils";
import type { AgentDeploymentSummary } from "@/lib/api";

// Thin adapter: maps an AgentDeployment row into the neutral props that
// DeployedAgentCard takes. Kept separate so the card stays decoupled from
// the server-side AgentDeployment shape and stories can keep mocking
// primitives.
export function DeploymentAgentCard({
  deployment,
  account,
  requestSeries,
  tokenSeries,
  onDeleteRequest,
}: {
  deployment: AgentDeploymentSummary;
  account: string;
  requestSeries?: number[];
  tokenSeries?: number[];
  onDeleteRequest?: () => void;
}) {
  const bust = useDeploymentAvatarBust(deployment.id);
  const avatarUrl = bust ?? getDeploymentAvatarUrl(deployment.id);
  const messaging = getMessagingEndpoint(deployment);
  const launchUrl = deployment.status === "Running" ? messaging?.url : undefined;
  const hasUpdateAvailable =
    !!deployment.latest_build_id && deployment.latest_build_id !== deployment.build_id;
  const hasError = deployment.status === "error";

  return (
    <DeployedAgentCard
      account={account}
      name={deployment.name}
      displayName={deployment.display_name}
      deploymentId={deployment.id}
      avatarUrl={avatarUrl}
      avatarColors={deployment.avatar_colors}
      requestSeries={requestSeries}
      tokenSeries={tokenSeries}
      launchUrl={launchUrl}
      hasError={hasError}
      hasUpdateAvailable={hasUpdateAvailable}
      latestBuildId={deployment.latest_build_id}
      onDeleteRequest={onDeleteRequest}
    />
  );
}
