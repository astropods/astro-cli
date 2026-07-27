import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { Link } from "react-router";
import { Check, ChevronDown, Plus, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { useDeploymentsSummary } from "@/api/queries/deployments";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { comparePersonalFirst } from "@/lib/account-order";

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
   * chat-eligible agent), the selector shows the current agent as the selected row
   * plus a footer linking here to deploy more agents from blueprints.
   */
  deployMoreHref?: string;
  /** Notified when the selector opens or closes. */
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
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState("");
  const listboxId = useId();
  const searchInputRef = useRef<HTMLInputElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);

  const { data: summaryData } = useDeploymentsSummary();
  const normalizedSearch = search.trim().toLocaleLowerCase();

  // Keep the current agent (shown as the selected row); drop only ineligible ones.
  const eligibleAccounts = (summaryData?.accounts ?? [])
    .map((acct) => {
      const eligibleDeployments = acct.deployments.filter((dep) => {
        // Always show the current agent as the selected row, before the
        // eligibility gate: it's the agent being viewed/chatted, so it's
        // inherently eligible (the detail page passes no eligibility set).
        if (dep.id === deployment.id) return true;
        if (eligibleDeploymentIds && !eligibleDeploymentIds.has(dep.id)) {
          return false;
        }
        return true;
      });
      const currentDeployment = eligibleDeployments.find(
        (dep) => dep.id === deployment.id,
      );
      return {
        ...acct,
        deployments: currentDeployment
          ? [
              currentDeployment,
              ...eligibleDeployments.filter(
                (dep) => dep.id !== deployment.id,
              ),
            ]
          : eligibleDeployments,
      };
    })
    .filter((acct) => acct.deployments.length > 0)
    .sort((a, b) => {
      const aIsCurrent = a.deployments[0]?.id === deployment.id;
      const bIsCurrent = b.deployments[0]?.id === deployment.id;
      if (aIsCurrent !== bIsCurrent) return aIsCurrent ? -1 : 1;
      // After the current account, retain the ordering shared with the org switcher.
      return comparePersonalFirst(a, b);
    });

  const accounts = eligibleAccounts
    .map((acct) => {
      const accountMatches = [acct.display_name, acct.name].some((value) =>
        value.toLocaleLowerCase().includes(normalizedSearch),
      );
      return {
        ...acct,
        deployments:
          normalizedSearch && !accountMatches
            ? acct.deployments.filter((dep) =>
                [dep.display_name, dep.name].some((value) =>
                  value?.toLocaleLowerCase().includes(normalizedSearch),
                ),
              )
            : acct.deployments,
      };
    })
    .filter((acct) => acct.deployments.length > 0);

  const hasOtherAgents = eligibleAccounts.some((acct) =>
    acct.deployments.some((dep) => dep.id !== deployment.id),
  );

  // With a single chat-eligible agent there is nothing to switch to, so prompt
  // the user to deploy more agents instead of opening to an empty panel.
  const showDeployMore = !!deployMoreHref && !hasOtherAgents;
  const handleOpenChange = (open: boolean) => {
    setIsOpen(open);
    if (!open) setSearch("");
    onOpenChange?.(open);
  };
  useEffect(() => {
    if (!isOpen) return;
    const frame = requestAnimationFrame(() => searchInputRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [isOpen]);
  const handleSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (
      event.key !== "ArrowDown" &&
      event.key !== "ArrowUp" &&
      event.key !== "Escape" &&
      event.key !== "Tab"
    ) {
      event.stopPropagation();
    }
  };
  // No shared command/combobox primitive exists yet. Keyboard traversal follows
  // the rendered DOM order; controls may opt out of Arrow roving while staying
  // in the native Tab order.
  const getKeyboardTargets = () =>
    Array.from(
      contentRef.current?.querySelectorAll<HTMLElement>(
        'input:not([disabled]), button:not([disabled]), a[href], [role="option"]:not([aria-disabled="true"])',
      ) ?? [],
    );
  const getArrowTargets = () =>
    getKeyboardTargets().filter(
      (element) => !element.hasAttribute("data-arrow-roving-skip"),
    );
  const handleContentKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Tab") {
      const targets = getKeyboardTargets();
      const index = targets.indexOf(event.target as HTMLElement);
      const remainingTargets = event.shiftKey
        ? targets.slice(0, index).reverse()
        : targets.slice(index + 1);
      if (!remainingTargets.some((element) => element.tabIndex >= 0)) {
        handleOpenChange(false);
      }
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;

    const targets = getArrowTargets();
    const index = targets.indexOf(event.target as HTMLElement);
    if (index < 0) return;

    event.preventDefault();
    const nextIndex = event.key === "ArrowDown" ? index + 1 : index - 1;
    targets[nextIndex]?.focus();
  };

  return (
    <Popover open={isOpen} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="Agent menu"
          aria-haspopup="listbox"
          aria-controls={isOpen ? listboxId : undefined}
          className={cn(
            "flex min-w-0 max-w-full cursor-pointer items-center gap-3 rounded-[8px] bg-transparent p-1 pl-1 pr-2.5 outline-none transition-colors hover:bg-black/5 focus-visible:ring-2 focus-visible:ring-ring/50 dark:-ml-2 dark:-mt-1.5 dark:rounded-md dark:bg-transparent dark:p-1.5 dark:pl-2 dark:pr-3 dark:hover:bg-white/5",
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
              "min-w-0 whitespace-nowrap text-base font-medium tracking-wide text-foreground max-[500px]:hidden",
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
      </PopoverTrigger>
      {/* Radix defaults popovers to dialog; this non-modal selector exposes its
          searchbox and listbox directly instead. */}
      <PopoverContent
        ref={contentRef}
        role={undefined}
        variant="panel"
        side="bottom"
        align="start"
        sideOffset={4}
        onKeyDown={handleContentKeyDown}
        className="flex w-[300px] max-w-[calc(100vw-2rem)] flex-col overflow-hidden"
      >
        <div
          role="search"
          className="flex h-11 shrink-0 items-center gap-2 border-b border-border px-3"
        >
          <Search className="size-3.5 shrink-0 text-muted-foreground" />
          <input
            ref={searchInputRef}
            type="search"
            aria-label="Search agents"
            aria-controls={listboxId}
            placeholder="Search agents or accounts"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={handleSearchKeyDown}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-faint-foreground"
          />
          {search && (
            <button
              type="button"
              aria-label="Clear agent search"
              data-arrow-roving-skip
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => {
                setSearch("");
                searchInputRef.current?.focus();
              }}
              className="rounded-sm p-0.5 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>

        {menuPrefix && (
          <>
            <div className="p-1" onClick={() => handleOpenChange(false)}>
              {menuPrefix}
            </div>
            <div role="separator" className="h-px shrink-0 bg-border" />
          </>
        )}

        <ScrollArea
          type="auto"
          className="min-h-0 max-h-[360px] flex-1"
          viewportClassName="max-h-[360px]"
        >
          <div className="p-1 pr-2">
            <div
              id={listboxId}
              role="listbox"
              aria-label="Agents"
            >
              {accounts.map((acct, i) => {
                const showAccountLabel =
                  showAccountLabels || eligibleAccounts.length > 1;
                const accountLabelId = `${listboxId}-account-${i}`;
                return (
                  <div
                    key={acct.id}
                    role="group"
                    aria-label={
                      showAccountLabel
                        ? undefined
                        : acct.display_name || acct.name
                    }
                    aria-labelledby={
                      showAccountLabel ? accountLabelId : undefined
                    }
                    className={cn(i > 0 && "mt-3")}
                  >
                    {showAccountLabel && (
                      <div
                        id={accountLabelId}
                        className="px-2 pt-1 pb-1.5 text-xs font-medium text-faint-foreground"
                      >
                        {acct.display_name || acct.name}
                      </div>
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
                      // The current agent remains in the listbox for context,
                      // but is disabled because selecting it would be a no-op.
                      return isCurrent ? (
                        <div
                          key={dep.id}
                          role="option"
                          aria-label={dep.display_name || dep.name}
                          aria-selected
                          aria-disabled
                          aria-current="true"
                          className="flex items-center gap-2 rounded-sm bg-accent/60 px-2 py-1.5 text-sm"
                        >
                          {row}
                        </div>
                      ) : (
                        <Link
                          key={dep.id}
                          to={getDeploymentPath(acct.name, dep)}
                          role="option"
                          aria-label={dep.display_name || dep.name}
                          aria-selected={false}
                          tabIndex={-1}
                          onClick={() => handleOpenChange(false)}
                          className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground"
                        >
                          {row}
                        </Link>
                      );
                    })}
                  </div>
                );
              })}
            </div>
            {accounts.length === 0 && (
              <div className="px-3 py-8 text-center">
                <p className="text-sm font-medium text-foreground">
                  No agents found
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Try another agent or account name.
                </p>
              </div>
            )}
          </div>
        </ScrollArea>

        {showDeployMore && (
          <>
            <div role="separator" className="h-px shrink-0 bg-border" />
            <Button
              asChild
              variant="ghost"
              size="sm"
              className="m-1 w-auto shrink-0 justify-start gap-1 font-medium"
              onClick={() => handleOpenChange(false)}
            >
              <Link to={deployMoreHref!}>
                <Plus className="size-3.5 text-muted-foreground" />
                Deploy more agents
              </Link>
            </Button>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
