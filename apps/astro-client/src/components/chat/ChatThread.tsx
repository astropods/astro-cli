import {
  useDeploymentRuntime,
  useDeploymentStatus,
} from "@/api/queries/deployments";
import { DeploymentChatRuntimeProvider } from "@/components/chat/DeploymentChatRuntimeProvider";
import { DeploymentChatThreadView } from "@/components/chat/DeploymentChatThreadView";
import { isChatEligible } from "@/lib/deployment-utils";
import type { AgentDeploymentSummary } from "@/lib/api";

export function ChatThread({
  account,
  deploymentId,
  deployment,
  conversationId,
  onConversationCreated,
}: {
  account: string;
  deploymentId: string;
  deployment?: AgentDeploymentSummary;
  conversationId?: string | null;
  onConversationCreated?: (conversationId: string, preview: string) => void;
}) {
  const { data: status } = useDeploymentStatus(deploymentId);
  const { data: runtimeData } = useDeploymentRuntime(deploymentId);

  const statusValue = status?.value;
  const messagingReachable = runtimeData?.runtime?.messaging_reachable ?? true;
  const canSend =
    isChatEligible(
      deployment ?? {
        id: deploymentId,
        name: "",
        build_id: "",
        created_at: "",
        messaging_web_configured: true,
      },
      statusValue,
    ) && messagingReachable;

  let disabledReason: string | undefined;
  if (statusValue && statusValue !== "active") {
    disabledReason = "Agent is not active yet.";
  } else if (!messagingReachable) {
    disabledReason = "Messaging endpoint is not reachable.";
  }

  const agentLabel =
    deployment?.display_name?.trim() ||
    deployment?.name?.trim() ||
    "your agent";

  return (
    <DeploymentChatRuntimeProvider
      deploymentId={deploymentId}
      conversationId={conversationId}
      onConversationCreated={onConversationCreated}
    >
      <DeploymentChatThreadView
        account={account}
        deploymentId={deploymentId}
        deployment={deployment}
        agentLabel={agentLabel}
        composerDisabled={!canSend}
        disabledReason={disabledReason}
      />
    </DeploymentChatRuntimeProvider>
  );
}
