import { AlertTriangle } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

/** Inline warning shown when a Slack-typed grant targets a user who hasn't
 *  linked any Slack workspace. Used in both the grants list and the member
 *  picker so the affordance is consistent across the editor. */
export function SlackUnlinkedBadge() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label="Hasn't connected Slack"
          className="shrink-0 inline-flex items-center justify-center text-destructive"
        >
          <AlertTriangle className="h-3.5 w-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-[240px] text-[11px]">
        Hasn't connected Slack. They won't be able to invoke this agent until they link their account.
      </TooltipContent>
    </Tooltip>
  );
}
