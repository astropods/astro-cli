import {
  Reasoning,
  ReasoningContent,
  ReasoningRoot,
  ReasoningText,
  ReasoningTrigger,
} from "@/components/assistant-ui/reasoning";
import {
  ToolGroupContent,
  ToolGroupRoot,
  ToolGroupTrigger,
} from "@/components/assistant-ui/tool-group";
import { ToolFallback } from "@/components/assistant-ui/tool-fallback";
import { ChatButton, chatButtonVariants } from "@/components/assistant-ui/chat-button";
import { TooltipIconButton } from "@/components/assistant-ui/tooltip-icon-button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { DeploymentChatHistoryScroll } from "@/components/chat/DeploymentChatHistoryScroll";
import { DeploymentChatStreamingIndicator } from "@/components/chat/DeploymentChatStreamingIndicator";
import { DeploymentChatText } from "@/components/chat/DeploymentChatText";
import { useDeploymentChatViewport } from "@/components/chat/deployment-chat-streaming-context";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { Button } from "@/components/ui/button";
import { deploymentPath } from "@/lib/routes";
import {
  isDictationActive,
  useDictationSupported,
} from "@/lib/chat/dictation";
import { loadDraft, saveDraft } from "@/lib/chat/chat-draft";
import { DictationWaveform } from "@/components/chat/DictationWaveform";
import { useDownloadDeploymentFile } from "@/api/queries/files";
import {
  readAttachmentRef,
  ASTRO_FILE_PART,
} from "@/lib/messaging/deployment-attachment-adapter";
import { formatBytes } from "@/lib/format-utils";
import type { ChatComposerState } from "@/lib/deployment-utils";
import type { AgentDeploymentSummary, ChatAttachment } from "@/lib/api";
import {
  ActionBarPrimitive,
  AttachmentPrimitive,
  AuiIf,
  ComposerPrimitive,
  ErrorPrimitive,
  groupPartByType,
  MessagePrimitive,
  ThreadPrimitive,
  useAuiState,
  useComposer,
  useComposerRuntime,
  type CompleteAttachment,
} from "@assistant-ui/react";
import {
  AlertTriangle,
  ArrowDownIcon,
  ArrowUpIcon,
  CheckIcon,
  CopyIcon,
  Download,
  ExternalLink,
  FileIcon,
  Loader2,
  Mic,
  Paperclip,
  Pause,
  Power,
  SquareIcon,
  Upload,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type FC,
} from "react";

export function DeploymentChatThreadView({
  account,
  deploymentId,
  deployment,
  agentLabel,
  composerState,
}: {
  account: string;
  deploymentId: string;
  deployment?: AgentDeploymentSummary;
  agentLabel: string;
  composerState: ChatComposerState;
}) {
  const { conversationId, isStreaming, historyLoading, streamError } =
    useDeploymentChatViewport();
  const useTopTurnAnchor = isStreaming && !historyLoading;
  const isEmpty = useAuiState((s) => s.thread.isEmpty);
  // States where the agent is off / broken: dim the thread to signal it.
  const dimThread =
    composerState === "paused" ||
    composerState === "stopped" ||
    composerState === "error";

  return (
    <ThreadPrimitive.Root
      className="aui-root aui-thread-root @container flex h-full min-h-0 flex-1 flex-col"
      style={{
        ["--thread-max-width" as string]: "44rem",
        ["--composer-radius" as string]: "20px",
        ["--composer-padding" as string]: "10px",
      }}
    >
      <ConversationDraftPersistence
        deploymentId={deploymentId}
        conversationId={conversationId}
      />
      <ThreadPrimitive.Viewport
        key={conversationId ?? "draft"}
        turnAnchor={useTopTurnAnchor ? "top" : "bottom"}
        autoScroll={useTopTurnAnchor}
        scrollToBottomOnInitialize
        className="chat-thread-scroll relative flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto"
      >
        <DeploymentChatHistoryScroll />
        <div
          className={cn(
            "mx-auto flex w-full max-w-(--thread-max-width) min-h-0 flex-1 flex-col px-3 pt-3 md:px-6 md:pt-4",
            isEmpty && "justify-center",
          )}
        >
          <AuiIf condition={(s) => s.thread.isEmpty}>
            <ThreadWelcome
              deployment={deployment}
              agentLabel={agentLabel}
            />
          </AuiIf>

          <div
            className={cn(
              "mb-10 flex flex-col gap-y-6 empty:hidden md:mb-12 md:gap-y-8",
              dimThread && "pointer-events-none opacity-60 saturate-50",
            )}
          >
            <ThreadPrimitive.Messages>
              {() => (
                <ThreadMessage deploymentId={deploymentId} agentLabel={agentLabel} />
              )}
            </ThreadPrimitive.Messages>
          </div>

          <ThreadPrimitive.ViewportFooter
            className={cn(
              "aui-thread-viewport-footer bg-background flex flex-col gap-2 overflow-visible rounded-t-(--composer-radius)",
              isEmpty ? "mt-8 pb-3 md:pb-4" : "sticky bottom-0 mt-auto pb-4 md:pb-5",
            )}
          >
            <ThreadScrollToBottom />
            {streamError ? (
              <p className="px-1 text-xs text-destructive" role="alert">
                {streamError}
              </p>
            ) : null}
            <ComposerArea
              state={composerState}
              agentLabel={agentLabel}
              account={account}
              deploymentId={deploymentId}
              expanded={isEmpty}
            />
          </ThreadPrimitive.ViewportFooter>
        </div>
      </ThreadPrimitive.Viewport>
    </ThreadPrimitive.Root>
  );
}

// Persists the composer draft per conversation. The composer store is shared
// across conversations (the runtime is keyed on the agent), so drafts are keyed
// by conversation in sessionStorage and swapped as the user navigates, surviving
// switches and reloads within the tab. Isolated as its own null-rendering node
// so the per-keystroke composer subscription re-renders only this, not the whole
// thread. See @/lib/chat/chat-draft.
const ConversationDraftPersistence: FC<{
  deploymentId: string;
  conversationId: string | null;
}> = ({ deploymentId, conversationId }) => {
  const composerRuntime = useComposerRuntime();
  const liveText = useComposer((c) => c.text);
  // Tracks which conversation the composer currently holds. Starts undefined so
  // the first run restores (covers mount / reload), then only re-restores when
  // the conversation actually changes.
  const restoredForRef = useRef<string | null | undefined>(undefined);

  // Restore the incoming conversation's saved draft.
  useEffect(() => {
    if (restoredForRef.current === conversationId) return;
    restoredForRef.current = conversationId;
    composerRuntime.setText(loadDraft(deploymentId, conversationId));
  }, [conversationId, deploymentId, composerRuntime]);

  // Mirror the live composer into storage for the conversation it belongs to.
  // Reads the store (not the render-captured liveText) so the mid-switch render
  // — where liveText is still the previous conversation's — can't write under
  // the new key; the restore effect above runs first and re-points the store.
  useEffect(() => {
    if (restoredForRef.current !== conversationId) return;
    saveDraft(deploymentId, conversationId, composerRuntime.getState().text);
  }, [liveText, conversationId, deploymentId, composerRuntime]);

  return null;
};

const ThreadMessage: FC<{
  deploymentId: string;
  agentLabel: string;
}> = ({ deploymentId, agentLabel }) => {
  const role = useAuiState((s) => s.message.role);
  if (role === "user") return <UserMessage deploymentId={deploymentId} />;
  return (
    <AssistantMessage deploymentId={deploymentId} agentLabel={agentLabel} />
  );
};

const ThreadScrollToBottom: FC = () => (
  <ThreadPrimitive.ScrollToBottom asChild>
    <TooltipIconButton
      tooltip="Scroll to bottom"
      variant="outline"
      className="aui-thread-scroll-to-bottom dark:border-border dark:bg-background dark:hover:bg-accent absolute -top-11 z-10 self-center rounded-full p-3 disabled:invisible"
    >
      <ArrowDownIcon />
    </TooltipIconButton>
  </ThreadPrimitive.ScrollToBottom>
);

const ThreadWelcome: FC<{
  deployment?: AgentDeploymentSummary;
  agentLabel: string;
}> = ({ deployment, agentLabel }) => (
  <div className="aui-thread-welcome-root fade-in slide-in-from-bottom-2 animate-in flex flex-col duration-500">
    <div className="aui-thread-welcome-center flex w-full flex-col items-center justify-center pb-2">
      <div className="aui-thread-welcome-message flex flex-col items-center gap-7 px-4 text-center">
        {deployment ? (
          <DeploymentAvatar
            deployment={deployment}
            size={64}
            className="size-16 rounded-sm"
          />
        ) : null}
        <div className="flex flex-col items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground md:text-3xl">
            What should {agentLabel} work on?
          </h1>
          <p className="text-muted-foreground text-body">
            Describe a task in your own words to get started.
          </p>
        </div>
      </div>
    </div>
  </div>
);

const TRANSIENT_NOTICE: Record<"starting" | "unreachable", string> = {
  starting: "Agent is starting…",
  unreachable: "The agent's messaging endpoint isn't reachable right now.",
};

const STATE_BANNER: Record<
  "paused" | "error" | "stopped",
  { Icon: typeof Pause; titleSuffix: string; body: string }
> = {
  paused: {
    Icon: Pause,
    titleSuffix: "is paused",
    body: "You can't send messages until it's resumed.",
  },
  stopped: {
    Icon: Power,
    titleSuffix: "isn't running",
    body: "Start the agent to chat.",
  },
  error: {
    Icon: AlertTriangle,
    titleSuffix: "is in an error state",
    body: "Check the agent page for details.",
  },
};

const ComposerArea: FC<{
  state: ChatComposerState;
  agentLabel: string;
  account: string;
  deploymentId: string;
  expanded?: boolean;
}> = ({ state, agentLabel, account, deploymentId, expanded }) => {
  // "unknown" = status not loaded yet; stay optimistic so the composer doesn't
  // flicker disabled on first paint for a healthy agent.
  if (state === "ready" || state === "unknown") {
    return (
      <DeploymentComposer agentLabel={agentLabel} expanded={expanded} />
    );
  }
  if (state === "starting" || state === "unreachable") {
    return (
      <>
        <p className="flex items-center gap-1.5 px-1 text-xs text-muted-foreground">
          {state === "starting" ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : null}
          {TRANSIENT_NOTICE[state]}
        </p>
        <DeploymentComposer disabled agentLabel={agentLabel} expanded={expanded} />
      </>
    );
  }
  return (
    <ChatStateBanner
      state={state}
      agentLabel={agentLabel}
      account={account}
      deploymentId={deploymentId}
    />
  );
};

const ChatStateBanner: FC<{
  state: "paused" | "error" | "stopped";
  agentLabel: string;
  account: string;
  deploymentId: string;
}> = ({ state, agentLabel, account, deploymentId }) => {
  const { Icon, titleSuffix, body } = STATE_BANNER[state];
  return (
    <div className="flex items-center gap-3 rounded-(--composer-radius) border border-border bg-surface/70 p-3 pl-4">
      <span
        className={cn(
          "flex size-8 shrink-0 items-center justify-center rounded-lg border border-border",
          state === "error" ? "text-destructive" : "text-muted-foreground",
        )}
      >
        <Icon className="size-4" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-foreground">
          {agentLabel} {titleSuffix}
        </p>
        <p className="text-xs text-muted-foreground">{body}</p>
      </div>
      <Button asChild variant="outline" size="sm" className="shrink-0">
        <Link to={deploymentPath(account, deploymentId)}>
          <ExternalLink className="size-3.5" />
          Open agent
        </Link>
      </Button>
    </div>
  );
};

const DeploymentComposer: FC<{
  disabled?: boolean;
  agentLabel: string;
  expanded?: boolean;
}> = ({ disabled, agentLabel, expanded }) => {
  const composer = useComposerRuntime();
  const dictation = useComposer((c) => c.dictation);
  const listening = isDictationActive(dictation);
  const shellRef = useRef<HTMLDivElement>(null);
  // Only offer file attachments (paperclip + drag-drop) when the agent supports
  // them; otherwise uploads would be silently ignored. Mirrors the adapter gate
  // in DeploymentChatRuntimeProvider so the UI and the runtime stay in lockstep.
  const { filesEnabled } = useDeploymentChatViewport();

  // Auto-focus on launch and when the agent becomes ready (#1355); skipped while
  // disabled or dictating. Esc/Enter are handled by the assistant-ui composer.
  useEffect(() => {
    if (disabled || listening) return;
    shellRef.current?.querySelector("textarea")?.focus();
  }, [disabled, listening]);

  // Files (paperclip and drag-drop) go through the attachment adapter: they show
  // as removable chips in the composer and upload on send, then ride the message
  // as the agent's context for that turn.
  const addFiles = useCallback(
    (files: FileList | null) => {
      if (!files) return;
      for (const file of Array.from(files)) void composer.addAttachment(file);
    },
    [composer],
  );

  // Drag-and-drop highlight. A depth counter tracks nested dragenter/dragleave
  // (children fire their own events) so the highlight doesn't flicker as the
  // pointer moves over the input/buttons.
  const [isDraggingFiles, setIsDraggingFiles] = useState(false);
  const dragDepth = useRef(0);
  const draggingFiles = (e: DragEvent) =>
    e.dataTransfer.types.includes("Files");
  const onDragEnter = useCallback(
    (e: DragEvent) => {
      if (disabled || !filesEnabled || !draggingFiles(e)) return;
      e.preventDefault();
      dragDepth.current += 1;
      setIsDraggingFiles(true);
    },
    [disabled, filesEnabled],
  );
  const onDragOver = useCallback(
    (e: DragEvent) => {
      if (disabled || !filesEnabled || !draggingFiles(e)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = "copy";
    },
    [disabled, filesEnabled],
  );
  const onDragLeave = useCallback(
    (e: DragEvent) => {
      if (disabled || !filesEnabled || !draggingFiles(e)) return;
      dragDepth.current -= 1;
      if (dragDepth.current <= 0) {
        dragDepth.current = 0;
        setIsDraggingFiles(false);
      }
    },
    [disabled, filesEnabled],
  );
  const onDrop = useCallback(
    (e: DragEvent) => {
      if (disabled || !filesEnabled || !draggingFiles(e)) return;
      e.preventDefault();
      dragDepth.current = 0;
      setIsDraggingFiles(false);
      addFiles(e.dataTransfer.files);
    },
    [disabled, filesEnabled, addFiles],
  );

  // Text typed before dictation started, captured at the moment we start so
  // Cancel can restore it (the runtime overwrites the composer text live as it
  // transcribes). Recorded in startDictation, not an effect, to avoid racing
  // the first interim result.
  const preDictationText = useRef("");

  const startDictation = useCallback(() => {
    preDictationText.current = composer.getState().text;
    composer.startDictation();
  }, [composer]);

  // Confirm keeps the transcript in the input for review/send; Cancel discards
  // it and restores whatever was there before.
  const confirmDictation = useCallback(() => {
    composer.stopDictation();
  }, [composer]);
  const cancelDictation = useCallback(() => {
    composer.stopDictation();
    composer.setText(preDictationText.current);
  }, [composer]);

  // Safety nets that used to live on the mic button: stop a live session when
  // the composer goes disabled (agent paused/stopped) or unmounts (panel close,
  // agent switch), otherwise the mic keeps streaming with no way to stop it.
  useEffect(() => {
    if (disabled && listening) composer.stopDictation();
  }, [disabled, listening, composer]);
  useEffect(() => () => composer.stopDictation(), [composer]);

  return (
    <ComposerPrimitive.Root
      className="aui-composer-root relative flex w-full flex-col"
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <div
        ref={shellRef}
        data-slot="aui_composer-shell"
        className={cn(
          "bg-surface/70 flex w-full flex-col gap-2 rounded-(--composer-radius) border border-input p-(--composer-padding) transition-[border-color,box-shadow]",
          "focus-within:border-primary/70 focus-within:ring-2 focus-within:ring-primary/15",
          isDraggingFiles && "border-primary/70 ring-2 ring-primary/15",
          disabled && "pointer-events-none opacity-60",
        )}
      >
        {listening ? (
          <DictationWaveform
            onCancel={cancelDictation}
            onConfirm={confirmDictation}
          />
        ) : (
          <>
            <ComposerAttachments />
            <div className="flex w-full items-end gap-2">
              <ComposerPrimitive.Input
                placeholder={
                  disabled
                    ? "Agent is not ready…"
                    : `Message ${agentLabel}…`
                }
                disabled={disabled}
                className={cn(
                  "aui-composer-input placeholder:text-muted-foreground/80 min-w-0 flex-1 resize-none bg-transparent px-1.75 py-1.5 text-sm outline-none transition-[min-height] duration-300 ease-out disabled:cursor-not-allowed",
                  expanded
                    ? "max-h-48 min-h-22 self-stretch"
                    : "max-h-32 min-h-8 self-center",
                )}
                rows={1}
                aria-label="Message input"
              />
              <ComposerAction
                disabled={disabled}
                filesEnabled={filesEnabled}
                onStartDictation={startDictation}
              />
            </div>
          </>
        )}
      </div>
      {isDraggingFiles ? (
        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-(--composer-radius) border-2 border-dashed border-primary/60 bg-surface/90">
          <span className="flex items-center gap-2 text-body-sm font-medium text-foreground">
            <Upload className="size-4" />
            Drop files to share with {agentLabel}
          </span>
        </div>
      ) : null}
    </ComposerPrimitive.Root>
  );
};

// Removable chips for files staged in the composer (before send). The adapter's
// send() uploads them; here we only show name + remove.
const ComposerAttachments: FC = () => (
  // Register under every type key: assistant-ui picks the component by the
  // attachment's `type` (image/document/file) and only falls back to
  // `Attachment` when none matches, so a "file" chip needs the File slot.
  <ComposerPrimitive.Attachments
    components={{
      Image: ComposerAttachmentChip,
      Document: ComposerAttachmentChip,
      File: ComposerAttachmentChip,
      Attachment: ComposerAttachmentChip,
    }}
  />
);

const ComposerAttachmentChip: FC = () => (
  <div className="flex items-center gap-1.5 rounded-lg border border-border bg-surface/60 px-2 py-1">
    <FileIcon className="size-3.5 shrink-0 text-muted-foreground" />
    <span className="max-w-40 truncate text-label text-foreground">
      <AttachmentPrimitive.Name />
    </span>
    <AttachmentPrimitive.Remove asChild>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Remove attachment"
        className="ml-0.5"
      >
        <X className="size-3" />
      </Button>
    </AttachmentPrimitive.Remove>
  </div>
);

// Mic button: starts browser-native dictation, which replaces the input with a
// live audio-reactive waveform until the user confirms or cancels. No
// client-side model/VAD; the browser handles capture + recognition. The
// session's stop/cancel controls live on the waveform; lifecycle safety (stop
// on disable/unmount) lives on DeploymentComposer, which stays mounted while
// the mic button is swapped out for the waveform.
const DictationButton: FC<{ disabled?: boolean; onStart: () => void }> = ({
  disabled,
  onStart,
}) => {
  return (
    <TooltipIconButton
      // Privacy disclosure: the Web Speech API may send captured audio to the
      // browser vendor's speech service (e.g. Chrome → Google) for transcription.
      tooltip="Click to dictate"
      side="bottom"
      type="button"
      variant="ghost"
      size="icon"
      disabled={disabled}
      className="aui-composer-mic size-8 rounded-full"
      aria-label="Dictate"
      onClick={onStart}
    >
      <Mic className="size-4" />
    </TooltipIconButton>
  );
};

// Attach button: opens the native file picker and routes selected files through
// the composer's attachment adapter (chips + upload-on-send). Styled to match
// the composer's icon buttons. `multiple` allows selecting several at once.
const ComposerAttachButton: FC<{ disabled?: boolean }> = ({ disabled }) => (
  <TooltipProvider delayDuration={0}>
    <Tooltip>
      <TooltipTrigger asChild>
        <ComposerPrimitive.AddAttachment
          multiple
          disabled={disabled}
          aria-label="Attach files"
          className={cn(
            chatButtonVariants({ variant: "ghost", size: "icon" }),
            "aui-composer-attach size-8 rounded-full",
          )}
        >
          <Paperclip className="size-4" />
        </ComposerPrimitive.AddAttachment>
      </TooltipTrigger>
      <TooltipContent side="bottom">Attach files</TooltipContent>
    </Tooltip>
  </TooltipProvider>
);

const ComposerAction: FC<{
  disabled?: boolean;
  filesEnabled?: boolean;
  onStartDictation: () => void;
}> = ({ disabled, filesEnabled, onStartDictation }) => {
  const dictationSupported = useDictationSupported();
  return (
    <div className="aui-composer-action-wrapper relative flex items-center justify-end gap-2">
      {filesEnabled ? <ComposerAttachButton disabled={disabled} /> : null}
      {dictationSupported ? (
        <DictationButton disabled={disabled} onStart={onStartDictation} />
      ) : null}
      <AuiIf condition={(s) => !s.thread.isRunning}>
        <ComposerPrimitive.Send asChild>
          <ChatButton
            type="button"
            variant="default"
            size="icon"
            disabled={disabled}
            className="aui-composer-send size-8 rounded-full"
            aria-label="Send message"
          >
            <ArrowUpIcon className="aui-composer-send-icon size-4" />
          </ChatButton>
        </ComposerPrimitive.Send>
      </AuiIf>
      <AuiIf condition={(s) => s.thread.isRunning}>
        <ComposerPrimitive.Cancel asChild>
          <ChatButton
            type="button"
            variant="default"
            size="icon"
            className="aui-composer-cancel size-8 rounded-full"
            aria-label="Stop generating"
          >
            <SquareIcon className="aui-composer-cancel-icon size-3 fill-current" />
          </ChatButton>
        </ComposerPrimitive.Cancel>
      </AuiIf>
    </div>
  );
};

const MessageError: FC = () => (
  <MessagePrimitive.Error>
    <ErrorPrimitive.Root className="aui-message-error-root border-destructive bg-destructive/10 text-destructive dark:bg-destructive/5 mt-2 rounded-md border p-3 text-sm dark:text-red-200">
      <ErrorPrimitive.Message className="aui-message-error-message line-clamp-4" />
    </ErrorPrimitive.Root>
  </MessagePrimitive.Error>
);

const AssistantMessage: FC<{
  deploymentId: string;
  agentLabel: string;
}> = ({ deploymentId, agentLabel }) => {
  const ACTION_BAR_PT = "pt-1.5";
  const ACTION_BAR_HEIGHT = `-mb-7.5 min-h-7.5 ${ACTION_BAR_PT}`;

  return (
    <MessagePrimitive.Root
      data-role="assistant"
      className="fade-in slide-in-from-bottom-1 animate-in relative duration-150"
    >
      <div className="text-foreground px-2 leading-relaxed wrap-break-word">
        <MessagePrimitive.GroupedParts
          indicator="always"
          groupBy={groupPartByType({
            reasoning: ["group-chainOfThought", "group-reasoning"],
            "tool-call": ["group-chainOfThought", "group-tool"],
            "standalone-tool-call": [],
          })}
        >
          {({ part, children }) => {
            switch (part.type) {
              case "group-chainOfThought":
                return <div>{children}</div>;
              case "group-reasoning": {
                const running = part.status.type === "running";
                return (
                  <ReasoningRoot defaultOpen={running}>
                    <ReasoningTrigger active={running} />
                    <ReasoningContent aria-busy={running}>
                      <ReasoningText>{children}</ReasoningText>
                    </ReasoningContent>
                  </ReasoningRoot>
                );
              }
              case "group-tool":
                return (
                  <ToolGroupRoot>
                    <ToolGroupTrigger
                      count={part.indices.length}
                      active={part.status.type === "running"}
                    />
                    <ToolGroupContent>{children}</ToolGroupContent>
                  </ToolGroupRoot>
                );
              case "text":
                return <DeploymentChatText />;
              case "reasoning":
                return <Reasoning {...part} />;
              case "tool-call":
                return part.toolUI ?? <ToolFallback {...part} />;
              case "indicator":
                return (
                  <DeploymentChatStreamingIndicator
                    deploymentId={deploymentId}
                    agentLabel={agentLabel}
                  />
                );
              case "data":
                // Agent-produced file → download chip (see chat-message-adapter).
                return part.name === ASTRO_FILE_PART ? (
                  <FileDownloadChip
                    file={part.data as ChatAttachment}
                    deploymentId={deploymentId}
                    className="mt-2"
                  />
                ) : null;
              default:
                return null;
            }
          }}
        </MessagePrimitive.GroupedParts>
        <MessageError />
      </div>
      <div className={cn("ms-2 flex items-center", ACTION_BAR_HEIGHT)}>
        <AssistantActionBar />
      </div>
    </MessagePrimitive.Root>
  );
};

const AssistantActionBar: FC = () => (
  <ActionBarPrimitive.Root
    hideWhenRunning
    autohide="not-last"
    className="aui-assistant-action-bar-root text-muted-foreground flex gap-1"
  >
    <ActionBarPrimitive.Copy asChild>
      <TooltipIconButton tooltip="Copy">
        <AuiIf condition={(s) => s.message.isCopied}>
          <CheckIcon />
        </AuiIf>
        <AuiIf condition={(s) => !s.message.isCopied}>
          <CopyIcon />
        </AuiIf>
      </TooltipIconButton>
    </ActionBarPrimitive.Copy>
  </ActionBarPrimitive.Root>
);

const UserMessage: FC<{ deploymentId: string }> = ({ deploymentId }) => (
  <MessagePrimitive.Root
    data-role="user"
    className="fade-in slide-in-from-bottom-1 animate-in flex justify-end px-2 duration-150"
  >
    <div className="bg-muted text-foreground max-w-[min(100%,42rem)] rounded-2xl px-4 py-2.5 text-sm wrap-break-word">
      <div className="empty:hidden mb-1.5 flex flex-wrap justify-end gap-1.5">
        <MessagePrimitive.Attachments>
          {({ attachment }) => (
            <UserAttachmentChip
              attachment={attachment}
              deploymentId={deploymentId}
            />
          )}
        </MessagePrimitive.Attachments>
      </div>
      <MessagePrimitive.Parts />
    </div>
  </MessagePrimitive.Root>
);

// User-attached file chip: resolves the files-API reference the composer adapter
// stashed on the completed attachment, then renders the shared download chip.
const UserAttachmentChip: FC<{
  attachment: CompleteAttachment;
  deploymentId: string;
}> = ({ attachment, deploymentId }) => {
  const file = readAttachmentRef(attachment);
  if (!file) return null;
  return <FileDownloadChip file={file} deploymentId={deploymentId} />;
};

// A single file chip: name + size + a download button. Shared by user-attached
// files (via attachments) and agent-produced files (via a data content part).
const FileDownloadChip: FC<{
  file: ChatAttachment;
  deploymentId: string;
  className?: string;
}> = ({ file, deploymentId, className }) => {
  const download = useDownloadDeploymentFile(deploymentId);
  return (
    <div
      className={cn(
        "flex w-fit items-center gap-2 rounded-lg border border-border bg-surface/70 px-2.5 py-1.5",
        className,
      )}
    >
      <FileIcon className="size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0">
        <p className="max-w-48 truncate text-label text-foreground">{file.name}</p>
        {file.size > 0 ? (
          <p className="text-label text-faint-foreground">
            {formatBytes(file.size)}
          </p>
        ) : null}
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={`Download ${file.name}`}
        disabled={download.isPending}
        onClick={() => download.mutate({ key: file.key, name: file.name })}
      >
        <Download className="size-4" />
      </Button>
    </div>
  );
};
