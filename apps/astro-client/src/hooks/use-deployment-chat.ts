import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useApiClient } from "@/lib/api-context";
import {
  ApiRequestError,
  type ChatAttachment,
  type GetDeploymentChatConversationResponse,
  type ListDeploymentChatConversationsResponse,
} from "@/lib/api";
import {
  deriveTurnInFlight,
  inFlightAssistantMessageId,
  mapServerMessages,
  serverTurnInFlight,
} from "@/lib/chat/message";
import { useDeploymentChatConversation } from "@/api/queries/chat";
import { chatKeys, fileKeys } from "@/api/queries/keys";
import {
  CHAT_INITIAL_PAGE_LIMIT,
  mergeConversationOlder,
  patchConversationAssistantChunk,
  patchConversationUserMessage,
  removeConversationMessage,
} from "@/lib/chat/conversation-sync";
import { openMessagingStream } from "@/lib/messaging/transport";
import { parseInteraction, type Interaction } from "@/lib/chat/interaction";

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
    onConversationCreated?: (conversationId: string) => void;
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
  // Live SSE interaction; the persisted queue (serverPending) is the reload source.
  const [liveInteraction, setLiveInteraction] = useState<{
    convId: string;
    interaction: Interaction;
  } | null>(null);
  // Answered-this-turn ids, suppressed until the refetch drops them (the query is
  // cache-served while the stream is open). A set so a multi-entry queue clears.
  const [resolvedInteractionIds, setResolvedInteractionIds] = useState<
    ReadonlySet<string>
  >(() => new Set());
  const assistantIdRef = useRef<string | null>(null);
  const sendLockRef = useRef(false);
  const sseActiveRef = useRef(false);

  const activeConversationId = conversationIdFromOptions ?? createdConversationId;
  const useTailPollRef = useRef(false);
  // Live SSE streams keyed by conversation. A stream's lifetime is scoped to its
  // turn, not to the active view: it stays open across conversation switches and
  // is closed only when the turn finishes or the hook unmounts. This lets a turn
  // complete and persist while a different conversation is on screen.
  const streamsRef = useRef<Map<string, EventSource>>(new Map());
  // Per-stream watchdog timers keyed by conversation. Armed when a stream opens,
  // cleared when its turn ends. If a turn never produces a finish event (a
  // stalled or reaped-but-not-closed sidecar generation), the timer finalizes
  // the stream so its EventSource is closed and a resend can open a fresh one.
  const streamTimersRef = useRef<Map<string, number>>(new Map());
  // Always-current active conversation, read by the long-lived stream callbacks.
  // They must compare against the conversation on screen now — not the one
  // captured when the stream opened — to tell whether a chunk/finish is for the
  // active view or a background one.
  const activeConversationIdRef = useRef(activeConversationId);
  activeConversationIdRef.current = activeConversationId;

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
    // A fresh conversation activates this query mid-send (its id lands the moment
    // the first turn starts), and react-query still runs an initial fetch for the
    // optimistically-seeded cache — a full replace that races the user row + live
    // SSE chunks and flickers the thread. While the stream is live the cache is
    // authoritative, so the query serves it instead of fetching.
    liveStreamRef: sseActiveRef,
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

  // Persisted pending queue, normalized through the same parse guard as the SSE path.
  const serverPending = useMemo<Interaction[]>(() => {
    const raw = serverData?.pending_interactions;
    if (!raw) return [];
    return raw
      .map((i) => parseInteraction(i))
      .filter((i): i is Interaction => i !== null);
  }, [serverData?.pending_interactions]);
  // Prefer the live interaction (the persisted queue lags mid-turn); skip answered ids.
  const pendingInteraction = useMemo<Interaction | null>(() => {
    if (
      liveInteraction &&
      liveInteraction.convId === activeConversationId &&
      !resolvedInteractionIds.has(liveInteraction.interaction.id)
    ) {
      return liveInteraction.interaction;
    }
    return serverPending.find((i) => !resolvedInteractionIds.has(i.id)) ?? null;
  }, [serverPending, liveInteraction, activeConversationId, resolvedInteractionIds]);

  const pendingInteractionRef = useRef<Interaction | null>(null);
  pendingInteractionRef.current = pendingInteraction;

  // Resolve via both sources: drop the live one, suppress the persisted id.
  const clearPendingInteraction = useCallback(() => {
    const resolved = pendingInteractionRef.current;
    if (resolved) {
      setResolvedInteractionIds((prev) => new Set(prev).add(resolved.id));
    }
    setLiveInteraction(null);
  }, []);

  // Drop suppressions the queue no longer lists (bounds the set; allows id reuse).
  useEffect(() => {
    setResolvedInteractionIds((prev) => {
      if (prev.size === 0) return prev;
      const live = new Set(serverPending.map((i) => i.id));
      const next = new Set([...prev].filter((id) => live.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [serverPending]);

  const historyLoading =
    !!activeConversationId &&
    (historyQueryLoading ||
      (historyFetching && messages.length === 0));

  const patchAssistantChunk = useCallback(
    (
      conversationId: string,
      content: string,
      chunkType?: string,
      attachments?: ChatAttachment[],
    ) => {
      const key = conversationKey(conversationId);
      const cached = readCachedThread(conversationId);
      const isActive = conversationId === activeConversationIdRef.current;

      let assistantId = isActive
        ? assistantIdRef.current
        : inFlightAssistantMessageId(cached);

      if (!assistantId || chunkType === "replace") {
        assistantId =
          inFlightAssistantMessageId(cached) ?? `assistant-${Date.now()}`;
        if (isActive) {
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
            attachments,
          );
        },
      );
    },
    [conversationKey, queryClient, readCachedThread],
  );

  const closeStream = useCallback((conversationId: string) => {
    streamsRef.current.get(conversationId)?.close();
    streamsRef.current.delete(conversationId);
    const timer = streamTimersRef.current.get(conversationId);
    if (timer !== undefined) {
      window.clearTimeout(timer);
      streamTimersRef.current.delete(conversationId);
    }
  }, []);

  const finalizeConversation = useCallback(
    (conversationId: string) => {
      // Close this turn's stream (possibly a background conversation's) and
      // cancel its watchdog timer.
      closeStream(conversationId);
      // Reset the on-screen streaming state only for the active conversation; a
      // background finish just marks that conversation's cached thread done.
      if (conversationId === activeConversationIdRef.current) {
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
      // Clear this conversation's "reply in progress" dot in the history list,
      // which is a separate query the per-conversation invalidation doesn't touch.
      queryClient.setQueryData<ListDeploymentChatConversationsResponse>(
        chatKeys.conversations(deploymentId),
        (old) =>
          old
            ? {
                ...old,
                conversations: old.conversations.map((c) =>
                  c.conversation_id === conversationId
                    ? { ...c, assistant_streaming: false }
                    : c,
                ),
              }
            : old,
      );
      // A finished turn may have written files; refresh the usage reading.
      void queryClient.invalidateQueries({
        queryKey: fileKeys.usage(deploymentId),
      });
    },
    [closeStream, conversationKey, deploymentId, queryClient],
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
    // Re-derive whether the active conversation has an open stream (it may have
    // one running in the background); the just-sent arbitration and the poll
    // gate read this.
    sseActiveRef.current = streamsRef.current.has(activeConversationId);
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
    setLiveInteraction(null);
    setResolvedInteractionIds(new Set());
  }, [conversationIdFromOptions, deploymentId]);

  useEffect(() => {
    useTailPollRef.current = turnInFlight && !sseActiveRef.current;
  }, [turnInFlight]);

  useEffect(() => {
    if (!activeConversationId || !turnInFlight) return;

    const convId = activeConversationId;
    sseActiveRef.current = true;
    useTailPollRef.current = false;
    // Don't open a second stream for a conversation already streaming (e.g. one
    // navigated back to mid-turn).
    if (streamsRef.current.has(convId)) return;
    const es = openMessagingStream(api, deploymentId, convId, {
      onChunk: (content, chunkType, attachments) =>
        patchAssistantChunk(convId, content, chunkType, attachments),
      onFinish: () => finalizeConversation(convId),
      onProtocolError: () => finalizeConversation(convId),
      onInteraction: (interaction) => setLiveInteraction({ convId, interaction }),
    });
    streamsRef.current.set(convId, es);
    // Watchdog: bound the turn's lifetime so a stall (no finish event ever
    // arrives) can't pin the stream open forever. This covers background turns
    // too — the stream outlives the active view, so the timer, not the on-screen
    // state, is what reaps a stalled turn. Cleared by finalizeConversation on a
    // normal finish; on fire it closes the stream so a resend opens a fresh one.
    const timer = window.setTimeout(() => {
      if (convId === activeConversationIdRef.current) {
        setStreamError("Response timed out. You can try sending again.");
        // Suppress this conversation's server streaming snapshot so the composer
        // unblocks; without it a persisted assistant_streaming with no live SSE
        // would keep turnInFlight deriving true. Cleared on the next send or a
        // conversation switch.
        setSuppressedConvId(convId);
      }
      finalizeConversation(convId);
    }, IN_FLIGHT_TIMEOUT_MS);
    streamTimersRef.current.set(convId, timer);
    // No cleanup here: the stream is closed by finalizeConversation when its turn
    // ends (which also cancels the timer above), or by the unmount effect below —
    // never on a conversation switch, so an in-flight turn keeps streaming in the
    // background.
  }, [
    activeConversationId,
    api,
    deploymentId,
    finalizeConversation,
    patchAssistantChunk,
    turnInFlight,
  ]);

  // Close every live stream and cancel its watchdog when the hook unmounts
  // (agent switch / chat closed).
  useEffect(() => {
    const streams = streamsRef.current;
    const timers = streamTimersRef.current;
    return () => {
      streams.forEach((es) => es.close());
      streams.clear();
      timers.forEach((t) => window.clearTimeout(t));
      timers.clear();
    };
  }, []);

  const sendMessage = useCallback(
    async (content: string, attachments?: ChatAttachment[]) => {
      const trimmed = content.trim();
      const hasAttachments = !!attachments && attachments.length > 0;
      // A turn needs text or at least one attachment (attach-and-send with no prose).
      if ((!trimmed && !hasAttachments) || sendLockRef.current || turnInFlight)
        return;
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
              attachments,
            }),
          );
          setCreatedConversationId(convId);
          await api.sendMessagingMessage(deploymentId, convId, trimmed, attachments);
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
                attachments,
              }),
          );
          await api.sendMessagingMessage(deploymentId, convId, trimmed, attachments);
        }

        onConversationCreated?.(convId);
      } catch (err) {
        applyInFlightState(undefined);
        if (convId) {
          // A stream may have opened before the send threw (existing
          // conversation, whose id is known synchronously); close it so a resend
          // isn't short-circuited by the "already streaming" guard into reusing a
          // dead stream.
          closeStream(convId);
          const key = conversationKey(convId);
          queryClient.setQueryData<GetDeploymentChatConversationResponse>(
            key,
            (old) =>
              old ? removeConversationMessage(old, userId) : old,
          );
        }
        // The message cap is a terminal per-conversation state, not a retryable
        // failure. Key on the sidecar's machine-readable error code rather than a
        // bare 409 (which can carry other, transient meanings) so a future
        // conflict isn't mislabeled as "permanently full". See the sidecar
        // contract in docs/04-guides/deployment-chat.md (HandleSendMessage).
        if (err instanceof ApiRequestError && err.code === "message_limit_reached") {
          toast.error("Conversation message limit reached", {
            description:
              "This chat has reached its message limit. Start a new chat to keep going.",
          });
        } else {
          setStreamError(
            err instanceof ApiRequestError
              ? err.message
              : "Failed to send message. Please try again.",
          );
        }
      } finally {
        sendLockRef.current = false;
      }
    },
    [
      activeConversationId,
      api,
      applyInFlightState,
      closeStream,
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
    pendingInteraction,
    clearPendingInteraction,
    loadOlderMessages,
    sendMessage,
    cancelStream,
  };
}
