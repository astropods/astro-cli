import { PanelRight, SquarePen } from "lucide-react";
import { useState } from "react";
import type { AgentDeploymentSummary } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { ChatSession } from "@/lib/chat/types";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useDeployments, useDeploymentsSummary } from "@/api/queries/deployments";
import { accountBlueprintsPath, chatDeploymentPath, explorePath } from "@/lib/routes";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ConversationHistoryDropdown } from "./ConversationHistoryDropdown";
import { ChatAgentSwitchCoachmark } from "./ChatAgentSwitchCoachmark";

// Set once the user opens the agent switcher or dismisses the coachmark, so
// the first-run nudge never shows again. Plain localStorage (not the shared
// experiments store) since this is a simple one-shot dismissal flag.
const COACHMARK_SEEN_KEY = "astro:chat-agent-switch-coachmark-seen";

function coachmarkSeen(): boolean {
  try {
    return localStorage.getItem(COACHMARK_SEEN_KEY) === "true";
  } catch {
    return false;
  }
}

/**
 * Where the "Deploy more agents" footer should point for a single-agent user:
 * the blueprints page when the account still has undeployed blueprints,
 * otherwise the Explore catalog to find new ones. Only fetches while `enabled`
 * (the footer can actually show).
 */
function useDeployMoreHref(account: string, enabled: boolean): string {
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
  // Mirror AgentDeploymentMenu's switch list off the same summary query, not
  // eligibleDeploymentIds — otherwise the two disagree while summary loads and
  // the coachmark points at a switcher that only shows the deploy-more footer.
  const { data: summaryData } = useDeploymentsSummary();
  const hasSwitchList = (summaryData?.accounts ?? []).some((acct) =>
    acct.deployments.some(
      (dep) => dep.id !== deployment.id && eligibleDeploymentIds.has(dep.id),
    ),
  );
  const deployMoreHref = useDeployMoreHref(account, !hasSwitchList);
  const [seen, setSeen] = useState(coachmarkSeen);
  // Only point the coachmark at the switcher when there's actually another
  // agent to switch to; otherwise the menu shows the deploy-more footer.
  const showCoachmark = !seen && hasSwitchList;
  const dismissCoachmark = () => {
    try {
      localStorage.setItem(COACHMARK_SEEN_KEY, "true");
    } catch {
      // localStorage may be unavailable (SSR, private mode); ignore.
    }
    setSeen(true);
  };

  return (
    <header className="@container relative z-10 flex h-[52px] shrink-0 items-center gap-3 border-b border-border bg-background px-3 md:px-4">
      <AgentDeploymentMenu
        deployment={deployment}
        variant="detail"
        triggerClassName={cn(
          "dark:mt-0 dark:ml-0",
          showCoachmark &&
            "bg-primary/10 ring-1 ring-inset ring-primary/50 dark:bg-primary-400/10 dark:ring-primary-400/50",
        )}
        eligibleDeploymentIds={eligibleDeploymentIds}
        getDeploymentPath={(_acct, dep) => chatDeploymentPath(dep.id)}
        showAccountLabels
        deployMoreHref={deployMoreHref}
        onOpenChange={(open) => {
          if (open) dismissCoachmark();
        }}
      />
      {showCoachmark ? (
        <ChatAgentSwitchCoachmark onClose={dismissCoachmark} />
      ) : null}

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
