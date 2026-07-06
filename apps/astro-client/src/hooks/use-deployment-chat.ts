import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { ApiRequestError, type GetDeploymentChatConversationResponse } from "@/lib/api";
import {
  deriveTurnInFlight,
  inFlightAssistantMessageId,
  mapServerMessages,
  serverTurnInFlight,
} from "@/lib/chat/message";
import { useDeploymentChatConversation } from "@/api/queries/chat";
import { chatKeys } from "@/api/queries/keys";
import {
  CHAT_INITIAL_PAGE_LIMIT,
  mergeConversationOlder,
  patchConversationAssistantChunk,
  patchConversationUserMessage,
  removeConversationMessage,
} from "@/lib/chat/conversation-sync";
import { openMessagingStream } from "@/lib/messaging/transport";

const IN_FLIGHT_TIMEOUT_MS = 3 * 60 * 1000;

/**
 * Platform deployment chat.
 *
 * The TanStack query cache is the only message source. SSE chunks patch the
 * cache in place during a live turn; on finish the thread is invalidated so
 * persisted server ids replace temporary streaming ids.
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
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [streamingAssistantId, setStreamingAssistantId] = useState<
    string | null
  >(null);
  // Conversation the user explicitly stopped. While set, a lagging server
  // snapshot that still reports the turn in flight is ignored so the cancelled
  // turn can't be reopened. Cleared on the next send or a conversation switch.
  const [suppressedConvId, setSuppressedConvId] = useState<string | null>(null);
  const assistantIdRef = useRef<string | null>(null);
  const sendLockRef = useRef(false);
  const sseActiveRef = useRef(false);

  const activeConversationId = conversationIdFromOptions ?? createdConversationId;
  const useTailPollRef = useRef(false);

  const {
    data: serverData,
    isLoading: historyQueryLoading,
    isFetching: historyFetching,
  } = useDeploymentChatConversation(deploymentId, activeConversationId, {
    shouldPoll: (data) => {
      if (sseActiveRef.current) return false;
      return data ? serverTurnInFlight(data) : false;
    },
    useTailPollRef,
  });

  const conversationKey = useCallback(
    (conversationId: string) =>
      chatKeys.conversation(deploymentId, conversationId),
    [deploymentId],
  );

  const readCachedThread = useCallback(
    (conversationId: string) =>
      queryClient.getQueryData<GetDeploymentChatConversationResponse>(
        conversationKey(conversationId),
      ),
    [conversationKey, queryClient],
  );

  const applyInFlightState = useCallback(
    (thread: GetDeploymentChatConversationResponse | undefined) => {
      if (!thread || !serverTurnInFlight(thread)) {
        setIsStreaming(false);
        setStreamingAssistantId(null);
        assistantIdRef.current = null;
        sseActiveRef.current = false;
        useTailPollRef.current = false;
        return;
      }
      setIsStreaming(true);
      const assistantId = inFlightAssistantMessageId(thread);
      setStreamingAssistantId(assistantId);
      assistantIdRef.current = assistantId;
    },
    [],
  );

  const turnInFlight = useMemo(() => {
    // After an explicit stop, ignore a lagging server snapshot that still
    // reports the turn in flight — the chat store is eventually consistent and
    // would otherwise reopen the cancelled turn.
    if (activeConversationId && suppressedConvId === activeConversationId) {
      return false;
    }
    return deriveTurnInFlight({
      // A just-sent turn with an open SSE outranks an early "not in flight"
      // server snapshot (see deriveTurnInFlight). sseActiveRef is a ref, but
      // it's set synchronously in sendMessage before isStreaming flips, so the
      // isStreaming/serverData deps already cover every transition that matters.
      activeLocalTurn: isStreaming && sseActiveRef.current,
      serverThread: serverData ?? undefined,
      cachedThread: activeConversationId
        ? readCachedThread(activeConversationId)
        : undefined,
      isStreaming,
    });
  }, [
    activeConversationId,
    isStreaming,
    readCachedThread,
    serverData,
    suppressedConvId,
  ]);

  const activeStreamingMessageId = useMemo(() => {
    if (streamingAssistantId) return streamingAssistantId;
    return inFlightAssistantMessageId(serverData ?? undefined);
  }, [serverData, streamingAssistantId]);

  const messages = useMemo(
    () =>
      mapServerMessages(serverData?.messages ?? [], activeStreamingMessageId),
    [activeStreamingMessageId, serverData?.messages],
  );

  const historyLoading =
    !!activeConversationId &&
    (historyQueryLoading ||
      (historyFetching && messages.length === 0));

  const patchAssistantChunk = useCallback(
    (
      conversationId: string,
      content: string,
      chunkType?: string,
    ) => {
      const key = conversationKey(conversationId);
      const cached = readCachedThread(conversationId);

      let assistantId =
        conversationId === activeConversationId
          ? assistantIdRef.current
          : inFlightAssistantMessageId(cached);

      if (!assistantId || chunkType === "replace") {
        assistantId =
          inFlightAssistantMessageId(cached) ?? `assistant-${Date.now()}`;
        if (conversationId === activeConversationId) {
          assistantIdRef.current = assistantId;
          setStreamingAssistantId(assistantId);
        }
      }

      queryClient.setQueryData<GetDeploymentChatConversationResponse>(
        key,
        (old) => {
          if (!old) return old;
          return patchConversationAssistantChunk(
            old,
            assistantId,
            content,
            chunkType,
          );
        },
      );
    },
    [
      activeConversationId,
      conversationKey,
      queryClient,
      readCachedThread,
    ],
  );

  const finalizeConversation = useCallback(
    (conversationId: string) => {
      if (conversationId === activeConversationId) {
        assistantIdRef.current = null;
        setStreamingAssistantId(null);
        setIsStreaming(false);
        sseActiveRef.current = false;
        useTailPollRef.current = false;
      }
      const key = conversationKey(conversationId);
      queryClient.setQueryData<GetDeploymentChatConversationResponse>(
        key,
        (old) => (old ? { ...old, assistant_streaming: false } : old),
      );
      void queryClient.invalidateQueries({ queryKey: key });
    },
    [activeConversationId, conversationKey, queryClient],
  );

  const prevConversationRef = useRef<string | null>(null);
  const prevDeploymentRef = useRef(deploymentId);
  useLayoutEffect(() => {
    const deploymentChanged = prevDeploymentRef.current !== deploymentId;
    prevDeploymentRef.current = deploymentId;

    const prev = prevConversationRef.current;
    prevConversationRef.current = activeConversationId;

    if (!deploymentChanged && prev === activeConversationId) return;
    if (
      !deploymentChanged &&
      prev === null &&
      activeConversationId !== null
    ) {
      return;
    }

    if (deploymentChanged || prev !== activeConversationId) {
      setStreamError(null);
      // A different conversation is now active — drop any stop-suppression that
      // belonged to the previous one.
      setSuppressedConvId(null);
    }

    if (!activeConversationId) {
      applyInFlightState(undefined);
      return;
    }

    applyInFlightState(readCachedThread(activeConversationId));
  }, [
    activeConversationId,
    applyInFlightState,
    deploymentId,
    readCachedThread,
  ]);

  useEffect(() => {
    if (!activeConversationId || !serverData) return;
    const suppressed = suppressedConvId === activeConversationId;
    if (!suppressed && serverTurnInFlight(serverData)) {
      applyInFlightState(serverData);
    } else if (isStreaming && !sseActiveRef.current) {
      // Only let the server snapshot end the turn once no local SSE is open.
      // While the SSE is live (a just-sent turn), an early "not in flight"
      // snapshot is stale — the turn ends via the SSE finish/error or the
      // in-flight timeout, both of which clear sseActiveRef first. A suppressed
      // (explicitly stopped) conversation never reactivates from the snapshot.
      applyInFlightState(undefined);
    }
  }, [
    activeConversationId,
    applyInFlightState,
    isStreaming,
    serverData,
    suppressedConvId,
  ]);

  useEffect(() => {
    setCreatedConversationId(null);
  }, [conversationIdFromOptions, deploymentId]);

  useEffect(() => {
    useTailPollRef.current = turnInFlight && !sseActiveRef.current;
  }, [turnInFlight]);

  useEffect(() => {
    if (!turnInFlight) return;
    const timeout = window.setTimeout(() => {
      setStreamError("Response timed out. You can try sending again.");
      applyInFlightState(undefined);
    }, IN_FLIGHT_TIMEOUT_MS);
    return () => window.clearTimeout(timeout);
  }, [applyInFlightState, turnInFlight]);

  useEffect(() => {
    if (!activeConversationId || !turnInFlight) return;

    const convId = activeConversationId;
    sseActiveRef.current = true;
    useTailPollRef.current = false;
    const es = openMessagingStream(api, deploymentId, convId, {
      onChunk: (content, chunkType) =>
        patchAssistantChunk(convId, content, chunkType),
      onFinish: () => finalizeConversation(convId),
      onProtocolError: () => finalizeConversation(convId),
    });
    return () => {
      es.close();
      sseActiveRef.current = false;
    };
  }, [
    activeConversationId,
    api,
    deploymentId,
    finalizeConversation,
    patchAssistantChunk,
    turnInFlight,
  ]);

  const sendMessage = useCallback(
    async (content: string) => {
      const trimmed = content.trim();
      if (!trimmed || sendLockRef.current || turnInFlight) return;
      sendLockRef.current = true;

      setStreamError(null);
      // A fresh send lifts any prior stop-suppression for this conversation.
      setSuppressedConvId(null);
      assistantIdRef.current = null;
      setStreamingAssistantId(null);
      setIsStreaming(true);
      sseActiveRef.current = true;
      useTailPollRef.current = false;

      const userId = `user-${Date.now()}`;
      let convId = activeConversationId;

      try {
        if (!convId) {
          const created = await api.createMessagingConversation(deploymentId);
          convId = created.conversation_id;
          queryClient.setQueryData(
            conversationKey(convId),
            patchConversationUserMessage(undefined, convId, {
              id: userId,
              role: "user",
              content: trimmed,
            }),
          );
          setCreatedConversationId(convId);
          await api.sendMessagingMessage(deploymentId, convId, trimmed);
        } else {
          const key = conversationKey(convId);
          await queryClient.cancelQueries({ queryKey: key });
          queryClient.setQueryData<GetDeploymentChatConversationResponse>(
            key,
            (old) =>
              patchConversationUserMessage(old, convId!, {
                id: userId,
                role: "user",
                content: trimmed,
              }),
          );
          await api.sendMessagingMessage(deploymentId, convId, trimmed);
        }

        onConversationCreated?.(convId, trimmed);
      } catch (err) {
        applyInFlightState(undefined);
        if (convId) {
          const key = conversationKey(convId);
          queryClient.setQueryData<GetDeploymentChatConversationResponse>(
            key,
            (old) =>
              old ? removeConversationMessage(old, userId) : old,
          );
        }
        setStreamError(
          err instanceof ApiRequestError
            ? err.message
            : "Failed to send message. Please try again.",
        );
      } finally {
        sendLockRef.current = false;
      }
    },
    [
      activeConversationId,
      api,
      applyInFlightState,
      conversationKey,
      deploymentId,
      onConversationCreated,
      queryClient,
      turnInFlight,
    ],
  );

  const cancelStream = useCallback(() => {
    const convId = activeConversationId;
    if (convId) {
      // Suppress reopen before tearing down so a lagging in-flight snapshot from
      // the refetch below can't resurrect the turn.
      setSuppressedConvId(convId);
      // Best-effort: ask the sidecar to stop generating. The local teardown
      // below ends the turn for the user regardless of this request's outcome,
      // but a breadcrumb makes a systematically failing /cancel observable
      // (e.g. the sidecar route regressed) rather than silently swallowed.
      void api.cancelMessagingStream(deploymentId, convId).catch((err) => {
        console.warn("[use-deployment-chat] cancelMessagingStream failed", err);
      });
    }
    applyInFlightState(undefined);
    if (convId) {
      finalizeConversation(convId);
    }
  }, [
    activeConversationId,
    api,
    deploymentId,
    applyInFlightState,
    finalizeConversation,
  ]);

  const loadOlderMessages = useCallback(async () => {
    if (!activeConversationId || !serverData?.has_more || !serverData.oldest_seq) {
      return;
    }
    const key = conversationKey(activeConversationId);
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
    conversationKey,
    deploymentId,
    queryClient,
    serverData,
  ]);

  return {
    messages,
    conversationId: activeConversationId,
    isStreaming: turnInFlight,
    assistantStreaming: activeStreamingMessageId,
    streamError,
    historyLoading: historyLoading && !!activeConversationId,
    hasMoreHistory: !!serverData?.has_more,
    loadOlderMessages,
    sendMessage,
    cancelStream,
  };
}
