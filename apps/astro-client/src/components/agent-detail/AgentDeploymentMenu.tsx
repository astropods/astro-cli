import type { ReactNode } from "react";
import { Link } from "react-router";
import { ArrowRight, Check, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { useDeploymentsSummary } from "@/api/queries/deployments";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
  DropdownMenuGroup,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

export interface AgentDeploymentMenuTarget {
  id: string;
  name: string;
  display_name?: string;
  avatar_url?: string;
}

interface AgentDeploymentMenuProps {
  deployment: AgentDeploymentMenuTarget;
  /** Build the route when the user picks another deployment. */
  getDeploymentPath: (account: string, deployment: AgentDeploymentMenuTarget) => string;
  /** When set, only these deployment ids appear in the switch list. */
  eligibleDeploymentIds?: ReadonlySet<string>;
  /** Rendered above the agent switch list (e.g. blueprint link, restart). */
  menuPrefix?: ReactNode;
  /** Extra classes merged onto the trigger button (e.g. to tune alignment per host). */
  triggerClassName?: string;
  /** When true, render the current deployment name without the fade mask. */
  showFullName?: boolean;
  /**
   * Always render the org/account label per group. Defaults to only showing it
   * when more than one account has agents (the detail-page behavior). Chat sets
   * this so agents stay separated by org even when one org is in the list.
   */
  showAccountLabels?: boolean;
  /**
   * When set and there are no other agents to switch to (the user has a single
   * chat-eligible agent), the menu shows the current agent as the selected row
   * plus a footer linking here to deploy more agents from blueprints.
   */
  deployMoreHref?: string;
  /** Notified when the switch menu opens or closes. */
  onOpenChange?: (open: boolean) => void;
}

export function AgentDeploymentMenu({
  deployment,
  getDeploymentPath,
  eligibleDeploymentIds,
  menuPrefix,
  triggerClassName,
  showFullName = false,
  showAccountLabels = false,
  deployMoreHref,
  onOpenChange,
}: AgentDeploymentMenuProps) {
  const displayName = deployment.display_name || deployment.name;

  const { data: summaryData } = useDeploymentsSummary();
  // Keep the current agent (shown as the selected row); drop only ineligible ones.
  const accounts = (summaryData?.accounts ?? [])
    .map((acct) => ({
      ...acct,
      deployments: acct.deployments.filter((dep) => {
        // Always show the current agent as the selected row, before the
        // eligibility gate: it's the agent being viewed/chatted, so it's
        // inherently eligible (the detail page passes no eligibility set).
        if (dep.id === deployment.id) return true;
        if (eligibleDeploymentIds && !eligibleDeploymentIds.has(dep.id)) {
          return false;
        }
        return true;
      }),
    }))
    .filter((acct) => acct.deployments.length > 0);

  const hasOtherAgents = accounts.some((acct) =>
    acct.deployments.some((dep) => dep.id !== deployment.id),
  );

  // With a single chat-eligible agent there is nothing to switch to, so prompt
  // the user to deploy more agents instead of opening to an empty panel.
  const showDeployMore = !!deployMoreHref && !hasOtherAgents;
  const hasContent = showDeployMore || accounts.length > 0;
  const currentAccount = (summaryData?.accounts ?? []).find((acct) =>
    acct.deployments.some((dep) => dep.id === deployment.id),
  );
  const currentAccountLabel =
    currentAccount?.display_name || currentAccount?.name;

  return (
    <DropdownMenu onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Agent menu"
          className={cn(
            "flex cursor-pointer items-center gap-3 rounded-[8px] bg-transparent p-1 pl-1 pr-2.5 outline-none transition-colors hover:bg-black/5 focus-visible:ring-2 focus-visible:ring-ring/50 dark:-ml-2 dark:-mt-1.5 dark:rounded-md dark:bg-transparent dark:p-1.5 dark:pl-2 dark:pr-3 dark:hover:bg-white/5",
            triggerClassName,
          )}
        >
          <DeploymentAvatar
            deployment={deployment}
            size={32}
            className="rounded-sm"
          />
          <span
            className={cn(
              "whitespace-nowrap text-base font-medium tracking-wide text-foreground @max-[500px]:hidden",
              showFullName
                ? "max-w-[calc(100vw-8rem)] overflow-hidden text-ellipsis min-[900px]:max-w-[42rem]"
                : "max-w-[6rem] overflow-hidden [--fade-start:4rem] [--fade-end:6rem] min-[600px]:max-w-[8rem] min-[600px]:[--fade-start:6rem] min-[600px]:[--fade-end:8rem] min-[820px]:max-w-[10rem] min-[820px]:[--fade-start:8rem] min-[820px]:[--fade-end:10rem] min-[1100px]:max-w-[18rem] min-[1100px]:[--fade-start:16rem] min-[1100px]:[--fade-end:18rem]",
            )}
            style={
              showFullName
                ? undefined
                : {
                    maskImage:
                      "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
                    WebkitMaskImage:
                      "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
                  }
            }
          >
            {displayName}
          </span>
          <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="flex w-[260px] flex-col">
        {menuPrefix}
        {menuPrefix && hasContent && <DropdownMenuSeparator />}
        {showDeployMore ? (
          <>
            <DropdownMenuGroup>
              {currentAccountLabel && (
                <DropdownMenuLabel className="px-2 pt-1 pb-1.5 text-xs font-medium text-faint-foreground">
                  {currentAccountLabel}
                </DropdownMenuLabel>
              )}
              <div
                aria-current="true"
                className="flex items-center gap-2 rounded-sm bg-accent/60 px-2 py-1.5 text-sm"
              >
                <DeploymentAvatar
                  deployment={deployment}
                  size={20}
                  className="size-5 shrink-0 rounded-sm"
                />
                <span className="truncate">{displayName}</span>
                <Check className="ml-auto size-4 shrink-0 text-foreground-accent" />
              </div>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <Button
              asChild
              variant="ghost"
              size="sm"
              className="mt-1 w-full justify-start gap-1 font-medium"
            >
              <Link to={deployMoreHref!}>
                Deploy more agents
                <ArrowRight className="size-3 text-muted-foreground" />
              </Link>
            </Button>
          </>
        ) : (
          <div className="max-h-[320px] overflow-y-auto">
            {accounts.map((acct, i) => (
              <DropdownMenuGroup key={acct.id} className={cn(i > 0 && "mt-3")}>
                {(showAccountLabels || accounts.length > 1) && (
                  <DropdownMenuLabel className="px-2 pt-1 pb-1.5 text-xs font-medium text-faint-foreground">
                    {acct.display_name || acct.name}
                  </DropdownMenuLabel>
                )}
                {acct.deployments.map((dep) => {
                  const isCurrent = dep.id === deployment.id;
                  const row = (
                    <>
                      <DeploymentAvatar
                        deployment={dep}
                        size={20}
                        className="size-5 shrink-0 rounded-sm"
                      />
                      <span className="truncate">
                        {dep.display_name || dep.name}
                      </span>
                      {isCurrent && (
                        <Check className="ml-auto size-4 shrink-0 text-foreground-accent" />
                      )}
                    </>
                  );
                  // The current agent is presentational, not a menuitem, so it
                  // stays out of roving focus and can't dismiss the menu as a no-op.
                  return isCurrent ? (
                    <div
                      key={dep.id}
                      aria-current="true"
                      className="flex items-center gap-2 rounded-sm bg-accent/60 px-2 py-1.5 text-sm"
                    >
                      {row}
                    </div>
                  ) : (
                    <DropdownMenuItem key={dep.id} asChild className="gap-2">
                      <Link to={getDeploymentPath(acct.name, dep)}>{row}</Link>
                    </DropdownMenuItem>
                  );
                })}
              </DropdownMenuGroup>
            ))}
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
