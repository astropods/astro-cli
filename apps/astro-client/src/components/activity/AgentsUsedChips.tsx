import { useId } from "react";
import { cn } from "@/lib/utils";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { OverflowPopover } from "./OverflowPopover";
import { AgentNameLink, type AgentDeploymentRef } from "./AgentNameLink";

export interface AgentRef {
  name: string;
  /** Publishing account — used to resolve the blueprint avatar URL and the
   *  deep-link route. Differs from the deploying org for public-blueprint
   *  deploys (SourceAccountID set server-side). */
  account: string;
}

interface AgentsUsedChipsProps {
  agents: AgentRef[];
  /** How many avatars to render before collapsing to a +N overflow indicator. */
  maxVisible?: number;
  /** Map of agent_name → all matching deployments (in the active account).
   *  When provided, each chip uses the shared `AgentNameLink` picker so
   *  clicks route to a deployment's Monitor tab. Falls back to the
   *  blueprint detail page when an agent has no matching deployment
   *  (cross-account public-blueprint deploys, missing data, etc). */
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
  className?: string;
}

export function AgentsUsedChips({
  agents,
  maxVisible = 3,
  deploymentsByAgent,
  className,
}: AgentsUsedChipsProps) {
  const titleId = useId();
  if (agents.length === 0) {
    return <span className="text-faint-foreground">—</span>;
  }
  const visible = agents.slice(0, maxVisible);
  const overflow = agents.length - visible.length;
  const summary = agents.map((a) => a.name).join(", ");
  return (
    <div className={cn("inline-flex items-center gap-1", className)} aria-labelledby={titleId}>
      <span id={titleId} className="sr-only">{summary}</span>
      <TooltipProvider delayDuration={200}>
        {visible.map(({ name, account }) => {
          const deps = deploymentsByAgent?.get(name) ?? [];
          const avatarNode = (
            <BlueprintIdentity
              account={account}
              name={name}
              size={20}
              className="size-5 rounded-full"
            />
          );
          // Every chip gets a hover tooltip with the agent name. The chip
          // itself is just a tiny avatar — without the tooltip the user
          // can't tell what they're hovering before clicking.
          return (
            <Tooltip key={`${account}/${name}`}>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <AgentNameLink
                    account={account}
                    agentName={name}
                    deployments={deps}
                    className="inline-flex rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    {avatarNode}
                  </AgentNameLink>
                </span>
              </TooltipTrigger>
              <TooltipContent side="top">{name}</TooltipContent>
            </Tooltip>
          );
        })}
      </TooltipProvider>
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={agents.length}
          itemNoun={{ singular: "agent", plural: "agents" }}
        >
          <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto">
            {agents.map(({ name, account }) => {
              const deps = deploymentsByAgent?.get(name) ?? [];
              return (
                <li key={`${account}/${name}`}>
                  <AgentNameLink
                    account={account}
                    agentName={name}
                    deployments={deps}
                    className="flex items-center gap-2 rounded px-2 py-1 text-body-sm text-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    <BlueprintIdentity
                      account={account}
                      name={name}
                      size={16}
                      className="size-4 shrink-0 rounded-full"
                    />
                    <span className="truncate">{name}</span>
                  </AgentNameLink>
                </li>
              );
            })}
          </ul>
        </OverflowPopover>
      )}
    </div>
  );
}
