import { StatusBadge, type StatusBadgeColor } from '@/components/StatusBadge';
import type { DeployedAgentStatus } from '@/components/DeployedAgentCard';
import { deploymentStatusLabel } from '@/lib/deployment-utils';

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

export function DeploymentStatusBadge({ status }: { status: DeployedAgentStatus }) {
  const { color, spinning } = STATUS_MAP[status];
  return (
    <StatusBadge color={color} indicator={spinning} spinning={spinning}>
      {deploymentStatusLabel[status]}
    </StatusBadge>
  );
}
