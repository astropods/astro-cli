import { useCallback, useMemo } from "react";
import { Navigate, useNavigate, useParams } from "react-router";
import type { Route } from "./+types/Chat";
import {
  useChatAgents,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import { ChatEmptyState } from "@/components/chat/ChatEmptyState";
import { ChatWorkspace } from "@/components/chat/ChatWorkspace";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { chatDeploymentPath } from "@/lib/routes";

export const meta: Route.MetaFunction = () => [{ title: "Chat | Astro" }];

export default function ChatPage() {
  const { isAuthenticated } = useAuth();
  const { activeAccount } = useActiveAccount();
  const { deploymentId } = useParams<{ deploymentId?: string }>();
  const navigate = useNavigate();

  // All chat-eligible agents across every account the user belongs to — the
  // page is cross-account, so switching agents (the thread-header dropdown) is
  // all the navigation needed; there is no account switcher and no left rail.
  const { entries, isLoading } = useChatAgents(isAuthenticated);

  const eligibleDeploymentIds = useMemo(
    () => new Set(entries.map((e) => e.deployment.id)),
    [entries],
  );

  const activeEntry = entries.find((e) => e.deployment.id === deploymentId);
  const deployment = activeEntry?.deployment;

  const upsertConversation = useUpsertDeploymentChatConversation(
    deployment?.id ?? "",
  );

  const handleNewConversation = useCallback(async () => {
    if (!deployment) return;
    const conversationId = crypto.randomUUID();
    await upsertConversation.mutateAsync({
      conversationId,
      title: "New conversation",
    });
    navigate(chatDeploymentPath(deployment.id, conversationId));
  }, [deployment, navigate, upsertConversation]);

  if (!isLoading && entries.length > 0 && !deployment) {
    return <Navigate to={chatDeploymentPath(entries[0].deployment.id)} replace />;
  }

  const awaitingAgents = isLoading || entries.length === 0;

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] overflow-hidden bg-background">
      {awaitingAgents ? (
        <ChatEmptyState
          className="flex-1"
          variant={isLoading ? "loading" : "no-chat-agents"}
          account={activeAccount}
        />
      ) : (
        <ChatWorkspace
          className="flex-1"
          deploymentId={deployment!.id}
          deployment={deployment!}
          eligibleDeploymentIds={eligibleDeploymentIds}
          onNewConversation={handleNewConversation}
        />
      )}
    </div>
  );
}
