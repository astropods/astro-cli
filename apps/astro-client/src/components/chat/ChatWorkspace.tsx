import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import type { AgentDeploymentSummary } from "@/lib/api";
import { useChatSessions } from "@/hooks/use-chat-sessions";
import {
  useDeleteDeploymentChatConversation,
  useSetDeploymentChatConversationTitle,
} from "@/api/queries/chat";
import { clearDraft } from "@/lib/chat/chat-draft";
import { SidePanel } from "@/components/ui/side-panel";
import { cn } from "@/lib/utils";
import { ChatThreadHeader } from "./ChatThreadHeader";
import { ChatThread } from "./ChatThread";
import { StorageCapacityBanner } from "@/components/StorageCapacityBanner";
import {
  ChatInspectorPanel,
  type ChatInspectorTab,
} from "./ChatInspectorPanel";

export function ChatWorkspace({
  account,
  deploymentId,
  deployment,
  eligibleDeploymentIds,
  onNewConversation,
  className,
}: {
  account: string;
  deploymentId: string;
  deployment: AgentDeploymentSummary;
  eligibleDeploymentIds: ReadonlySet<string>;
  onNewConversation?: () => void;
  className?: string;
}) {
  const [searchParams, setSearchParams] = useSearchParams();
  const conversationId = searchParams.get("conversation");
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<ChatInspectorTab>("overview");

  const { sessions, recordFirstMessage, isLoading: sessionsLoading } =
    useChatSessions(deploymentId);
  const renameConversation = useSetDeploymentChatConversationTitle(deploymentId);
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
    // Auto-select the latest conversation only once per agent, on first load with
    // no conversation in the URL. Mark it done as soon as sessions resolve — even
    // when a conversation is already selected — so a later deliberate "New chat"
    // (which clears the conversation from the URL) isn't bounced straight back to
    // the most recent thread.
    if (autoSelectedRef.current || sessionsLoading) return;
    autoSelectedRef.current = true;
    if (conversationId) return;
    const latest = sessions[0];
    if (latest) setConversationId(latest.conversationId);
  }, [conversationId, sessions, sessionsLoading, setConversationId]);

  // Plain handlers: ChatThread/ChatThreadHeader aren't memoized, so a stable
  // identity would buy nothing (and the mutation objects aren't stable refs
  // anyway). setConversationId stays memoized — it anchors the auto-select effect.
  const onConversationCreated = async (convId: string) => {
    await recordFirstMessage();
    if (conversationId !== convId) {
      setConversationId(convId);
    }
  };

  const onRenameSession = (convId: string, title: string) => {
    renameConversation.mutate({ conversationId: convId, title });
  };

  const onDeleteSession = (convId: string) => {
    deleteConversation.mutate(convId);
    // Drop the deleted conversation's draft so it doesn't linger in
    // sessionStorage (and can't resurrect if the id ever recurs).
    clearDraft(deploymentId, convId);
    if (conversationId === convId) {
      autoSelectedRef.current = true;
      setConversationId(null);
    }
  };

  return (
    <div
      className={cn(
        "chat-pane-bg flex h-full min-h-0 min-w-0 flex-1 overflow-hidden",
        className,
      )}
    >
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <ChatThreadHeader
          account={account}
          deployment={deployment}
          eligibleDeploymentIds={eligibleDeploymentIds}
          sessions={sessions}
          activeConversationId={conversationId}
          onSelectSession={setConversationId}
          onRenameSession={onRenameSession}
          onDeleteSession={onDeleteSession}
          onNewConversation={onNewConversation}
          inspectorOpen={inspectorOpen}
          onToggleInspector={() => setInspectorOpen((open) => !open)}
        />
        <StorageCapacityBanner deploymentId={deploymentId} className="px-3.5 pt-3.5" />
        <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <ChatThread
            key={deploymentId}
            account={account}
            deploymentId={deploymentId}
            deployment={deployment}
            conversationId={conversationId}
            onConversationCreated={onConversationCreated}
          />
        </section>
      </div>

      <SidePanel
        open={inspectorOpen}
        onClose={() => setInspectorOpen(false)}
        ariaLabel="Agent details"
      >
        <ChatInspectorPanel
          account={account}
          deploymentId={deploymentId}
          deployment={deployment}
          tab={inspectorTab}
          onTabChange={setInspectorTab}
          onClose={() => setInspectorOpen(false)}
        />
      </SidePanel>
    </div>
  );
}
