import type { ReactNode } from "react";
import { Link } from "react-router";
import { ChevronDown } from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
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
import { inputBase, inputFocusVisible } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export interface AgentDeploymentMenuTarget {
  id: string;
  name: string;
  display_name?: string;
  avatar_url?: string;
}

interface AgentDeploymentMenuProps {
  account: string;
  deployment: AgentDeploymentMenuTarget;
  /** Build the route when the user picks another deployment. */
  getDeploymentPath: (account: string, deployment: AgentDeploymentMenuTarget) => string;
  /** When set, only these deployment ids appear in the switch list. */
  eligibleDeploymentIds?: ReadonlySet<string>;
  /** Rendered above the agent switch list (e.g. blueprint link, restart). */
  menuPrefix?: ReactNode;
  /** `detail` matches the agent detail page trigger; `header` is the chat bar. */
  variant?: "detail" | "header";
  /**
   * Always render the org/account label per group. Defaults to only showing it
   * when more than one account has agents (the detail-page behavior). Chat sets
   * this so agents stay separated by org even when one org is in the list.
   */
  showAccountLabels?: boolean;
}

export function AgentDeploymentMenu({
  account,
  deployment,
  getDeploymentPath,
  eligibleDeploymentIds,
  menuPrefix,
  variant = "header",
  showAccountLabels = false,
}: AgentDeploymentMenuProps) {
  const avatarBust = useDeploymentAvatarBust(deployment.id);
  const avatarUrl =
    avatarBust ?? deployment.avatar_url ?? getDeploymentAvatarUrl(deployment.id);
  const displayName = deployment.display_name || deployment.name;

  const { data: summaryData } = useDeploymentsSummary();
  const accounts = (summaryData?.accounts ?? [])
    .map((acct) => ({
      ...acct,
      deployments: acct.deployments.filter((dep) => {
        if (dep.id === deployment.id) return false;
        if (eligibleDeploymentIds && !eligibleDeploymentIds.has(dep.id)) {
          return false;
        }
        return true;
      }),
    }))
    .filter((acct) => acct.deployments.length > 0);

  const hasSwitchList = accounts.length > 0;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        {variant === "detail" ? (
          <button
            type="button"
            aria-label="Agent menu"
            className="flex cursor-pointer items-center gap-3 rounded-[8px] bg-background p-1 pl-1 pr-2.5 outline-none transition-colors hover:bg-background/90 focus-visible:ring-2 focus-visible:ring-ring/50 dark:-ml-2 dark:-mt-1.5 dark:rounded-md dark:bg-transparent dark:p-1.5 dark:pl-2 dark:pr-3 dark:hover:bg-white/5"
          >
            <BlueprintIdentity
              account={account}
              name={deployment.name}
              size={32}
              url={avatarUrl}
              className="rounded-sm"
            />
            <span
              className="max-w-[10rem] overflow-hidden whitespace-nowrap text-base font-medium tracking-wide text-foreground [--fade-start:8rem] [--fade-end:10rem] @max-[500px]:hidden min-[1100px]:max-w-[18rem] min-[1100px]:[--fade-start:16rem] min-[1100px]:[--fade-end:18rem]"
              style={{
                maskImage:
                  "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
                WebkitMaskImage:
                  "linear-gradient(to right, black var(--fade-start), transparent var(--fade-end))",
              }}
            >
              {displayName}
            </span>
            <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
          </button>
        ) : (
          <button
            type="button"
            aria-label="Select agent"
            className={cn(
              "flex h-8 w-full cursor-pointer items-center justify-between px-2.5 text-sm leading-none text-foreground transition-colors !bg-white dark:!bg-transparent hover:!bg-slate-50 dark:hover:!bg-slate-800",
              inputBase,
              inputFocusVisible,
            )}
          >
            <span className="flex min-w-0 items-center gap-2">
              <BlueprintIdentity
                account={account}
                name={deployment.name}
                size={18}
                url={avatarUrl}
                className="size-[18px] shrink-0 rounded-sm"
              />
              <span className="truncate">{displayName}</span>
            </span>
            <ChevronDown className="size-4 shrink-0 opacity-50" />
          </button>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="flex w-[260px] flex-col">
        {menuPrefix}
        {menuPrefix && hasSwitchList && <DropdownMenuSeparator />}
        <div className="max-h-[300px] overflow-y-auto">
          {accounts.map((acct) => (
            <DropdownMenuGroup key={acct.id}>
              {(showAccountLabels || accounts.length > 1) && (
                <DropdownMenuLabel className="text-faint-foreground">
                  {acct.display_name || acct.name}
                </DropdownMenuLabel>
              )}
              {acct.deployments.map((dep) => (
                <DropdownMenuItem key={dep.id} asChild>
                  <Link to={getDeploymentPath(acct.name, dep)}>
                    <BlueprintIdentity
                      account={acct.name}
                      name={dep.name}
                      size={20}
                      url={dep.avatar_url}
                      className="size-5 shrink-0 rounded-sm"
                    />
                    <span className="truncate">
                      {dep.display_name || dep.name}
                    </span>
                  </Link>
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          ))}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
