import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { ApiRequestError, type GetDeploymentChatConversationResponse } from "@/lib/api";
import {
  type ChatMessage,
  mapServerMessages,
  mergeLocalAndServerMessages,
} from "@/lib/chat/message";
import { useDeploymentChatConversation } from "@/api/queries/chat";
import { chatKeys } from "@/api/queries/keys";
import {
  CHAT_INITIAL_PAGE_LIMIT,
  mergeConversationOlder,
} from "@/lib/chat/conversation-sync";
import { openMessagingStream } from "@/lib/messaging/transport";

const IN_FLIGHT_TIMEOUT_MS = 3 * 60 * 1000;

/**
 * Platform deployment chat.
 *
 * Persisted history is hydrated from astro-server (Langfuse-backed, keyed by
 * conversation id) when a conversation is opened. Live turns are appended to
 * local state and streamed via SSE; a just-sent turn stays local until its
 * Langfuse trace lands, at which point it is de-duplicated against the server
 * snapshot by (role, content).
 */
export function useDeploymentChat(
  deploymentId: string,
  options?: {
    conversationId?: string | null;
    onConversationCreated?: (conversationId: string, preview: string) => void;
  },
) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  const conversationIdFromOptions = options?.conversationId ?? null;
  const onConversationCreated = options?.onConversationCreated;

  const [createdConversationId, setCreatedConversationId] = useState<
    string | null
  >(null);
  const [localMessages, setLocalMessages] = useState<ChatMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const assistantIdRef = useRef<string | null>(null);
  const sendLockRef = useRef(false);

  const activeConversationId = conversationIdFromOptions ?? createdConversationId;

  // Persisted history (Postgres + Langfuse). Refetches when stale or while a
  // turn is in flight (see chat.ts query options).
  const {
    data: serverData,
    isLoading: historyQueryLoading,
    isFetching: historyFetching,
  } = useDeploymentChatConversation(deploymentId, activeConversationId);

  const serverMessages = useMemo(
    () => mapServerMessages(serverData?.messages ?? [], null),
    [serverData?.messages],
  );

  const historyLoading =
    !!activeConversationId &&
    (historyQueryLoading ||
      (historyFetching && serverMessages.length === 0));

  const appendAssistantChunk = useCallback((content: string, chunkType?: string) => {
    setLocalMessages((prev) => {
      const next = [...prev];
      const assistantId = assistantIdRef.current;
      const idx =
        assistantId != null
          ? next.findIndex((m) => m.id === assistantId)
          : -1;

      if (chunkType === "replace" || idx < 0) {
        const id = assistantId ?? `assistant-${Date.now()}`;
        assistantIdRef.current = id;
        if (idx >= 0) {
          next[idx] = { id, role: "assistant", content, isStreaming: true };
        } else {
          next.push({ id, role: "assistant", content, isStreaming: true });
        }
        return next;
      }

      const existing = next[idx];
      next[idx] = {
        ...existing,
        content: existing.content + content,
        isStreaming: true,
      };
      return next;
    });
  }, []);

  const finalizeAssistant = useCallback(() => {
    const assistantId = assistantIdRef.current;
    if (assistantId) {
      setLocalMessages((prev) =>
        prev.map((m) =>
          m.id === assistantId ? { ...m, isStreaming: false } : m,
        ),
      );
    }
    assistantIdRef.current = null;
    setIsStreaming(false);
  }, []);

  const prevConversationRef = useRef<string | null>(null);
  const prevDeploymentRef = useRef(deploymentId);
  useEffect(() => {
    const deploymentChanged = prevDeploymentRef.current !== deploymentId;
    prevDeploymentRef.current = deploymentId;

    const prev = prevConversationRef.current;
    prevConversationRef.current = activeConversationId;

    if (!deploymentChanged && prev === activeConversationId) return;
    // Lazy-create goes null → new id in one send; keep messages already appended.
    if (
      !deploymentChanged &&
      prev === null &&
      activeConversationId !== null
    ) {
      return;
    }

    setLocalMessages([]);
    assistantIdRef.current = null;
    setIsStreaming(false);
    setStreamError(null);
  }, [activeConversationId, deploymentId]);

  useEffect(() => {
    setCreatedConversationId(null);
  }, [conversationIdFromOptions, deploymentId]);

  useEffect(() => {
    if (!isStreaming) return;
    const timeout = window.setTimeout(() => {
      setStreamError("Response timed out. You can try sending again.");
      setIsStreaming(false);
      assistantIdRef.current = null;
    }, IN_FLIGHT_TIMEOUT_MS);
    return () => window.clearTimeout(timeout);
  }, [isStreaming]);

  useEffect(() => {
    if (!isStreaming || !activeConversationId) return;

    const es = openMessagingStream(api, deploymentId, activeConversationId, {
      onChunk: appendAssistantChunk,
      onFinish: finalizeAssistant,
      onProtocolError: finalizeAssistant,
    });
    return () => es.close();
  }, [
    activeConversationId,
    api,
    appendAssistantChunk,
    deploymentId,
    finalizeAssistant,
    isStreaming,
  ]);

  // Server history first, then local turns not yet reflected server-side. Once
  // the server snapshot catches up with the trailing local turns (Langfuse
  // trace landed + a refetch), those turns are dropped to avoid duplicates.
  // Matching is anchored to the tail (in order) so a repeated identical message
  // earlier in history is never mistaken for an unconfirmed local turn.
  const messages = useMemo(
    () => mergeLocalAndServerMessages(serverMessages, localMessages),
    [serverMessages, localMessages],
  );

  const sendMessage = useCallback(
    async (content: string) => {
      const trimmed = content.trim();
      if (!trimmed || sendLockRef.current || isStreaming) return;
      sendLockRef.current = true;

      setStreamError(null);
      assistantIdRef.current = null;
      setIsStreaming(true);

      const userId = `user-${Date.now()}`;
      setLocalMessages((prev) => [
        ...prev,
        { id: userId, role: "user", content: trimmed },
      ]);

      try {
        let convId = activeConversationId;
        if (!convId) {
          const created = await api.createMessagingConversation(deploymentId);
          convId = created.conversation_id;
          setCreatedConversationId(convId);
        }

        await api.sendMessagingMessage(deploymentId, convId, trimmed);
        onConversationCreated?.(convId, trimmed);
      } catch (err) {
        setIsStreaming(false);
        assistantIdRef.current = null;
        setStreamError(
          err instanceof ApiRequestError
            ? err.message
            : "Failed to send message. Please try again.",
        );
        setLocalMessages((prev) => prev.filter((m) => m.id !== userId));
      } finally {
        sendLockRef.current = false;
      }
    },
    [
      activeConversationId,
      api,
      deploymentId,
      isStreaming,
      onConversationCreated,
    ],
  );

  const cancelStream = useCallback(() => {
    setIsStreaming(false);
    assistantIdRef.current = null;
    finalizeAssistant();
  }, [finalizeAssistant]);

  const assistantStreaming = useMemo(() => {
    if (!isStreaming) return null;
    const tail = messages[messages.length - 1];
    return tail?.role === "assistant" ? tail.id : null;
  }, [isStreaming, messages]);

  const loadOlderMessages = useCallback(async () => {
    if (!activeConversationId || !serverData?.has_more || !serverData.oldest_seq) {
      return;
    }
    const key = chatKeys.conversation(deploymentId, activeConversationId);
    const existing =
      queryClient.getQueryData<GetDeploymentChatConversationResponse>(key) ??
      serverData;
    if (!existing.has_more || !existing.oldest_seq) return;

    const older = await api.getDeploymentChatConversation(
      deploymentId,
      activeConversationId,
      {
        limit: CHAT_INITIAL_PAGE_LIMIT,
        before_seq: existing.oldest_seq,
      },
    );
    queryClient.setQueryData(key, mergeConversationOlder(existing, older));
  }, [
    activeConversationId,
    api,
    deploymentId,
    queryClient,
    serverData,
  ]);

  return {
    messages,
    conversationId: activeConversationId,
    isStreaming,
    assistantStreaming,
    streamError,
    historyLoading: historyLoading && !!activeConversationId,
    hasMoreHistory: !!serverData?.has_more,
    loadOlderMessages,
    sendMessage,
    cancelStream,
  };
}
