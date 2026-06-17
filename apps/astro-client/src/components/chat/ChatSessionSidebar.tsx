import { useState } from "react";
import {
  EllipsisHorizontalIcon,
  PencilSquareIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ChatSession } from "@/lib/chat/types";

export function ChatSessionSidebar({
  sessions,
  activeConversationId,
  onSelectSession,
  onRenameSession,
  onDeleteSession,
}: {
  sessions: ChatSession[];
  activeConversationId?: string | null;
  onSelectSession: (conversationId: string) => void;
  onRenameSession?: (conversationId: string, title: string) => void;
  onDeleteSession?: (conversationId: string) => void;
}) {
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftTitle, setDraftTitle] = useState("");
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(
    null,
  );

  const startRename = (session: ChatSession) => {
    setConfirmingDeleteId(null);
    setDraftTitle(session.title);
    setEditingId(session.conversationId);
  };

  const commitRename = (conversationId: string) => {
    const next = draftTitle.trim();
    const current = sessions.find((s) => s.conversationId === conversationId);
    if (next && next !== current?.title) {
      onRenameSession?.(conversationId, next);
    }
    setEditingId(null);
    setDraftTitle("");
  };

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
              const isEditing = editingId === session.conversationId;
              const isConfirmingDelete =
                confirmingDeleteId === session.conversationId;

              if (isEditing) {
                return (
                  <li key={session.conversationId}>
                    <input
                      autoFocus
                      value={draftTitle}
                      onChange={(e) => setDraftTitle(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          commitRename(session.conversationId);
                        } else if (e.key === "Escape") {
                          e.preventDefault();
                          setEditingId(null);
                          setDraftTitle("");
                        }
                      }}
                      onBlur={() => commitRename(session.conversationId)}
                      maxLength={200}
                      className="w-full rounded-lg border border-border bg-card px-3 py-2 text-sm text-foreground outline-none focus:border-ring"
                      aria-label="Conversation title"
                    />
                  </li>
                );
              }

              if (isConfirmingDelete) {
                return (
                  <li
                    key={session.conversationId}
                    className="rounded-lg bg-muted px-3 py-2"
                  >
                    <p className="text-xs text-muted-foreground">
                      Delete this conversation?
                    </p>
                    <div className="mt-2 flex justify-end gap-2">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2 text-xs"
                        onClick={() => setConfirmingDeleteId(null)}
                      >
                        Cancel
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2 text-xs text-destructive hover:bg-destructive/10 hover:text-destructive"
                        onClick={() => {
                          setConfirmingDeleteId(null);
                          onDeleteSession?.(session.conversationId);
                        }}
                      >
                        Delete
                      </Button>
                    </div>
                  </li>
                );
              }

              return (
                <li
                  key={session.conversationId}
                  className={cn(
                    "group flex items-center gap-1 rounded-lg pr-1 transition-colors",
                    isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-foreground hover:bg-muted",
                  )}
                >
                  <button
                    type="button"
                    onClick={() => onSelectSession(session.conversationId)}
                    className="min-w-0 flex-1 px-3 py-2 text-left"
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
                  {(onRenameSession || onDeleteSession) && (
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        aria-label="Conversation options"
                        className="shrink-0 rounded-md p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-card focus:opacity-100 focus-visible:outline-none group-hover:opacity-100 data-[state=open]:opacity-100"
                      >
                        <EllipsisHorizontalIcon className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-40">
                        {onRenameSession && (
                          <DropdownMenuItem onClick={() => startRename(session)}>
                            <PencilSquareIcon className="size-4" />
                            Rename
                          </DropdownMenuItem>
                        )}
                        {onDeleteSession && (
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              setConfirmingDeleteId(session.conversationId)
                            }
                          >
                            <TrashIcon className="size-4" />
                            Delete
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
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
