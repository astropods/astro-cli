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
import { ChatButton } from "@/components/assistant-ui/chat-button";
import { TooltipIconButton } from "@/components/assistant-ui/tooltip-icon-button";
import { DeploymentChatHistoryScroll } from "@/components/chat/DeploymentChatHistoryScroll";
import { DeploymentChatStreamingIndicator } from "@/components/chat/DeploymentChatStreamingIndicator";
import { DeploymentChatText } from "@/components/chat/DeploymentChatText";
import { useDeploymentChatViewport } from "@/components/chat/deployment-chat-streaming-context";
import { cn } from "@/lib/utils";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import type { AgentDeploymentSummary } from "@/lib/api";
import {
  ActionBarPrimitive,
  AuiIf,
  ComposerPrimitive,
  ErrorPrimitive,
  groupPartByType,
  MessagePrimitive,
  ThreadPrimitive,
  useAuiState,
} from "@assistant-ui/react";
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CheckIcon,
  CopyIcon,
  SquareIcon,
} from "lucide-react";
import type { FC } from "react";

export function DeploymentChatThreadView({
  account,
  deploymentId,
  deployment,
  agentLabel,
  composerDisabled,
  disabledReason,
}: {
  account: string;
  deploymentId: string;
  deployment?: AgentDeploymentSummary;
  agentLabel: string;
  composerDisabled?: boolean;
  disabledReason?: string;
}) {
  const { conversationId, isStreaming, historyLoading, streamError } =
    useDeploymentChatViewport();
  const useTopTurnAnchor = isStreaming && !historyLoading;
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const avatarUrl = avatarBust ?? getDeploymentAvatarUrl(deploymentId);

  return (
    <ThreadPrimitive.Root
      className="aui-root aui-thread-root chat-pane-bg @container flex h-full min-h-0 flex-1 flex-col"
      style={{
        ["--thread-max-width" as string]: "44rem",
        ["--composer-radius" as string]: "20px",
        ["--composer-padding" as string]: "10px",
      }}
    >
      <ThreadPrimitive.Viewport
        key={conversationId ?? "draft"}
        turnAnchor={useTopTurnAnchor ? "top" : "bottom"}
        autoScroll={useTopTurnAnchor}
        scrollToBottomOnInitialize
        className="chat-thread-scroll relative flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto"
      >
        <DeploymentChatHistoryScroll />
        <div className="mx-auto flex w-full max-w-(--thread-max-width) min-h-0 flex-1 flex-col px-3 pt-3 md:px-6 md:pt-4">
          <AuiIf condition={(s) => s.thread.isEmpty}>
            <ThreadWelcome
              account={account}
              deployment={deployment}
              avatarUrl={avatarUrl}
              agentLabel={agentLabel}
            />
          </AuiIf>

          <div className="mb-6 flex flex-col gap-y-6 empty:hidden md:mb-8 md:gap-y-8">
            <ThreadPrimitive.Messages>
              {() => (
                <ThreadMessage deploymentId={deploymentId} agentLabel={agentLabel} />
              )}
            </ThreadPrimitive.Messages>
          </div>

          <ThreadPrimitive.ViewportFooter className="aui-thread-viewport-footer bg-background sticky bottom-0 mt-auto flex flex-col gap-2 overflow-visible rounded-t-(--composer-radius) pb-3 md:pb-4">
            <ThreadScrollToBottom />
            {streamError ? (
              <p className="px-1 text-xs text-destructive" role="alert">
                {streamError}
              </p>
            ) : null}
            {disabledReason ? (
              <p className="px-1 text-xs text-muted-foreground">{disabledReason}</p>
            ) : null}
            <DeploymentComposer disabled={composerDisabled} agentLabel={agentLabel} />
          </ThreadPrimitive.ViewportFooter>
        </div>
      </ThreadPrimitive.Viewport>
    </ThreadPrimitive.Root>
  );
}

const ThreadMessage: FC<{
  deploymentId: string;
  agentLabel: string;
}> = ({ deploymentId, agentLabel }) => {
  const role = useAuiState((s) => s.message.role);
  if (role === "user") return <UserMessage />;
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
  account: string;
  deployment?: AgentDeploymentSummary;
  avatarUrl?: string;
  agentLabel: string;
}> = ({ account, deployment, avatarUrl, agentLabel }) => (
  <div className="aui-thread-welcome-root my-auto flex grow flex-col">
    <div className="aui-thread-welcome-center flex w-full grow flex-col items-center justify-center py-8">
      <div className="aui-thread-welcome-message flex flex-col items-center gap-3 px-4 text-center">
        {deployment ? (
          <BlueprintIdentity
            account={account}
            name={deployment.name}
            size={48}
            url={avatarUrl}
            className="size-12 rounded-xl"
          />
        ) : null}
        <div className="flex flex-col items-center gap-1">
          <h1 className="text-xl font-semibold tracking-tight text-foreground md:text-2xl">
            {agentLabel}
          </h1>
          <p className="text-muted-foreground text-sm md:text-base">
            Send a message below to start a new chat.
          </p>
        </div>
      </div>
    </div>
  </div>
);

const DeploymentComposer: FC<{ disabled?: boolean; agentLabel: string }> = ({
  disabled,
  agentLabel,
}) => (
  <ComposerPrimitive.Root className="aui-composer-root relative flex w-full flex-col">
    <div
      data-slot="aui_composer-shell"
      className={cn(
        "bg-surface/70 flex w-full flex-col gap-2 rounded-(--composer-radius) border border-input p-(--composer-padding) transition-[border-color,box-shadow]",
        "focus-within:border-primary/70 focus-within:ring-2 focus-within:ring-primary/15",
        disabled && "pointer-events-none opacity-60",
      )}
    >
      <ComposerPrimitive.Input
        placeholder={
          disabled ? "Agent is not ready…" : `Send a message to ${agentLabel}…`
        }
        disabled={disabled}
        className="aui-composer-input placeholder:text-muted-foreground/80 max-h-32 min-h-10 w-full resize-none bg-transparent px-1.75 py-1 text-sm outline-none disabled:cursor-not-allowed"
        rows={1}
        aria-label="Message input"
      />
      <ComposerAction disabled={disabled} />
    </div>
  </ComposerPrimitive.Root>
);

const ComposerAction: FC<{ disabled?: boolean }> = ({ disabled }) => (
  <div className="aui-composer-action-wrapper relative flex items-center justify-end">
    <AuiIf condition={(s) => !s.thread.isRunning}>
      <ComposerPrimitive.Send asChild>
        <TooltipIconButton
          tooltip="Send message"
          side="bottom"
          type="button"
          variant="default"
          size="icon"
          disabled={disabled}
          className="aui-composer-send size-8 rounded-full"
          aria-label="Send message"
        >
          <ArrowUpIcon className="aui-composer-send-icon size-4" />
        </TooltipIconButton>
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

const UserMessage: FC = () => (
  <MessagePrimitive.Root
    data-role="user"
    className="fade-in slide-in-from-bottom-1 animate-in flex justify-end px-2 duration-150"
  >
    <div className="bg-muted text-foreground max-w-[min(100%,42rem)] rounded-2xl px-4 py-2.5 text-sm wrap-break-word">
      <MessagePrimitive.Parts />
    </div>
  </MessagePrimitive.Root>
);
