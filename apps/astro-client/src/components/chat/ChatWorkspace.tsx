import { useCallback, useEffect, useRef } from "react";
import { ChevronLeftIcon } from "@heroicons/react/24/outline";
import { useSearchParams } from "react-router";
import type { AgentDeploymentSummary } from "@/lib/api";
import { useChatSessions } from "@/hooks/use-chat-sessions";
import {
  useDeleteDeploymentChatConversation,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { ChatSessionSidebar } from "./ChatSessionSidebar";
import { ChatThread } from "./ChatThread";

export function ChatWorkspace({
  deploymentId,
  deployment,
  sidebarMdWidth = "md:w-64 md:max-w-64 md:shrink-0 md:flex-none",
  className,
}: {
  deploymentId: string;
  deployment?: AgentDeploymentSummary;
  sidebarMdWidth?: string;
  className?: string;
}) {
  const [searchParams, setSearchParams] = useSearchParams();
  const conversationId = searchParams.get("conversation");

  const { sessions, recordFirstMessage, isLoading: sessionsLoading } =
    useChatSessions(deploymentId);
  const renameConversation = useUpsertDeploymentChatConversation(deploymentId);
  const deleteConversation = useDeleteDeploymentChatConversation(deploymentId);
  const autoSelectedRef = useRef(false);

  const setConversationId = useCallback(
    (id: string | null) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (id) next.set("conversation", id);
          else next.delete("conversation");
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  useEffect(() => {
    autoSelectedRef.current = false;
  }, [deploymentId]);

  // Open the most recent conversation when landing on /chat/:id without ?conversation=.
  useEffect(() => {
    if (conversationId) return;
    if (sessionsLoading || autoSelectedRef.current) return;
    const latest = sessions[0];
    if (!latest) return;
    autoSelectedRef.current = true;
    setConversationId(latest.conversationId);
  }, [
    conversationId,
    sessions,
    sessionsLoading,
    setConversationId,
  ]);

  const onConversationCreated = useCallback(
    async (convId: string, preview: string) => {
      await recordFirstMessage(convId, preview);
      if (conversationId !== convId) {
        setConversationId(convId);
      }
    },
    [conversationId, recordFirstMessage, setConversationId],
  );

  const onRenameSession = useCallback(
    (convId: string, title: string) => {
      renameConversation.mutate({ conversationId: convId, title });
    },
    [renameConversation],
  );

  const onDeleteSession = useCallback(
    (convId: string) => {
      deleteConversation.mutate(convId);
      if (conversationId === convId) {
        autoSelectedRef.current = true;
        setConversationId(null);
      }
    },
    [conversationId, deleteConversation, setConversationId],
  );

  return (
    <div className={cn("flex h-full min-h-0 overflow-hidden", className)}>
      <aside
        className={cn(
          "flex h-full min-h-0 flex-col overflow-hidden border-r border-border bg-background",
          sidebarMdWidth,
          "md:flex md:flex-none",
          conversationId ? "max-md:hidden" : "max-md:min-h-0 max-md:flex-1",
        )}
      >
        <ChatSessionSidebar
          sessions={sessions}
          activeConversationId={conversationId}
          onSelectSession={setConversationId}
          onRenameSession={onRenameSession}
          onDeleteSession={onDeleteSession}
        />
      </aside>
      <section
        className={cn(
          "flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background",
          conversationId ? "max-md:flex" : "max-md:hidden",
          "md:flex",
        )}
      >
        {conversationId ? (
          <div className="flex shrink-0 items-center border-b border-border px-2 py-1.5 md:hidden">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="gap-1.5"
              onClick={() => setConversationId(null)}
            >
              <ChevronLeftIcon className="size-4" aria-hidden />
              Conversations
            </Button>
          </div>
        ) : null}
        <ChatThread
          key={deploymentId}
          deploymentId={deploymentId}
          deployment={deployment}
          conversationId={conversationId}
          onConversationCreated={onConversationCreated}
        />
      </section>
    </div>
  );
}
