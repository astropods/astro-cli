import { useCallback, useMemo, useState } from "react";
import type { ChatSession } from "@/lib/chat/types";

export function titleFromFirstMessage(
  preview: string,
  existingTitle?: string,
): string {
  const trimmed = preview.trim().slice(0, 80);
  return trimmed || existingTitle || "New conversation";
}

/**
 * Ephemeral chat session list for the current browser session.
 *
 * TODO: Load session summaries from Langfuse-backed history once server
 * persistence returns.
 */
export function useChatSessions(deploymentId: string) {
  const [sessionsByDeployment, setSessionsByDeployment] = useState<
    Record<string, ChatSession[]>
  >({});

  const sessions = useMemo(
    (): ChatSession[] => sessionsByDeployment[deploymentId] ?? [],
    [deploymentId, sessionsByDeployment],
  );

  const recordSession = useCallback(
    (session: ChatSession) => {
      setSessionsByDeployment((prev) => {
        const existing = prev[deploymentId] ?? [];
        const idx = existing.findIndex(
          (s) => s.conversationId === session.conversationId,
        );
        const next =
          idx >= 0
            ? existing.map((s, i) => (i === idx ? session : s))
            : [session, ...existing];
        return { ...prev, [deploymentId]: next };
      });
    },
    [deploymentId],
  );

  const recordFirstMessage = useCallback(
    (convId: string, preview: string) => {
      const existing = sessions.find((s) => s.conversationId === convId);
      recordSession({
        conversationId: convId,
        deploymentId,
        title: titleFromFirstMessage(preview, existing?.title),
        updatedAt: new Date().toISOString(),
      });
    },
    [deploymentId, recordSession, sessions],
  );

  return { sessions, recordSession, recordFirstMessage, isLoading: false };
}
