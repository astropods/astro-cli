import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import type { AgentDeploymentSummary } from "@/lib/api";
import { useChatSessions } from "@/hooks/use-chat-sessions";
import {
  useDeleteDeploymentChatConversation,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import { useIsMobile } from "@/hooks/use-compact-layout";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { ChatThreadHeader } from "./ChatThreadHeader";
import { ChatThread } from "./ChatThread";
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
  const isMobile = useIsMobile();
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<ChatInspectorTab>("overview");
  // Two-phase mount so the docked panel animates on both open and close: mount
  // collapsed, expand on the next frame; on close, collapse then unmount after
  // the transition (which also keeps its queries idle until it is opened).
  const [inspectorMounted, setInspectorMounted] = useState(false);
  const [inspectorEntered, setInspectorEntered] = useState(false);
  useEffect(() => {
    if (inspectorOpen) {
      setInspectorMounted(true);
      const raf = requestAnimationFrame(() => setInspectorEntered(true));
      return () => cancelAnimationFrame(raf);
    }
    setInspectorEntered(false);
    const timer = setTimeout(() => setInspectorMounted(false), 300);
    return () => clearTimeout(timer);
  }, [inspectorOpen]);

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

      {inspectorMounted && !isMobile ? (
        <aside
          aria-hidden={!inspectorOpen}
          className={cn(
            "flex shrink-0 flex-col overflow-hidden transition-[width] duration-300 ease-out motion-reduce:transition-none",
            inspectorEntered ? "w-[368px]" : "w-0",
          )}
        >
          <div
            className={cn(
              "m-3.5 flex w-[340px] min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border bg-surface transition-[transform,opacity] duration-300 ease-out motion-reduce:transition-none",
              inspectorEntered ? "translate-x-0 opacity-100" : "translate-x-3 opacity-0",
            )}
          >
            <ChatInspectorPanel
              account={account}
              deploymentId={deploymentId}
              deployment={deployment}
              tab={inspectorTab}
              onTabChange={setInspectorTab}
              onClose={() => setInspectorOpen(false)}
            />
          </div>
        </aside>
      ) : null}

      {isMobile ? (
        <Sheet open={inspectorOpen} onOpenChange={setInspectorOpen}>
          <SheetContent
            side="bottom"
            showCloseButton={false}
            className="h-[min(86dvh,760px)] max-h-[calc(100dvh-0.75rem)] gap-0 overflow-hidden rounded-t-2xl border-border bg-surface p-0 shadow-2xl"
          >
            <SheetTitle className="sr-only">Agent details</SheetTitle>
            <ChatInspectorPanel
              account={account}
              deploymentId={deploymentId}
              deployment={deployment}
              tab={inspectorTab}
              onTabChange={setInspectorTab}
              onClose={() => setInspectorOpen(false)}
            />
          </SheetContent>
        </Sheet>
      ) : null}
    </div>
  );
}
