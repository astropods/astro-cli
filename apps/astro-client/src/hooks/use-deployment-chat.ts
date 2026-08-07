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
  type ChatRole,
  deriveTurnInFlight,
  inFlightAssistantMessageId,
  mapServerMessages,
  serverTurnInFlight,
} from "@/lib/chat/message";
import { useDeploymentChatConversation } from "@/api/queries/chat";
import { chatKeys, fileKeys } from "@/api/queries/keys";
import {
  CHAT_INITIAL_PAGE_LIMIT,
  appendConversationMessage,
  mergeConversationOlder,
  patchConversationAssistantChunk,
  removeConversationMessage,
} from "@/lib/chat/conversation-sync";
import { openMessagingStream } from "@/lib/messaging/transport";
import { parseInteraction, type Interaction } from "@/lib/chat/interaction";

// Transport backstop: the server is authoritative for turn termination (it emits
// a terminal finish/error on finish, agent error, disconnect, or stall), so this
// only guards a dead SSE pipe. Reset on any inbound event (chunks + 30s
// heartbeats); fires only after this long of total silence.
const STREAM_LIVENESS_TIMEOUT_MS = 90 * 1000;
// Content-stall cap: defense-in-depth against a sidecar that keeps heartbeating
// (so the liveness watchdog never fires) but produces no content and never sends
// a terminal event, which would otherwise pin the composer open forever. Reset
// only by content chunks (not heartbeats), so a healthy turn that keeps streaming
// is never cut off, while a heartbeat-only zombie is still reaped after this long
// with no content. Comfortably above the server's 5-min idle window.
const CONTENT_STALL_TIMEOUT_MS = 15 * 60 * 1000;

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
  // Per-stream liveness watchdog timers keyed by conversation. Armed on stream
  // open and reset on any inbound activity; if the SSE pipe goes fully silent
  // (no chunks, no heartbeats) the timer finalizes the stream so its EventSource
  // is closed and a resend can open a fresh one.
  const streamTimersRef = useRef<Map<string, number>>(new Map());
  // Per-stream content-stall caps keyed by conversation. Armed on stream open and
  // reset on each content chunk (not heartbeats) — the no-progress backstop
  // (see CONTENT_STALL_TIMEOUT_MS).
  const streamStallTimersRef = useRef<Map<string, number>>(new Map());
  // Terminal errors that arrived for a background (off-screen) conversation,
  // keyed by conversation. Surfaced when the user returns to it instead of being
  // silently dropped (the composer would otherwise just re-arm with no reason).
  const backgroundErrorsRef = useRef<Map<string, string>>(new Map());
  // Latest armStallTimer, so clearPendingInteraction (defined above it) can re-arm
  // the content-stall cap when the user answers an interaction.
  const armStallTimerRef = useRef<(conversationId: string) => void>(() => {});
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
    // The user answered: re-arm the content-stall cap (paused while the interaction
    // was pending) so the agent's resume is still bounded if it produces no content.
    const convId = activeConversationIdRef.current;
    if (convId && streamsRef.current.has(convId)) {
      armStallTimerRef.current(convId);
    }
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

      // A null pointer opens a fresh assistant bubble — at turn start, and after a note lands (a non-assistant tail), which is how the continuation breaks into its own bubble.
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
    const stallTimer = streamStallTimersRef.current.get(conversationId);
    if (stallTimer !== undefined) {
      window.clearTimeout(stallTimer);
      streamStallTimersRef.current.delete(conversationId);
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

  // Surface a turn's terminal error. On the active conversation it shows the
  // message and suppresses the lagging server snapshot so the composer unblocks;
  // for a background conversation it's stashed and surfaced when the user returns
  // (see the conversation-switch effect), so an off-screen failure isn't dropped.
  const surfaceTurnError = useCallback(
    (conversationId: string, message: string) => {
      if (conversationId === activeConversationIdRef.current) {
        setStreamError(message);
        setSuppressedConvId(conversationId);
      } else {
        backgroundErrorsRef.current.set(conversationId, message);
      }
    },
    [],
  );

  // Arm (or re-arm) a conversation's liveness watchdog. Reset on any inbound SSE
  // activity; fires only when the pipe goes fully silent (dead transport).
  const armWatchdog = useCallback(
    (conversationId: string) => {
      const existing = streamTimersRef.current.get(conversationId);
      if (existing !== undefined) window.clearTimeout(existing);
      const timer = window.setTimeout(() => {
        surfaceTurnError(
          conversationId,
          "Connection lost. You can try sending again.",
        );
        finalizeConversation(conversationId);
      }, STREAM_LIVENESS_TIMEOUT_MS);
      streamTimersRef.current.set(conversationId, timer);
    },
    [finalizeConversation, surfaceTurnError],
  );

  // Arm (or reset) a conversation's content-stall cap. Reset on each content
  // chunk (see the stream's onChunk), so a turn that keeps streaming is never cut
  // off; a sidecar that only heartbeats but produces no content can't pin the
  // composer open past this window. Sits alongside the liveness watchdog.
  const armStallTimer = useCallback(
    (conversationId: string) => {
      const existing = streamStallTimersRef.current.get(conversationId);
      if (existing !== undefined) window.clearTimeout(existing);
      const timer = window.setTimeout(() => {
        surfaceTurnError(
          conversationId,
          "The agent stopped producing output. You can try sending again.",
        );
        finalizeConversation(conversationId);
      }, CONTENT_STALL_TIMEOUT_MS);
      streamStallTimersRef.current.set(conversationId, timer);
    },
    [finalizeConversation, surfaceTurnError],
  );
  armStallTimerRef.current = armStallTimer;

  const prevConversationRef = useRef<string | null>(null);
  const prevDeploymentRef = useRef(deploymentId);
  useLayoutEffect(() => {
    const deploymentChanged = prevDeploymentRef.current !== deploymentId;
    prevDeploymentRef.current = deploymentId;

    const prev = prevConversationRef.current;
    prevConversationRef.current = activeConversationId;

    if (deploymentChanged || prev !== activeConversationId) {
      // Surface a terminal error that arrived while this conversation was in the
      // background (else clear the previous conversation's error/stop-suppression).
      // Runs before the early-returns below so it also covers a null -> conversation
      // switch, which those returns skip — otherwise the stashed error is dropped
      // and the composer re-arms with no reason.
      const bgError =
        activeConversationId !== null
          ? backgroundErrorsRef.current.get(activeConversationId)
          : undefined;
      if (bgError !== undefined) {
        backgroundErrorsRef.current.delete(activeConversationId!);
        setStreamError(bgError);
        setSuppressedConvId(activeConversationId);
      } else {
        setStreamError(null);
        setSuppressedConvId(null);
      }
    }

    if (!deploymentChanged && prev === activeConversationId) return;
    if (
      !deploymentChanged &&
      prev === null &&
      activeConversationId !== null
    ) {
      return;
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

  // Drop stashed background errors when the deployment changes — their
  // conversations are no longer reachable here, so bound the map to one
  // deployment's conversations rather than the hook's whole lifetime.
  useEffect(() => {
    backgroundErrorsRef.current.clear();
  }, [deploymentId]);

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
      onChunk: (content, chunkType, attachments) => {
        // Content is real progress: reset the stall cap so a turn that keeps
        // streaming is never cut off (heartbeats, via onActivity, do not).
        armStallTimer(convId);
        patchAssistantChunk(convId, content, chunkType, attachments);
      },
      onFinish: () => finalizeConversation(convId),
      onError: (message) => {
        surfaceTurnError(convId, message);
        finalizeConversation(convId);
      },
      // Reset the liveness watchdog on any inbound activity (chunks + heartbeats).
      onActivity: () => armWatchdog(convId),
      onInteraction: (interaction) => {
        // A pending interaction parks the turn on the user, not the agent: pause
        // the content-stall cap so a long human pause isn't read as a stall. The
        // next content chunk (the agent resuming after the reply) re-arms it.
        const stall = streamStallTimersRef.current.get(convId);
        if (stall !== undefined) {
          window.clearTimeout(stall);
          streamStallTimersRef.current.delete(convId);
        }
        setLiveInteraction({ convId, interaction });
      },
      onInjected: (id, role, content) => {
        // A server-injected row the client didn't send: a resolved-interaction note (grey line) or a "write your own reply" (user bubble). Append it and clear the streaming pointer so it becomes a non-assistant tail and the continuation opens a fresh bubble; the turn stays in flight.
        const messageId = id || `${role}-${Date.now()}`;
        queryClient.setQueryData<GetDeploymentChatConversationResponse>(
          conversationKey(convId),
          (old) => {
            // Idempotent by id: a redelivered event after a reconnect must not double the row.
            if (old?.messages?.some((m) => m.id === messageId)) return old;
            return appendConversationMessage(old, convId, {
              id: messageId,
              role: role as ChatRole,
              content,
            });
          },
        );
        // Only reset the on-screen streaming pointers for the active conversation (a background row must not clobber the active view).
        if (convId === activeConversationIdRef.current) {
          assistantIdRef.current = null;
          setStreamingAssistantId(null);
        }
        armStallTimer(convId);
      },
    });
    streamsRef.current.set(convId, es);
    // Arm the liveness watchdog for the pipe (reset by any activity) and the
    // content-stall cap (reset only by content chunks). Both cover background
    // turns too: the stream outlives the active view, so the timers, not the
    // on-screen state, reap a stuck turn.
    armWatchdog(convId);
    armStallTimer(convId);
    // No cleanup here: the stream is closed by finalizeConversation when its turn
    // ends (which also cancels the timers above), or by the unmount effect below —
    // never on a conversation switch, so an in-flight turn keeps streaming in the
    // background.
  }, [
    activeConversationId,
    api,
    armWatchdog,
    armStallTimer,
    conversationKey,
    deploymentId,
    finalizeConversation,
    patchAssistantChunk,
    queryClient,
    surfaceTurnError,
    turnInFlight,
  ]);

  // Close every live stream and cancel its watchdog when the hook unmounts
  // (agent switch / chat closed).
  useEffect(() => {
    const streams = streamsRef.current;
    const timers = streamTimersRef.current;
    const stallTimers = streamStallTimersRef.current;
    return () => {
      streams.forEach((es) => es.close());
      streams.clear();
      timers.forEach((t) => window.clearTimeout(t));
      timers.clear();
      stallTimers.forEach((t) => window.clearTimeout(t));
      stallTimers.clear();
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
            appendConversationMessage(undefined, convId, {
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
              appendConversationMessage(old, convId!, {
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
