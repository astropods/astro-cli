import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useApiClient } from "@/lib/api-context";
import { ApiRequestError } from "@/lib/api";
import type { ChatMessage } from "@/lib/chat/message";
import { openMessagingStream } from "@/lib/messaging/transport";

const IN_FLIGHT_TIMEOUT_MS = 3 * 60 * 1000;

/**
 * Platform deployment chat with in-session history only.
 *
 * TODO: Replace client-local message state with Langfuse-backed history from
 * astro-server once durable storage moves off Postgres.
 */
export function useDeploymentChat(
  deploymentId: string,
  options?: {
    conversationId?: string | null;
    onConversationCreated?: (conversationId: string, preview: string) => void;
  },
) {
  const api = useApiClient();
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

  const messages = useMemo(() => localMessages, [localMessages]);

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

  return {
    messages,
    conversationId: activeConversationId,
    isStreaming,
    assistantStreaming,
    streamError,
    historyLoading: false,
    sendMessage,
    cancelStream,
  };
}
