import {
  useDeploymentRuntime,
  useDeploymentStatus,
} from "@/api/queries/deployments";
import { DeploymentChatRuntimeProvider } from "@/components/chat/DeploymentChatRuntimeProvider";
import { DeploymentChatThreadView } from "@/components/chat/DeploymentChatThreadView";
import { deriveChatComposerState } from "@/lib/deployment-utils";
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
  onConversationCreated?: (conversationId: string) => void;
}) {
  const { data: status } = useDeploymentStatus(deploymentId);
  const { data: runtimeData } = useDeploymentRuntime(deploymentId);

  const composerState = deriveChatComposerState(status, runtimeData?.runtime);

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
        composerState={composerState}
      />
    </DeploymentChatRuntimeProvider>
  );
}
