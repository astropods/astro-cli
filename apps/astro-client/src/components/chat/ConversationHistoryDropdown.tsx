import { useState } from "react";
import {
  ChevronDownIcon,
  ClockIcon,
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

const DEFAULT_TITLE = "New conversation";

export function ConversationHistoryDropdown({
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
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="h-8 shrink-0 gap-1.5 px-2.5 text-body-sm font-medium"
          aria-label="Chat history"
        >
          <ClockIcon className="size-4 shrink-0 text-muted-foreground" />
          <span className="text-foreground">History</span>
          <span className="font-mono text-mono-sm text-faint-foreground">
            {sessions.length}
          </span>
          <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[min(100vw-2rem,20rem)] p-0">
        <ConversationHistoryList
          sessions={sessions}
          activeConversationId={activeConversationId}
          onSelectSession={onSelectSession}
          onRenameSession={onRenameSession}
          onDeleteSession={onDeleteSession}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ConversationHistoryList({
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
    <div className="flex max-h-[min(60vh,24rem)] flex-col">
      <div className="flex shrink-0 items-baseline justify-between border-b border-border px-3.5 py-2.5">
        <span className="font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
          History
        </span>
        <span className="font-mono text-mono-sm text-faint-foreground">
          {sessions.length}
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-1.5">
        {sessions.length === 0 ? (
          <p className="px-2 py-3 text-body-sm text-faint-foreground">
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
                      className="w-full rounded-lg border border-border bg-card px-3 py-2 text-body-sm text-foreground outline-none focus:border-ring"
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
                    <p className="text-body-sm text-faint-foreground">
                      Delete this conversation?
                    </p>
                    <div className="mt-2 flex justify-end gap-2">
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        className="text-body-sm"
                        onClick={() => setConfirmingDeleteId(null)}
                      >
                        Cancel
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        className="text-body-sm text-destructive hover:bg-destructive/10 hover:text-destructive"
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
                  <DropdownMenuItem
                    onSelect={() => onSelectSession(session.conversationId)}
                    className="min-w-0 flex-1 cursor-pointer flex-col items-start gap-0 rounded-lg px-2.5 py-2 text-inherit focus:bg-transparent focus:text-inherit data-[highlighted]:bg-transparent"
                  >
                    <span className="line-clamp-2 text-body-sm font-medium">
                      {session.title || DEFAULT_TITLE}
                    </span>
                    <span
                      className="mt-0.5 block text-body-sm text-faint-foreground"
                      suppressHydrationWarning
                    >
                      {formatSessionTime(session.updatedAt)}
                    </span>
                  </DropdownMenuItem>
                  {session.assistantStreaming ? (
                    <span
                      className="mr-1 size-2 shrink-0 rounded-full bg-primary"
                      aria-label="Reply in progress"
                      title="Reply in progress"
                    />
                  ) : null}
                  {(onRenameSession || onDeleteSession) && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label="Conversation options"
                          className="size-7 shrink-0 text-muted-foreground opacity-0 transition-opacity hover:bg-card focus:opacity-100 group-hover:opacity-100 data-[state=open]:opacity-100"
                          onPointerDown={(e) => e.stopPropagation()}
                          onClick={(e) => e.stopPropagation()}
                        >
                          <EllipsisHorizontalIcon className="size-4" />
                        </Button>
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
      </div>
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
