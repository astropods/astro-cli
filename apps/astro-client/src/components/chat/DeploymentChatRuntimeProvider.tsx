import { useCallback, useMemo, type ReactNode } from "react";
import { DeploymentChatStreamingContext } from "@/components/chat/deployment-chat-streaming-context";
import {
  AssistantRuntimeProvider,
  useExternalStoreRuntime,
  type AppendMessage,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { useDeploymentChat } from "@/hooks/use-deployment-chat";
import { chatMessagesToThreadMessages } from "@/lib/messaging/chat-message-adapter";
import { dictationAdapter } from "@/lib/chat/dictation";

const convertMessage = (message: ThreadMessageLike) => message;

export function DeploymentChatRuntimeProvider({
  deploymentId,
  conversationId,
  onConversationCreated,
  children,
}: {
  deploymentId: string;
  conversationId?: string | null;
  onConversationCreated?: (conversationId: string, preview: string) => void;
  children: ReactNode;
}) {
  const {
    messages,
    isStreaming,
    assistantStreaming,
    historyLoading,
    streamError,
    sendMessage,
    cancelStream,
    hasMoreHistory,
    loadOlderMessages,
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

  const onNew = useCallback(
    async (message: AppendMessage) => {
      const part = message.content[0];
      if (message.content.length !== 1 || part?.type !== "text") return;
      await sendMessage(part.text);
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
    isLoading: historyLoading && !!conversationId,
    onNew,
    onCancel: async () => {
      cancelStream();
    },
    convertMessage,
    adapters: dictationAdapter ? { dictation: dictationAdapter } : undefined,
  });

  const viewportState = useMemo(
    () => ({
      streamingMessageId,
      conversationId: conversationId ?? null,
      historyLoading: historyLoading && !!conversationId,
      isStreaming: threadIsRunning,
      streamError,
      hasMoreHistory,
      loadOlderMessages,
    }),
    [
      conversationId,
      hasMoreHistory,
      historyLoading,
      loadOlderMessages,
      streamError,
      streamingMessageId,
      threadIsRunning,
    ],
  );

  return (
    <DeploymentChatStreamingContext.Provider value={viewportState}>
      <AssistantRuntimeProvider runtime={runtime}>{children}</AssistantRuntimeProvider>
    </DeploymentChatStreamingContext.Provider>
  );
}
