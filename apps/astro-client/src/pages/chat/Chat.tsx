import { useMemo } from "react";
import { Navigate, useNavigate, useParams } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import type { Route } from "./+types/Chat";
import { useDeployments } from "@/api/queries/deployments";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { ChatEmptyState } from "@/components/chat/ChatEmptyState";
import { ChatWorkspace } from "@/components/chat/ChatWorkspace";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useUpsertDeploymentChatConversation } from "@/api/queries/chat";
import { isChatListEligible } from "@/lib/deployment-utils";
import { chatDeploymentPath } from "@/lib/routes";

// Desktop left column (16rem) — must match workspace aside; do not add flex-1 on md.
const SIDEBAR_MD = "md:w-64 md:max-w-64 md:shrink-0 md:flex-none";

export const meta: Route.MetaFunction = () => [{ title: "Chat | Astro" }];

export default function ChatPage() {
  const { isAuthenticated } = useAuth();
  const { activeAccount } = useActiveAccount();
  const { deploymentId } = useParams<{ deploymentId?: string }>();
  const navigate = useNavigate();

  const { data, isLoading } = useDeployments(activeAccount, isAuthenticated);
  const deployments = useMemo(
    () => (data?.deployments ?? []).filter(isChatListEligible),
    [data?.deployments],
  );

  const deployment = deployments.find((d) => d.id === deploymentId);
  const eligibleIds = useMemo(
    () => new Set(deployments.map((d) => d.id)),
    [deployments],
  );

  const upsertConversation = useUpsertDeploymentChatConversation(
    deployment?.id ?? "",
  );

  // Client-generated id (messaging treats it as opaque); persisted on the server first.
  const handleNewConversation = async () => {
    if (!deployment) return;
    const conversationId = crypto.randomUUID();
    await upsertConversation.mutateAsync({
      conversationId,
      title: "New conversation",
    });
    navigate(chatDeploymentPath(deployment.id, conversationId));
  };

  // Redirect /chat (or an unknown id) to the first eligible agent.
  if (!isLoading && deployments.length > 0 && !deployment) {
    return <Navigate to={chatDeploymentPath(deployments[0].id)} replace />;
  }

  const awaitingAgents = isLoading || deployments.length === 0;

  return (
    <div className="grid h-[calc(100dvh-3.5rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-background">
      <div className="flex shrink-0 flex-col border-b border-border bg-background md:h-12 md:flex-row md:items-stretch">
        <div
          className={`flex min-w-0 flex-1 items-center gap-2 overflow-hidden border-b border-border px-2 py-2 md:flex-none md:border-b-0 md:border-r md:py-0 ${SIDEBAR_MD}`}
        >
          {deployment ? (
            <div className="min-w-0 flex-1 overflow-hidden">
              <AgentDeploymentMenu
                account={activeAccount}
                deployment={deployment}
                eligibleDeploymentIds={eligibleIds}
                getDeploymentPath={(_acct, dep) => chatDeploymentPath(dep.id)}
              />
            </div>
          ) : (
            <span className="flex-1 px-1 text-sm font-semibold text-foreground">
              Chat
            </span>
          )}
          <Button
            type="button"
            size="icon"
            className="size-8 shrink-0"
            onClick={handleNewConversation}
            disabled={!deployment}
            aria-label="New conversation"
            title="New conversation"
          >
            <PlusIcon className="size-4" />
          </Button>
        </div>
        <div className="flex shrink-0 items-center justify-end px-3 py-2 md:flex-1 md:px-6 md:py-0">
          <PageScopeSwitcher />
        </div>
      </div>
      {awaitingAgents ? (
        <ChatEmptyState
          className="min-h-0"
          variant={isLoading ? "loading" : "no-chat-agents"}
          account={activeAccount}
        />
      ) : (
        <ChatWorkspace
          className="min-h-0"
          deploymentId={deployment!.id}
          deployment={deployment}
          sidebarMdWidth={SIDEBAR_MD}
        />
      )}
    </div>
  );
}
