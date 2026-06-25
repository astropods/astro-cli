import { useCallback, useEffect, useRef } from "react";
import { useSearchParams } from "react-router";
import type { AgentDeploymentSummary } from "@/lib/api";
import { useChatSessions } from "@/hooks/use-chat-sessions";
import {
  useDeleteDeploymentChatConversation,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import { cn } from "@/lib/utils";
import { ChatThreadHeader } from "./ChatThreadHeader";
import { ChatThread } from "./ChatThread";

export function ChatWorkspace({
  deploymentId,
  deployment,
  eligibleDeploymentIds,
  onNewConversation,
  className,
}: {
  deploymentId: string;
  deployment: AgentDeploymentSummary;
  eligibleDeploymentIds: ReadonlySet<string>;
  onNewConversation?: () => void;
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

  useEffect(() => {
    if (conversationId) return;
    if (sessionsLoading || autoSelectedRef.current) return;
    const latest = sessions[0];
    if (!latest) return;
    autoSelectedRef.current = true;
    setConversationId(latest.conversationId);
  }, [conversationId, sessions, sessionsLoading, setConversationId]);

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
    <div
      className={cn(
        "flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background",
        className,
      )}
    >
      <ChatThreadHeader
        deployment={deployment}
        eligibleDeploymentIds={eligibleDeploymentIds}
        sessions={sessions}
        activeConversationId={conversationId}
        onSelectSession={setConversationId}
        onRenameSession={onRenameSession}
        onDeleteSession={onDeleteSession}
        onNewConversation={onNewConversation}
      />
      <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <ChatThread
          key={`${deploymentId}:${conversationId ?? "draft"}`}
          account={account}
          deploymentId={deploymentId}
          deployment={deployment}
          conversationId={conversationId}
          onConversationCreated={onConversationCreated}
        />
      </section>
    </div>
  );
}
