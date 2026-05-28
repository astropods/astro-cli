import { StatusBadge, type StatusBadgeColor } from '@/components/StatusBadge';
import { deploymentStatusLabel, type DeployedAgentStatus } from '@/lib/deployment-utils';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';

interface DeploymentStatusBadgeProps {
  status: DeployedAgentStatus;
  /**
   * Server-supplied human-readable reason this deployment is in error/failed.
   * Rendered as a tooltip on the badge so operators see WHY without leaving
   * the dashboard. Ignored for non-error statuses.
   */
  errorMessage?: string;
}

export function DeploymentStatusBadge({ status, errorMessage }: DeploymentStatusBadgeProps) {
  const { color, spinning } = STATUS_MAP[status];
  const badge = (
    <StatusBadge color={color} indicator={spinning} spinning={spinning}>
      {deploymentStatusLabel[status]}
    </StatusBadge>
  );

  if (status !== 'error' || !errorMessage) {
    return badge;
  }

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex" tabIndex={0} aria-label={errorMessage}>
            {badge}
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={4} className="max-w-xs whitespace-pre-line break-words">
          {errorMessage}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

const STATUS_MAP: Record<DeployedAgentStatus, { color: StatusBadgeColor; spinning: boolean }> = {
  active:      { color: 'success', spinning: false },
  resuming:    { color: 'success', spinning: true  },
  deploying:   { color: 'warning', spinning: true  },
  restarting:  { color: 'warning', spinning: true  },
  pausing:     { color: 'error',   spinning: true  },
  error:       { color: 'error',   spinning: false },
  undeploying: { color: 'muted',   spinning: true  },
  inactive:    { color: 'muted',   spinning: false },
};
