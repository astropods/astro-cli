import { useCallback, useMemo, type ReactNode } from "react";
import { DeploymentChatStreamingContext } from "@/components/chat/deployment-chat-streaming-context";
import {
  AssistantRuntimeProvider,
  useExternalStoreRuntime,
  type AppendMessage,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { useDeploymentChat } from "@/hooks/use-deployment-chat";
import { useDeploymentAgentConfig } from "@/api/queries/chat";
import { chatMessagesToThreadMessages } from "@/lib/messaging/chat-message-adapter";
import { dictationAdapter } from "@/lib/chat/dictation";
import { useApiClient } from "@/lib/api-context";
import {
  createDeploymentAttachmentAdapter,
  readAttachmentRef,
} from "@/lib/messaging/deployment-attachment-adapter";
import type { ChatAttachment } from "@/lib/api";

const convertMessage = (message: ThreadMessageLike) => message;

export function DeploymentChatRuntimeProvider({
  deploymentId,
  conversationId,
  onConversationCreated,
  children,
}: {
  deploymentId: string;
  conversationId?: string | null;
  onConversationCreated?: (conversationId: string) => void;
  children: ReactNode;
}) {
  const {
    messages,
    // The hook's active conversation id is stable for the turn: it becomes the
    // real id the moment the conversation is created (before the URL catches up
    // via onConversationCreated). The viewport keys on this, so a new chat's
    // blank→id URL transition doesn't remount the thread mid-reply and flicker it.
    conversationId: activeConversationId,
    isStreaming,
    assistantStreaming,
    historyLoading,
    streamError,
    sendMessage,
    cancelStream,
    hasMoreHistory,
    loadOlderMessages,
    pendingInteraction,
    clearPendingInteraction,
  } = useDeploymentChat(deploymentId, { conversationId, onConversationCreated });

  const threadMessages = useMemo(
    () => chatMessagesToThreadMessages(messages),
    [messages],
  );

  const streamingMessageId = useMemo(
    () => messages.find((m) => m.isStreaming)?.id ?? null,
    [messages],
  );

  // assistant-ui treats isRunning as "show an in-flight assistant turn". When the
  // tail is still the *previous* assistant it re-marks that message running
  // (loading dot above the new user bubble). Only signal running once the sent
  // user message is in history, or the server tail is a streaming assistant.
  const threadIsRunning = useMemo(() => {
    if (assistantStreaming != null) return true;
    if (!isStreaming) return false;
    return messages.at(-1)?.role === "user";
  }, [assistantStreaming, isStreaming, messages]);

  const api = useApiClient();
  const attachmentAdapter = useMemo(
    () => createDeploymentAttachmentAdapter(api, deploymentId),
    [api, deploymentId],
  );

  // Gate the composer's upload affordance on the agent actually supporting files:
  // the sidecar reports capabilities.files only when it has storage AND the agent
  // declared it consumes attachments. Absent capabilities (older sidecar / agent
  // that didn't opt in) reads as false, so the button stays hidden rather than
  // offering an upload the agent would silently ignore. The query is cached and
  // shared with the inspector's fetch, so this adds no extra request.
  const { data: agentConfig } = useDeploymentAgentConfig(deploymentId);
  const filesEnabled = agentConfig?.capabilities?.files === true;

  const onNew = useCallback(
    async (message: AppendMessage) => {
      // Join any text parts and collect uploaded-file references the composer's
      // attachment adapter stashed on each completed attachment.
      const text = message.content
        .filter((p): p is { type: "text"; text: string } => p.type === "text")
        .map((p) => p.text)
        .join("");
      const attachments = (message.attachments ?? [])
        .map((a) => readAttachmentRef(a))
        .filter((a): a is ChatAttachment => a !== null);
      if (!text && attachments.length === 0) return;
      await sendMessage(text, attachments);
    },
    [sendMessage],
  );

  // Browser-native dictation (Web Speech API) transcribes mic input into the
  // composer as text; the agent receives a normal text message. The shared
  // singleton is the same gate the composer uses to show the mic button, so the
  // two can't diverge; undefined (e.g. unsupported / SSR) registers no adapter.
  const runtime = useExternalStoreRuntime({
    messages: threadMessages,
    isRunning: threadIsRunning,
    isLoading: historyLoading && !!activeConversationId,
    onNew,
    onCancel: async () => {
      cancelStream();
    },
    convertMessage,
    adapters: {
      ...(dictationAdapter ? { dictation: dictationAdapter } : {}),
      ...(filesEnabled ? { attachments: attachmentAdapter } : {}),
    },
  });

  const viewportState = useMemo(
    () => ({
      streamingMessageId,
      conversationId: activeConversationId ?? null,
      historyLoading: historyLoading && !!activeConversationId,
      isStreaming: threadIsRunning,
      streamError,
      hasMoreHistory,
      loadOlderMessages,
      filesEnabled,
      pendingInteraction,
      clearPendingInteraction,
    }),
    [
      activeConversationId,
      hasMoreHistory,
      historyLoading,
      loadOlderMessages,
      streamError,
      streamingMessageId,
      threadIsRunning,
      filesEnabled,
      pendingInteraction,
      clearPendingInteraction,
    ],
  );

  return (
    <DeploymentChatStreamingContext.Provider value={viewportState}>
      <AssistantRuntimeProvider runtime={runtime}>{children}</AssistantRuntimeProvider>
    </DeploymentChatStreamingContext.Provider>
  );
}
