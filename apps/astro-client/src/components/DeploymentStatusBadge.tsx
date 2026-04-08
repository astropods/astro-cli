import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { DeployedAgentStatus } from "./DeployedAgentCard";
import { deploymentStatusLabel } from "@/lib/deployment-utils";

const spinningStatuses = new Set<DeployedAgentStatus>([
  "deploying", "restarting", "pausing", "resuming", "undeploying",
]);

const badgeVariants: Record<DeployedAgentStatus, string> = {
  active:      "bg-teal-600/8 border-teal-600/22 text-teal-600 dark:text-teal-400",
  resuming:    "bg-teal-600/8 border-teal-600/22 text-teal-600 dark:text-teal-400",
  inactive:    "bg-stone-500/10 border-stone-400/50 text-stone-500 dark:border-stone-600 dark:text-stone-400",
  undeploying: "bg-stone-500/10 border-stone-400/50 text-stone-500 dark:border-stone-600 dark:text-stone-400",
  deploying:   "bg-yellow-500/10 border-yellow-500/30 text-yellow-600 dark:text-yellow-400",
  restarting:  "bg-yellow-500/10 border-yellow-500/30 text-yellow-600 dark:text-yellow-400",
  error:       "bg-coral-600/10 border-coral-600/25 text-coral-600 dark:text-coral-400",
  pausing:     "bg-coral-600/10 border-coral-600/25 text-coral-600 dark:text-coral-400",
};

export function DeploymentStatusBadge({
  status,
  className,
}: {
  status: DeployedAgentStatus;
  className?: string;
}) {
  const spinning = spinningStatuses.has(status);
  return (
    <span
      className={cn(
        "inline-flex items-center gap-[5px] rounded-full border px-2.5 py-0.5",
        "font-mono text-[11px] tracking-[0.06em]",
        badgeVariants[status],
        className,
      )}
    >
      {spinning ? (
        <Loader2 className="size-3 shrink-0 animate-spin" />
      ) : (
        <span className="size-[5px] shrink-0 rounded-full bg-current" />
      )}
      {deploymentStatusLabel[status]}
    </span>
  );
}
