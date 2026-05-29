import type { ReactNode } from "react";
import { Link } from "react-router";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

/** Minimal slice of an AgentDeployment that the agent-name picker needs. */
export interface AgentDeploymentRef {
  id: string;
  name: string;
  display_name?: string;
  namespace?: string;
}

// Agents roll up across deployments in the blueprints summary (one row per
// agent_name regardless of how many regions / namespaces it's running in).
// Clicking the name needs to land the user on a real deployment's Monitor
// tab, but with multi-region we don't have a single "correct" target — so
// 2+ deployments opens a dropdown so the user picks one. 1 deployment
// skips the picker and links straight through; 0 deployments falls back
// to the blueprint detail page (still a useful destination).
interface AgentNameLinkProps {
  account: string;
  agentName: string;
  deployments: AgentDeploymentRef[];
  children: ReactNode;
  /** Style override for the link / button trigger. Useful when the trigger
   *  is a tight avatar chip vs a full table-cell name. */
  className?: string;
}

const DEFAULT_TRIGGER_CLASS =
  "inline-flex items-center hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded";

export function AgentNameLink({
  account,
  agentName,
  deployments,
  children,
  className,
}: AgentNameLinkProps) {
  const triggerClass = className ?? DEFAULT_TRIGGER_CLASS;

  if (deployments.length === 0) {
    return (
      <Link to={`/${account}/${agentName}`} className={triggerClass}>
        {children}
      </Link>
    );
  }
  if (deployments.length === 1) {
    return (
      <Link to={`/${account}/agents/${deployments[0].id}/monitor`} className={triggerClass}>
        {children}
      </Link>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={triggerClass}
          aria-label={`Choose a deployment of ${agentName}`}
        >
          {children}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="right" align="start" sideOffset={8} className="w-64">
        <DropdownMenuLabel className="text-mono-sm text-faint-foreground">
          {deployments.length} deployments
        </DropdownMenuLabel>
        {deployments.map((d) => (
          <DropdownMenuItem key={d.id} asChild>
            <Link to={`/${account}/agents/${d.id}/monitor`} className="flex flex-col items-start gap-0.5">
              <span className="text-body-sm text-foreground">
                {d.display_name || d.name}
              </span>
              {d.namespace && (
                <span className="font-mono text-mono-sm text-faint-foreground">
                  {d.namespace}
                </span>
              )}
            </Link>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
