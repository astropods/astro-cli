import { useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";
import { useDeploymentStatus, useStopDeployment, useWakeUpDeployment } from "@/api/queries/deployments";
import { isPausedState } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";

interface AgentStatusToggleProps {
  deployment: AgentDeployment;
  account: string;
}

export function AgentStatusToggle({ deployment, account }: AgentStatusToggleProps) {
  const stopMutation = useStopDeployment(account);
  const wakeupMutation = useWakeUpDeployment(account);
  // Server-derived status — the single source of truth for the badge label
  // and transitioning indicator. No record/runtime join here; the server
  // already did it.
  const { data: statusData } = useDeploymentStatus(deployment.id);
  const liveStatus = statusData?.value;
  const statusDetails = statusData?.details;

  const paused = isPausedState(deployment);
  const serverTransitioning = liveStatus === "deploying" || liveStatus === "undeploying";

  // Track local intent: "pausing" or "resuming" until server catches up
  const [intent, setIntent] = useState<"pausing" | "resuming" | null>(null);

  // Clear intent once server state reflects the completed action (render-time adjustment)
  if (
    (intent === "pausing" && paused && !serverTransitioning) ||
    (intent === "resuming" && !paused && !serverTransitioning)
  ) {
    setIntent(null);
  }

  const transitioning = serverTransitioning || intent !== null;
  const busy = stopMutation.isPending || wakeupMutation.isPending || transitioning;
  const checked = intent === "resuming" ? true : intent === "pausing" ? false : !paused;

  function handleToggle(next: boolean) {
    if (busy) return;
    if (next) {
      setIntent("resuming");
      wakeupMutation.mutate({ deploymentId: deployment.id });
    } else {
      setIntent("pausing");
      stopMutation.mutate({ deploymentId: deployment.id });
    }
  }

  const label = transitioning
    ? (checked ? "Deploying" : "Pausing")
    : checked ? "Active" : "Paused";

  const accentClass = transitioning && checked
    ? "text-yellow-700 dark:text-yellow-400"
    : checked
      ? "text-green-700 dark:text-green-400"
      : "text-stone-600 dark:text-stone-400";

  return (
    <div data-testid="agent-status-toggle" className="flex items-center gap-2.5">
      <span className="inline-flex items-center gap-2">
        {transitioning ? (
          <Loader2 className={cn("size-3 shrink-0 animate-spin", accentClass)} />
        ) : (
          <span
            className={cn(
              "size-2 shrink-0 rounded-full",
              checked
                ? "bg-green-600 shadow-[0_0_6px_2px] shadow-green-600/50 dark:bg-green-400 dark:shadow-green-400/50"
                : "bg-stone-500",
            )}
          />
        )}
        <span className={cn("text-body font-medium tracking-wide", accentClass)}>
          {label}
        </span>
      </span>
      <TooltipProvider delayDuration={0}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex items-center">
              <Switch
                checked={checked}
                onCheckedChange={handleToggle}
                disabled={busy}
              />
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {transitioning
              ? (checked ? "Deploying agent…" : "Pausing agent…")
              : statusDetails || (checked ? "Pause this agent" : "Redeploy this agent")}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}
