import { useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";
import { useDeploymentStatus, useStopDeployment, useWakeUpDeployment } from "@/api/queries/deployments";
import { useBillingStatus } from "@/api/queries/billing";
import { isBillingSuspendedStatus, isPausedState } from "@/lib/deployment-utils";
import { billingStoppedHint } from "@/lib/billing-copy";
import { getApiErrorMessage } from "@/lib/api";
import { toast } from "sonner";
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
  // Reads as off and cannot be toggled back on: a wakeup would be re-suspended
  // on the next recompute.
  const suspended = isBillingSuspendedStatus(statusData);
  // Same query key as the app-shell banner, so this is a cache read.
  const { data: billing } = useBillingStatus(suspended ? account : "");
  const suspendedHint = billingStoppedHint(billing?.action);
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
  const checked = intent === "resuming" ? true : intent === "pausing" ? false : !paused && !suspended;
  // A failed deploy is neither active nor paused: its record status isn't
  // "stopped" (so `checked` reads true), but the live status is "error". Without
  // this the toggle would render a healthy green "Active" for a broken agent.
  // Suppressed mid-intent so an optimistic pause/resume isn't masked by a stale
  // error from the previous state.
  const errored = liveStatus === "error" && intent === null;

  // A refused mutation leaves the toggle mid-intent, so clear it and say why.
  // The message is the server's: getApiErrorMessage prefers `details`, and a
  // billing 402 puts the actionable sentence there. The app-shell banner already
  // links a gated account to billing, so there is nothing to route here.
  function onMutationError(err: unknown) {
    setIntent(null);
    toast.error(getApiErrorMessage(err, "Could not change the agent's state."));
  }

  function handleToggle(next: boolean) {
    if (busy) return;
    if (next) {
      setIntent("resuming");
      wakeupMutation.mutate({ deploymentId: deployment.id }, { onError: onMutationError });
    } else {
      setIntent("pausing");
      stopMutation.mutate({ deploymentId: deployment.id }, { onError: onMutationError });
    }
  }

  const label = transitioning
    ? (checked ? "Deploying" : "Pausing")
    : errored
      ? "Error"
      : suspended
        ? "Suspended"
        : checked ? "Active" : "Paused";

  const accentClass = transitioning && checked
    ? "text-yellow-700 dark:text-yellow-400"
    : errored
      ? "text-red-700 dark:text-red-400"
      : suspended
        ? "text-amber-700 dark:text-amber-400"
      : checked
        ? "text-green-700 dark:text-green-400"
        : "text-stone-600 dark:text-stone-400";

  return (
    <div data-testid="agent-status-toggle" className="flex items-center gap-2.5">
      <TooltipProvider delayDuration={0}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex cursor-default items-center gap-2">
              {transitioning ? (
                <Loader2 className={cn("size-3 shrink-0 animate-spin", accentClass)} />
              ) : (
                <span
                  className={cn(
                    "size-2 shrink-0 rounded-full",
                    errored
                      ? "bg-red-600 dark:bg-red-400"
                      : suspended
                        ? "bg-amber-600 dark:bg-amber-400"
                      : checked
                        ? "bg-green-600 shadow-[0_0_6px_2px] shadow-green-600/50 dark:bg-green-400 dark:shadow-green-400/50"
                        : "bg-stone-500 dark:bg-stone-400",
                  )}
                />
              )}
              <span className={cn("text-body font-medium tracking-wide", accentClass)}>
                {label}
              </span>
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {suspended ? suspendedHint : statusDetails || label}
          </TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex items-center">
              <Switch
                checked={checked}
                onCheckedChange={handleToggle}
                disabled={busy || suspended}
              />
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {suspended
              ? suspendedHint
              : transitioning
                ? (checked ? "Deploying agent…" : "Pausing agent…")
                : (checked ? "Pause this agent" : "Redeploy this agent")}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}
