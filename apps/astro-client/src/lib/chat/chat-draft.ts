/** Per-conversation composer draft persistence.
 *
 * Drafts are keyed by deployment + conversation and stored in sessionStorage, so
 * each conversation keeps its own unsent text across switches and reloads within
 * the tab (but not indefinitely across sessions, as localStorage would). Access
 * is try/catch-wrapped for SSR and private-mode / quota failures. The new-chat
 * composer (no conversation id yet) gets its own stable slot. */

const PREFIX = "astro:chat-draft:";
const NEW_CHAT = "__new__";

function draftKey(deploymentId: string, conversationId: string | null): string {
  return `${PREFIX}${deploymentId}:${conversationId ?? NEW_CHAT}`;
}

export function loadDraft(
  deploymentId: string,
  conversationId: string | null,
): string {
  try {
    if (typeof window === "undefined") return "";
    return sessionStorage.getItem(draftKey(deploymentId, conversationId)) ?? "";
  } catch {
    return "";
  }
}

/** Persists text for a conversation, or removes the slot when empty so a sent /
 *  cleared composer doesn't resurrect a stale draft on the next visit. */
export function saveDraft(
  deploymentId: string,
  conversationId: string | null,
  text: string,
): void {
  try {
    if (typeof window === "undefined") return;
    const key = draftKey(deploymentId, conversationId);
    if (text) {
      sessionStorage.setItem(key, text);
    } else {
      sessionStorage.removeItem(key);
    }
  } catch {
    // sessionStorage unavailable (SSR, private mode) or quota exceeded — a lost
    // draft is a minor UX regression, never a hard failure.
  }
}

/** Drops a conversation's draft slot. Called when a conversation is deleted so
 *  its unsent text doesn't outlive it (and can't resurrect if the id recurs). */
export function clearDraft(
  deploymentId: string,
  conversationId: string | null,
): void {
  try {
    if (typeof window === "undefined") return;
    sessionStorage.removeItem(draftKey(deploymentId, conversationId));
  } catch {
    // sessionStorage unavailable — nothing to clean up.
  }
}
