import { SquarePen } from "lucide-react";
import type { AgentDeploymentSummary } from "@/lib/api";
import type { ChatSession } from "@/lib/chat/types";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { chatDeploymentPath } from "@/lib/routes";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ConversationHistoryDropdown } from "./ConversationHistoryDropdown";

export function ChatThreadHeader({
  deployment,
  eligibleDeploymentIds,
  sessions,
  activeConversationId,
  onSelectSession,
  onRenameSession,
  onDeleteSession,
  onNewConversation,
}: {
  deployment: AgentDeploymentSummary;
  /** Chat-eligible deployments to show in the agent switch list. */
  eligibleDeploymentIds: ReadonlySet<string>;
  sessions: ChatSession[];
  activeConversationId?: string | null;
  onSelectSession: (conversationId: string) => void;
  onRenameSession?: (conversationId: string, title: string) => void;
  onDeleteSession?: (conversationId: string) => void;
  onNewConversation?: () => void;
}) {
  return (
    <header className="relative z-10 flex h-[52px] shrink-0 items-center gap-3 border-b border-border bg-background px-3 md:px-4">
      <AgentDeploymentMenu
        deployment={deployment}
        variant="detail"
        eligibleDeploymentIds={eligibleDeploymentIds}
        getDeploymentPath={(_acct, dep) => chatDeploymentPath(dep.id)}
        showAccountLabels
      />

      <span className="min-w-0 flex-1" aria-hidden />

      <div className="flex shrink-0 items-center gap-1">
        {onNewConversation ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8"
                  aria-label="New chat"
                  onClick={onNewConversation}
                >
                  <SquarePen className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">New chat</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : null}

        <ConversationHistoryDropdown
          sessions={sessions}
          activeConversationId={activeConversationId}
          onSelectSession={onSelectSession}
          onRenameSession={onRenameSession}
          onDeleteSession={onDeleteSession}
        />
      </div>
    </header>
  );
}
