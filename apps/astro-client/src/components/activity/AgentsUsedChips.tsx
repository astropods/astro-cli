import { useId } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { deploymentPath, DeploymentTab, accountProfilePath } from "@/lib/routes";
import { OverflowPopover } from "./OverflowPopover";
import { type AgentDeploymentRef } from "./AgentNameLink";

export interface AgentRef {
  /** Unique per-deployment identifier — two deployments of the same blueprint
   *  produce two refs with identical name/account but distinct deployment_id.
   *  Drives the per-deployment click target and dedup key. */
  deployment_id: string;
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
   *  Used to enrich the per-chip tooltip with display_name and namespace so
   *  identical-avatar chips (same blueprint, different deployments) are
   *  distinguishable on hover. Chips for deployments not present in this
   *  map (cross-account public-blueprint deploys, missing data) fall back
   *  to the blueprint detail route and show the agent name in the tooltip. */
  deploymentsByAgent?: Map<string, AgentDeploymentRef[]>;
  className?: string;
}

/** Looks up the per-deployment metadata so a chip can show "display_name
 *  (namespace)" in its tooltip — the only visual differentiator between
 *  two chips of the same blueprint. Returns null when the deployment
 *  isn't in the active-account index (cross-account public deploy, etc). */
function findDeployment(
  index: Map<string, AgentDeploymentRef[]> | undefined,
  agentName: string,
  deploymentID: string,
): AgentDeploymentRef | null {
  const deps = index?.get(agentName) ?? [];
  return deps.find((d) => d.id === deploymentID) ?? null;
}

/** Tooltip body for one chip. Three cases:
 *   - dep found → display_name (or agent name when display_name is empty)
 *   - dep missing AND index loaded → "agentName (deleted)" (tombstoned)
 *   - dep missing AND index absent → just "agentName"
 *  The third case matters during initial render / loading: without the
 *  guard, every chip flashes "(deleted)" until deploymentsByAgent
 *  resolves. indexAvailable is the explicit "we actually checked"
 *  signal — derived once at the call site so callers don't have to
 *  re-reason about it per chip.
 *
 *  Namespace is deliberately NOT appended — it's a K8s-generated handle
 *  ("astro-w4ahbnqxd-0") that means nothing to end users. Two
 *  same-blueprint chips differ on their display_name in practice; when
 *  they share a display_name too, the click target still differentiates. */
function chipTooltip(
  agentName: string,
  dep: AgentDeploymentRef | null,
  indexAvailable: boolean,
): string {
  if (dep) return dep.display_name || dep.name;
  return indexAvailable ? `${agentName} (deleted)` : agentName;
}

function AgentUsedAvatar({
  account,
  name,
  isDeleted,
}: {
  account: string;
  name: string;
  isDeleted: boolean;
}) {
  return (
    <BlueprintIdentity
      account={account}
      name={name}
      size={24}
      className={cn(
        "size-6 shrink-0 rounded-[3px] border-[0.5px] border-slate-100 dark:border-white/20",
        isDeleted && "opacity-60",
      )}
    />
  );
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
  // indexAvailable distinguishes "we have the deployments index and the
  // deployment isn't in it" (genuinely tombstoned → muted + "(deleted)")
  // from "the index isn't loaded yet / was never passed" (treat as alive
  // so chips don't flash deleted on initial render).
  const indexAvailable = deploymentsByAgent !== undefined;
  // Screen-reader summary uses display_name/namespace where available so
  // duplicate-blueprint chips read distinguishably out loud, falling back
  // to the agent name when the deployment isn't resolvable. The
  // "(deleted)" annotation stays out of the sr-only label — it's a
  // visual cue for the tooltip, not relevant for the assistive readout.
  const summary = agents
    .map((a) => {
      const dep = findDeployment(deploymentsByAgent, a.name, a.deployment_id);
      return dep ? (dep.display_name || dep.name) : a.name;
    })
    .join(", ");
  return (
    <div className={cn("inline-flex items-center gap-1", className)} aria-labelledby={titleId}>
      <span id={titleId} className="sr-only">{summary}</span>
      <TooltipProvider delayDuration={200}>
        {visible.map(({ deployment_id, name, account }) => {
          const dep = findDeployment(deploymentsByAgent, name, deployment_id);
          // A chip counts as deleted only when the index has loaded AND
          // the deployment isn't in it. Pre-load (index undefined) chips
          // render alive with a fallback route + plain tooltip so users
          // don't see a flash of "(deleted)" before deploymentsByAgent
          // resolves.
          const isDeleted = indexAvailable && dep === null;
          const linkTo = dep
            ? deploymentPath(account, deployment_id, DeploymentTab.Monitor)
            : `${accountProfilePath(account)}/${name}`;
          return (
            <Tooltip key={deployment_id}>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Link
                    to={linkTo}
                    className="inline-flex rounded-[3px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    <AgentUsedAvatar account={account} name={name} isDeleted={isDeleted} />
                  </Link>
                </span>
              </TooltipTrigger>
              <TooltipContent side="top">{chipTooltip(name, dep, indexAvailable)}</TooltipContent>
            </Tooltip>
          );
        })}
      </TooltipProvider>
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={agents.length}
          itemNoun={{ singular: "deployment", plural: "deployments" }}
        >
          <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto">
            {agents.map(({ deployment_id, name, account }) => {
              const dep = findDeployment(deploymentsByAgent, name, deployment_id);
              const isDeleted = indexAvailable && dep === null;
              const linkTo = dep
                ? deploymentPath(account, deployment_id, DeploymentTab.Monitor)
                : `${accountProfilePath(account)}/${name}`;
              const primary = dep?.display_name || dep?.name || name;
              return (
                <li key={deployment_id}>
                  <Link
                    to={linkTo}
                    className={cn(
                      "flex items-center gap-2 rounded px-2 py-1 text-body-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                      isDeleted ? "text-muted-foreground" : "text-foreground",
                    )}
                  >
                    <AgentUsedAvatar account={account} name={name} isDeleted={isDeleted} />
                    <span className="min-w-0 flex-1 truncate">
                      {primary}
                      {isDeleted && (
                        <span className="ml-1.5 text-faint-foreground">(deleted)</span>
                      )}
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        </OverflowPopover>
      )}
    </div>
  );
}
