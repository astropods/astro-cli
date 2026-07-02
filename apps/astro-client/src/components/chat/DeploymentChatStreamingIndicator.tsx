import type { FC } from "react";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { cn } from "@/lib/utils";

export const DeploymentChatStreamingIndicator: FC<{
  deploymentId: string;
  agentLabel: string;
}> = ({ deploymentId, agentLabel }) => {
  return (
    <span
      className="relative mx-1.5 inline-flex size-6 shrink-0 align-middle"
      role="status"
      aria-label={`${agentLabel} is replying`}
    >
      <span
        className="pointer-events-none absolute inset-0 rounded-sm bg-primary/20 dark:bg-primary/50 animate-ping"
        aria-hidden
      />
      <DeploymentAvatar
        deployment={{ id: deploymentId, name: agentLabel }}
        size={24}
        className={cn(
          "relative size-6 rounded-sm",
          "[animation:astro-logo-loader-body-pulse_1.5s_ease-in-out_infinite]",
        )}
      />
    </span>
  );
};
