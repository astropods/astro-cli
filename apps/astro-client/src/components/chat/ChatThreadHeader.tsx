import { PanelRight, SquarePen } from "lucide-react";
import type { AgentDeploymentSummary } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { ChatSession } from "@/lib/chat/types";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useDeployments } from "@/api/queries/deployments";
import { accountBlueprintsPath, chatDeploymentPath, explorePath } from "@/lib/routes";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ConversationHistoryDropdown } from "./ConversationHistoryDropdown";

/**
 * Where the "Deploy more agents" footer should point for a single-agent user:
 * the blueprints page when the account still has undeployed blueprints,
 * otherwise the Explore catalog to find new ones. Only fetches while `enabled`
 * (the footer can actually show).
 */
function useGrowFleetHref(account: string, enabled: boolean): string {
  const { data: blueprints } = useAccountBlueprints(account, { enabled });
  const { data: deployments } = useDeployments(account, enabled);
  if (!blueprints || !deployments) return accountBlueprintsPath;
  const deployedNames = new Set(deployments.deployments.map((d) => d.name));
  const hasUndeployedBlueprints = blueprints.agents.some(
    (bp) => !bp.archived_at && !deployedNames.has(bp.name),
  );
  return hasUndeployedBlueprints ? accountBlueprintsPath : explorePath;
}

export function ChatThreadHeader({
  account,
  deployment,
  eligibleDeploymentIds,
  sessions,
  activeConversationId,
  onSelectSession,
  onRenameSession,
  onDeleteSession,
  onNewConversation,
  inspectorOpen,
  onToggleInspector,
}: {
  account: string;
  deployment: AgentDeploymentSummary;
  /** Chat-eligible deployments to show in the agent switch list. */
  eligibleDeploymentIds: ReadonlySet<string>;
  sessions: ChatSession[];
  activeConversationId?: string | null;
  onSelectSession: (conversationId: string) => void;
  onRenameSession?: (conversationId: string, title: string) => void;
  onDeleteSession?: (conversationId: string) => void;
  onNewConversation?: () => void;
  inspectorOpen?: boolean;
  onToggleInspector?: () => void;
}) {
  // Single chat-eligible agent → the switch list is empty, so the menu shows
  // the grow-fleet footer instead. Only then do we need its link target.
  const growFleetHref = useGrowFleetHref(account, eligibleDeploymentIds.size <= 1);

  return (
    <header className="@container relative z-10 flex h-[52px] shrink-0 items-center gap-3 border-b border-border bg-background px-3 md:px-4">
      <AgentDeploymentMenu
        deployment={deployment}
        variant="detail"
        triggerClassName="dark:mt-0 dark:ml-0"
        eligibleDeploymentIds={eligibleDeploymentIds}
        getDeploymentPath={(_acct, dep) => chatDeploymentPath(dep.id)}
        showAccountLabels
        growFleetHref={growFleetHref}
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

        {onToggleInspector ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className={cn(
                    "h-8 gap-1.5 px-2.5 text-body-sm font-medium",
                    inspectorOpen && "bg-muted text-foreground",
                  )}
                  aria-label="Agent details"
                  aria-pressed={inspectorOpen}
                  onClick={onToggleInspector}
                >
                  <PanelRight className="size-4 text-foreground" />
                  <span className="hidden text-foreground sm:inline">Details</span>
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">Agent details</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : null}
      </div>
    </header>
  );
}
