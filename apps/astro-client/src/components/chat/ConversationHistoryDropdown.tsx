import { useCallback, useState } from "react";
import { Clock, Pencil, Trash2 } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ChatPanelSectionHeader } from "@/components/chat/ChatPanelSectionHeader";
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
    <TooltipProvider>
      {/* Non-modal: a modal menu locks document.body pointer-events while open,
          and a re-render during close (frequent while a reply streams) can leave
          that lock stuck, so the next click — e.g. selecting a conversation to
          switch to — lands on a dead body and does nothing. A history menu needs
          no modal behavior. */}
      <DropdownMenu modal={false}>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 shrink-0 gap-1.5 px-2.5 text-body-sm font-medium"
                aria-label="Chat history"
              >
                <Clock className="size-4 shrink-0 text-foreground" />
                <span className="font-mono text-mono-sm text-foreground">
                  {sessions.length}
                </span>
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent side="bottom">Chat history</TooltipContent>
        </Tooltip>
        <DropdownMenuContent
          align="end"
          collisionPadding={16}
          className="w-[min(100vw-2rem,20rem)] p-0"
        >
          <ConversationHistoryList
            sessions={sessions}
            activeConversationId={activeConversationId}
            onSelectSession={onSelectSession}
            onRenameSession={onRenameSession}
            onDeleteSession={onDeleteSession}
          />
        </DropdownMenuContent>
      </DropdownMenu>
    </TooltipProvider>
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

  const cancelRename = () => {
    setEditingId(null);
    setDraftTitle("");
  };

  // Stable callback ref: focuses/selects the input the moment it mounts. Stable
  // identity means React won't re-run it (resetting the cursor) on each keystroke.
  const focusRenameInput = useCallback((el: HTMLInputElement | null) => {
    if (el) {
      el.focus();
      el.select();
    }
  }, []);

  return (
    <div className="flex max-h-[min(60vh,24rem)] flex-col">
      <div className="flex shrink-0 items-center border-b border-border px-3.5 py-2.5">
        <ChatPanelSectionHeader
          label="Chat history"
          icon={Clock}
          count={sessions.length}
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-1.5">
        {sessions.length === 0 ? (
          <p className="px-2 py-3 text-body-sm text-faint-foreground">
            No conversations yet.
          </p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {sessions.map((session) => {
              const isActive = activeConversationId === session.conversationId;
              const isEditing = editingId === session.conversationId;
              const isConfirmingDelete =
                confirmingDeleteId === session.conversationId;

              if (isEditing) {
                return (
                  <li key={session.conversationId}>
                    <input
                      ref={focusRenameInput}
                      value={draftTitle}
                      onChange={(e) => setDraftTitle(e.target.value)}
                      // Keep keystrokes from bubbling to the Radix menu, whose
                      // typeahead would otherwise steal focus to a matching row.
                      onKeyDown={(e) => {
                        e.stopPropagation();
                        if (e.key === "Enter") {
                          e.preventDefault();
                          commitRename(session.conversationId);
                        } else if (e.key === "Escape") {
                          e.preventDefault();
                          cancelRename();
                        }
                      }}
                      onBlur={() => commitRename(session.conversationId)}
                      maxLength={200}
                      placeholder={DEFAULT_TITLE}
                      className="w-full rounded-sm border border-border bg-card px-3 py-2 text-body-sm text-foreground outline-none focus:border-ring"
                      aria-label="Conversation title"
                    />
                  </li>
                );
              }

              if (isConfirmingDelete) {
                return (
                  <li
                    key={session.conversationId}
                    className="rounded-sm bg-accent/50 px-3 py-2"
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
                    "group flex items-center gap-1 rounded-sm pr-1 transition-colors",
                    isActive
                      ? "bg-accent/60 text-accent-foreground"
                      : "text-foreground hover:bg-accent/40",
                  )}
                >
                  <DropdownMenuItem
                    onSelect={() => onSelectSession(session.conversationId)}
                    className="min-w-0 flex-1 cursor-pointer flex-col items-start gap-0 rounded-sm px-2.5 py-2 text-inherit focus:bg-transparent focus:text-inherit data-[highlighted]:bg-transparent"
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
                    <div className="flex shrink-0 items-center gap-0.5 opacity-100 transition-opacity focus-within:opacity-100 [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100">
                      {onRenameSession && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              aria-label="Rename conversation"
                              className="size-7 shrink-0 text-muted-foreground hover:bg-transparent hover:text-foreground dark:hover:bg-transparent"
                              // Inline action instead of a nested menu: no menu
                              // teardown means nothing steals focus from the input.
                              onPointerDown={(e) => e.stopPropagation()}
                              onClick={(e) => {
                                e.stopPropagation();
                                startRename(session);
                              }}
                            >
                              <Pencil className="size-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent side="top">Rename</TooltipContent>
                        </Tooltip>
                      )}
                      {onDeleteSession && (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              aria-label="Delete conversation"
                              className="size-7 shrink-0 text-muted-foreground hover:bg-transparent hover:text-destructive dark:hover:bg-transparent"
                              onPointerDown={(e) => e.stopPropagation()}
                              onClick={(e) => {
                                e.stopPropagation();
                                setConfirmingDeleteId(session.conversationId);
                              }}
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent side="top">Delete</TooltipContent>
                        </Tooltip>
                      )}
                    </div>
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
