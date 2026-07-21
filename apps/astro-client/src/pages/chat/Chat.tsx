import { useCallback, useMemo } from "react";
import { Navigate, useNavigate, useParams } from "react-router";
import type { Route } from "./+types/Chat";
import { useChatAgents } from "@/api/queries/chat";
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
  const { entries, totalDeployments, isLoading, isError, refetch } =
    useChatAgents(isAuthenticated);

  const eligibleDeploymentIds = useMemo(
    () => new Set(entries.map((e) => e.deployment.id)),
    [entries],
  );

  const activeEntry = entries.find((e) => e.deployment.id === deploymentId);
  const deployment = activeEntry?.deployment;
  // The chat page is cross-account, so the selected agent may live in a
  // different org than the globally-active one. Scope everything (identity link,
  // observability, members) to the deployment's own account, not activeAccount.
  const deploymentAccount = activeEntry?.account ?? activeAccount;

  // A new conversation goes to a blank chat with no conversation id in the URL.
  // The row is created server-side on the first send (createMessagingConversation
  // persists it before the SSE stream subscribes), and the URL is then updated to
  // the real id. Pre-seeding a client-generated id here would instead route the
  // send through the lazy-create path, where the stream can subscribe to the
  // not-yet-persisted conversation and get a 404 — hanging the turn until reload.
  const handleNewConversation = useCallback(() => {
    if (!deployment) return;
    navigate(chatDeploymentPath(deployment.id));
  }, [deployment, navigate]);

  if (!isLoading && entries.length > 0 && !deployment) {
    return <Navigate to={chatDeploymentPath(entries[0].deployment.id)} replace />;
  }

  const awaitingAgents = isLoading || entries.length === 0;

  // isError before count branches — failed reads report 0 and would wrongly show the deploy nudge.
  const emptyVariant = isLoading
    ? "loading"
    : isError
      ? "error"
      : totalDeployments > 0
        ? "agents-not-chattable"
        : "no-chat-agents";

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] overflow-hidden bg-background">
      {awaitingAgents ? (
        <ChatEmptyState
          className="flex-1"
          variant={emptyVariant}
          account={activeAccount}
          onRetry={refetch}
        />
      ) : (
        <ChatWorkspace
          className="flex-1"
          account={deploymentAccount}
          deploymentId={deployment!.id}
          deployment={deployment!}
          eligibleDeploymentIds={eligibleDeploymentIds}
          onNewConversation={handleNewConversation}
        />
      )}
    </div>
  );
}
