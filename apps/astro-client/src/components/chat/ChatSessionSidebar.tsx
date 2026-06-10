import { cn } from "@/lib/utils";
import type { ChatSession } from "@/lib/chat/types";

export function ChatSessionSidebar({
  sessions,
  activeConversationId,
  onSelectSession,
}: {
  sessions: ChatSession[];
  activeConversationId?: string | null;
  onSelectSession: (conversationId: string) => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground">
        Conversations
      </div>
      <nav
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-2"
        aria-label="Conversations"
      >
        {sessions.length === 0 ? (
          <p className="px-2 py-3 text-xs text-muted-foreground">
            No conversations yet.
          </p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {sessions.map((session) => {
              const isActive =
                activeConversationId === session.conversationId;
              return (
                <li key={session.conversationId}>
                  <button
                    type="button"
                    onClick={() => onSelectSession(session.conversationId)}
                    className={cn(
                      "w-full rounded-lg px-3 py-2 text-left transition-colors",
                      isActive
                        ? "bg-accent text-accent-foreground"
                        : "text-foreground hover:bg-muted",
                    )}
                  >
                    <span className="line-clamp-2 text-sm font-medium">
                      {session.title}
                    </span>
                    <span
                      className="mt-0.5 block text-xs text-muted-foreground"
                      suppressHydrationWarning
                    >
                      {formatSessionTime(session.updatedAt)}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </nav>
    </div>
  );
}

function formatSessionTime(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const sameDay =
    date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear();
  if (sameDay) {
    return date.toLocaleTimeString(undefined, {
      hour: "numeric",
      minute: "2-digit",
    });
  }
  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });
}
