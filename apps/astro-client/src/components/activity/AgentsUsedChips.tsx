import { useId } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

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
  className?: string;
}

export function AgentsUsedChips({ agents, maxVisible = 5, className }: AgentsUsedChipsProps) {
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
        {visible.map(({ name, account }) => (
          <Tooltip key={`${account}/${name}`}>
            <TooltipTrigger asChild>
              <Link to={`/${account}/${name}`} className="inline-flex rounded-full">
                <BlueprintIdentity
                  account={account}
                  name={name}
                  size={20}
                  className="size-5 rounded-full"
                />
              </Link>
            </TooltipTrigger>
            <TooltipContent side="top">{name}</TooltipContent>
          </Tooltip>
        ))}
      </TooltipProvider>
      {overflow > 0 && (
        <span className="font-mono text-mono-sm text-muted-foreground" aria-hidden title={summary}>
          +{overflow}
        </span>
      )}
    </div>
  );
}
