import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  fetchDeploymentChatConversation,
  refreshDeploymentChatTail,
  useDeploymentChatConversation,
} from "@/api/queries/chat";
import { chatKeys } from "@/api/queries/keys";
import { useApiClient } from "@/lib/api-context";
import { ApiRequestError, type GetDeploymentChatConversationResponse } from "@/lib/api";
import {
  mapServerMessages,
  serverTurnInFlight,
  streamingAssistantMessageId,
} from "@/lib/chat/message";
import { openMessagingStream } from "@/lib/messaging/transport";

const IN_FLIGHT_TIMEOUT_MS = 3 * 60 * 1000;
/** Stable assistant-tail polls before treating catch-up as complete. */
const CATCHUP_STABLE_POLLS = 3;

type LivePollSignals = {
  turnDismissed: boolean;
  resuming: boolean;
  catchUp: boolean;
  pendingSend: boolean;
};

/**
 * Platform deployment chat: server-owned history, client is a live view.
 *
 * Turn state comes from GET …/chat/conversations/:id (`assistant_streaming` or
 * a trailing user message). While a turn is in flight the query polls; active
 * sends also attach SSE so the proxy persists chunks. Navigating away closes
 * SSE but the server keeps consuming upstream — returning to the conversation
 * refetches fresh server state and resumes polling (without a new SSE unless
 * the user just sent from this session).
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
  const [pendingSend, setPendingSend] = useState<{
    id: string;
    content: string;
    baseline: number;
  } | null>(null);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [turnDismissed, setTurnDismissed] = useState(false);
  const [resuming, setResuming] = useState(false);
  const [catchUp, setCatchUp] = useState(false);
  const [catchUpLive, setCatchUpLive] = useState(false);
  const attachStreamRef = useRef(false);
  const sendLockRef = useRef(false);
  const catchUpLenRef = useRef<number | null>(null);
  const catchUpStableRef = useRef(0);
  const prevResumeIdRef = useRef<string | null>(null);
  const useTailPollRef = useRef(false);
  const livePollRef = useRef<LivePollSignals>({
    turnDismissed: false,
    resuming: false,
    catchUp: false,
    pendingSend: false,
  });

  const activeConversationId = conversationIdFromOptions ?? createdConversationId;

  const shouldPoll = useCallback(
    (data: GetDeploymentChatConversationResponse | undefined) => {
      const live = livePollRef.current;
      if (live.turnDismissed) return false;
      if (live.resuming || live.catchUp || live.pendingSend) return true;
      return data != null && serverTurnInFlight(data);
    },
    [],
  );

  const { data: serverThread, isLoading: historyLoading } =
    useDeploymentChatConversation(deploymentId, activeConversationId, {
      shouldPoll,
      useTailPollRef,
    });

  const records = useMemo(
    () => serverThread?.messages ?? [],
    [serverThread?.messages],
  );

  const showOptimisticUser =
    pendingSend !== null && records.length <= pendingSend.baseline;
  const serverInFlight =
    serverThread != null && serverTurnInFlight(serverThread);
  const awaitingReply =
    !turnDismissed && (showOptimisticUser || serverInFlight);
  const isStreaming = awaitingReply || catchUpLive;

  useTailPollRef.current =
    !turnDismissed &&
    !resuming &&
    !historyLoading &&
    (serverInFlight || catchUp || showOptimisticUser);

  livePollRef.current = {
    turnDismissed,
    resuming,
    catchUp,
    pendingSend: showOptimisticUser,
  };

  const assistantStreaming = showOptimisticUser
    ? null
    : streamingAssistantMessageId(records, isStreaming);

  const messages = useMemo(() => {
    const base = mapServerMessages(records, assistantStreaming);
    if (showOptimisticUser && pendingSend) {
      base.push({
        id: pendingSend.id,
        role: "user",
        content: pendingSend.content,
      });
    }
    return base;
  }, [assistantStreaming, pendingSend, records, showOptimisticUser]);

  const resetCatchUp = useCallback(() => {
    setCatchUp(false);
    setCatchUpLive(false);
    catchUpLenRef.current = null;
    catchUpStableRef.current = 0;
  }, []);

  const maybeStartCatchUp = useCallback(
    (thread: GetDeploymentChatConversationResponse) => {
      if (serverTurnInFlight(thread)) {
        resetCatchUp();
        return;
      }
      const tail = thread.messages.at(-1);
      if (tail?.role !== "assistant") {
        resetCatchUp();
        return;
      }
      setCatchUp(true);
      catchUpLenRef.current = tail.content.length;
      catchUpStableRef.current = 0;
    },
    [resetCatchUp],
  );

  // Fresh fetch whenever we land on a conversation (including navigate-back).
  useEffect(() => {
    if (!activeConversationId) {
      prevResumeIdRef.current = null;
      setResuming(false);
      resetCatchUp();
      return;
    }

    const prev = prevResumeIdRef.current;
    prevResumeIdRef.current = activeConversationId;

    setResuming(true);
    resetCatchUp();
    setTurnDismissed(false);
    // Lazy-create goes null → new id in one send; keep the SSE attachment.
    if (prev !== null && prev !== activeConversationId) {
      attachStreamRef.current = false;
    }

    void queryClient
      .fetchQuery({
        queryKey: chatKeys.conversation(deploymentId, activeConversationId),
        queryFn: () =>
          fetchDeploymentChatConversation(api, deploymentId, activeConversationId),
      })
      .then(maybeStartCatchUp)
      .finally(() => setResuming(false));
  }, [
    activeConversationId,
    api,
    deploymentId,
    maybeStartCatchUp,
    queryClient,
    resetCatchUp,
  ]);

  // During catch-up, keep polling only while assistant content is still growing.
  useEffect(() => {
    if (!catchUp || !serverThread) return;
    if (serverTurnInFlight(serverThread)) {
      resetCatchUp();
      return;
    }
    const tail = serverThread.messages.at(-1);
    if (tail?.role !== "assistant") {
      resetCatchUp();
      return;
    }
    const len = tail.content.length;
    if (catchUpLenRef.current === null) {
      catchUpLenRef.current = len;
      return;
    }
    if (len > catchUpLenRef.current) {
      catchUpLenRef.current = len;
      catchUpStableRef.current = 0;
      setCatchUpLive(true);
      return;
    }
    catchUpStableRef.current += 1;
    if (catchUpStableRef.current >= CATCHUP_STABLE_POLLS) {
      resetCatchUp();
    }
  }, [catchUp, resetCatchUp, serverThread]);

  useEffect(() => {
    if (pendingSend && records.length > pendingSend.baseline) {
      setPendingSend(null);
    }
  }, [pendingSend, records.length]);

  useEffect(() => {
    if (!serverInFlight) setTurnDismissed(false);
  }, [serverInFlight]);

  const prevConversationIdRef = useRef<string | null>(activeConversationId);
  useEffect(() => {
    const prev = prevConversationIdRef.current;
    prevConversationIdRef.current = activeConversationId;
    if (prev !== null && prev !== activeConversationId) {
      setPendingSend(null);
      setStreamError(null);
    }
  }, [activeConversationId]);

  useEffect(() => {
    setCreatedConversationId(null);
  }, [conversationIdFromOptions, deploymentId]);

  // Active sends attach SSE; recovery after navigation is polling-only.
  useEffect(() => {
    if (!awaitingReply || !activeConversationId || !attachStreamRef.current) {
      return;
    }
    const es = openMessagingStream(api, deploymentId, activeConversationId, {
      onChunk: () => {
        void refreshDeploymentChatTail(
          queryClient,
          api,
          deploymentId,
          activeConversationId,
        );
      },
      onFinish: () => {
        attachStreamRef.current = false;
        void queryClient.invalidateQueries({
          queryKey: chatKeys.conversation(deploymentId, activeConversationId),
        });
        void queryClient.invalidateQueries({
          queryKey: chatKeys.conversations(deploymentId),
        });
      },
      onProtocolError: () => {
        attachStreamRef.current = false;
        void refreshDeploymentChatTail(
          queryClient,
          api,
          deploymentId,
          activeConversationId,
        );
      },
    });
    return () => es.close();
  }, [activeConversationId, api, deploymentId, queryClient, awaitingReply]);

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState !== "visible" || !activeConversationId) {
        return;
      }
      if (useTailPollRef.current) {
        void refreshDeploymentChatTail(
          queryClient,
          api,
          deploymentId,
          activeConversationId,
        );
        return;
      }
      void queryClient.invalidateQueries({
        queryKey: chatKeys.conversation(deploymentId, activeConversationId),
      });
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [activeConversationId, api, deploymentId, queryClient]);

  useEffect(() => {
    if (!isStreaming) return;
    const timeout = window.setTimeout(() => {
      setStreamError("Response timed out. You can try sending again.");
      setTurnDismissed(true);
      attachStreamRef.current = false;
    }, IN_FLIGHT_TIMEOUT_MS);
    return () => window.clearTimeout(timeout);
  }, [isStreaming]);

  const sendMessage = useCallback(
    async (content: string) => {
      const trimmed = content.trim();
      if (!trimmed || sendLockRef.current || awaitingReply) return;
      sendLockRef.current = true;

      setStreamError(null);
      setTurnDismissed(false);
      resetCatchUp();
      attachStreamRef.current = true;
      setPendingSend({
        id: `optimistic-${Date.now()}`,
        content: trimmed,
        baseline: records.length,
      });

      try {
        let convId = activeConversationId;
        if (!convId) {
          const created = await api.createMessagingConversation(deploymentId);
          convId = created.conversation_id;
          setCreatedConversationId(convId);
        }

        await api.sendMessagingMessage(deploymentId, convId, trimmed);
        await queryClient.fetchQuery({
          queryKey: chatKeys.conversation(deploymentId, convId),
          queryFn: () =>
            fetchDeploymentChatConversation(api, deploymentId, convId),
        });
        onConversationCreated?.(convId, trimmed);
      } catch (err) {
        attachStreamRef.current = false;
        setStreamError(
          err instanceof ApiRequestError
            ? err.message
            : "Failed to send message. Please try again.",
        );
        setPendingSend(null);
      } finally {
        sendLockRef.current = false;
      }
    },
    [
      activeConversationId,
      api,
      deploymentId,
      onConversationCreated,
      queryClient,
      records.length,
      resetCatchUp,
      awaitingReply,
    ],
  );

  const cancelStream = useCallback(() => {
    setTurnDismissed(true);
    attachStreamRef.current = false;
  }, []);

  return {
    messages,
    conversationId: activeConversationId,
    isStreaming,
    assistantStreaming,
    streamError,
    historyLoading,
    sendMessage,
    cancelStream,
  };
}
