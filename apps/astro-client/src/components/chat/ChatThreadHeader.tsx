import {
  ArrowLeftRight,
  ExternalLink,
  PanelRight,
  SquarePen,
  X,
} from "lucide-react";
import { useCallback, useState } from "react";
import type { AgentDeploymentSummary } from "@/lib/api";
import { cn } from "@/lib/utils";
import { DEFAULT_CONVERSATION_TITLE, type ChatSession } from "@/lib/chat/types";
import { AgentDeploymentMenu } from "@/components/agent-detail/AgentDeploymentMenu";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import {
  useDeployments,
  useDeploymentsSummary,
} from "@/api/queries/deployments";
import {
  accountBlueprintsPath,
  chatDeploymentPath,
  explorePath,
} from "@/lib/routes";
import { Button } from "@/components/ui/button";
import { Coachmark } from "@/components/ui/coachmark";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ConversationHistoryDropdown } from "./ConversationHistoryDropdown";

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
  // eligibleDeploymentIds, otherwise the two disagree while summary loads and
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

  const activeSession = sessions.find(
    (s) => s.conversationId === activeConversationId,
  );
  const activeTitle = activeSession
    ? activeSession.title.trim() || DEFAULT_CONVERSATION_TITLE
    : undefined;
  const [editingTitle, setEditingTitle] = useState(false);
  const [draftTitle, setDraftTitle] = useState("");

  const startTitleEdit = () => {
    if (!activeSession || !onRenameSession) return;
    setDraftTitle(activeSession.title);
    setEditingTitle(true);
  };

  const commitTitleEdit = () => {
    if (!activeSession) {
      setEditingTitle(false);
      setDraftTitle("");
      return;
    }
    const next = draftTitle.trim();
    if (next && next !== activeSession.title) {
      onRenameSession?.(activeSession.conversationId, next);
    }
    setEditingTitle(false);
    setDraftTitle("");
  };

  const cancelTitleEdit = () => {
    setEditingTitle(false);
    setDraftTitle("");
  };

  const focusTitleInput = useCallback((el: HTMLInputElement | null) => {
    if (el) {
      el.focus();
      el.select();
    }
  }, []);

  return (
    <header className="@container relative z-10 flex h-[52px] shrink-0 items-center gap-3 border-b border-border/60 px-3 md:px-4">
      <Coachmark
        open={showCoachmark}
        anchor={
          <AgentDeploymentMenu
            deployment={deployment}
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
        }
        announcement="Switch agents here"
        sideOffset={8}
        contentClassName="flex items-center gap-2.5 py-2 pl-3 pr-2 text-body text-foreground"
      >
        <ArrowLeftRight className="size-4 shrink-0 text-foreground-accent" />
        <span className="whitespace-nowrap font-medium">
          Switch agents here
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Dismiss"
          onClick={dismissCoachmark}
          className="-mr-0.5 ml-1 text-muted-foreground hover:text-foreground"
        >
          <X className="size-3.5" />
        </Button>
      </Coachmark>

      <div
        aria-hidden
        className="h-5 shrink-0 border-l border-border/70 dark:border-white/10"
      />

      {activeTitle ? (
        <div className="flex min-w-0 flex-1 items-center gap-1.5">
          {editingTitle ? (
            <div className="relative -ml-1.5 inline-block max-w-full min-w-0 align-middle">
              <span
                aria-hidden
                className="invisible block max-w-full truncate whitespace-pre rounded-sm border border-transparent px-1.5 py-1 text-body font-normal"
              >
                {draftTitle || DEFAULT_CONVERSATION_TITLE}
              </span>
              <input
                ref={focusTitleInput}
                value={draftTitle}
                onChange={(e) => setDraftTitle(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    commitTitleEdit();
                  } else if (e.key === "Escape") {
                    e.preventDefault();
                    cancelTitleEdit();
                  }
                }}
                onBlur={commitTitleEdit}
                maxLength={200}
                placeholder={DEFAULT_CONVERSATION_TITLE}
                aria-label="Conversation title"
                className="absolute inset-0 h-full w-full min-w-0 rounded-sm border border-transparent bg-muted/60 px-1.5 py-1 text-body font-normal text-foreground outline-none transition-colors focus:border-ring focus:bg-card dark:bg-muted dark:focus:bg-muted"
              />
            </div>
          ) : onRenameSession ? (
            <button
              type="button"
              className="-ml-1.5 max-w-full min-w-0 cursor-text truncate rounded-sm px-1.5 py-1 text-left text-body font-normal text-foreground transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:hover:bg-muted"
              title={activeTitle}
              aria-label="Rename conversation title"
              onClick={startTitleEdit}
            >
              {activeTitle}
            </button>
          ) : (
            <p
              className="truncate text-body font-normal text-foreground"
              title={activeTitle}
            >
              {activeTitle}
            </p>
          )}
          {editingTitle ? null : (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <a
                    href={chatDeploymentPath(
                      deployment.id,
                      activeConversationId,
                    )}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label="Open chat in new tab"
                    className="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
                  >
                    <ExternalLink className="size-3" />
                  </a>
                </TooltipTrigger>
                <TooltipContent side="bottom">Open in new tab</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
        </div>
      ) : (
        <span className="min-w-0 flex-1" aria-hidden />
      )}

      <div className="flex shrink-0 items-center gap-3">
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
                    inspectorOpen &&
                      "bg-muted text-foreground ring-1 ring-inset ring-border/70 dark:bg-muted dark:ring-white/12",
                  )}
                  aria-label="Agent details"
                  aria-pressed={inspectorOpen}
                  onClick={onToggleInspector}
                >
                  <PanelRight className="hidden size-4 shrink-0 md:block" />
                  <span className="leading-none text-foreground">Details</span>
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
