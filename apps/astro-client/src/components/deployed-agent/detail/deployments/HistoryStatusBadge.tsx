import { StatusBadge, type StatusBadgeColor } from '@/components/StatusBadge';
import { statusLabel } from './history/utils';
import type { DeployHistoryStatus } from './history/types';

const STATUS_COLOR: Record<DeployHistoryStatus, StatusBadgeColor> = {
  active:      'success',
  resuming:    'success',
  deploying:   'warning',
  restarting:  'warning',
  pausing:     'error',
  failed:      'error',
  undeploying: 'muted',
  inactive:    'muted',
  undeployed:  'muted',
};

export const HISTORY_STATUS_FG: Record<DeployHistoryStatus, string> = {
  active:      'var(--color-teal-600)',
  resuming:    'var(--color-teal-600)',
  deploying:   'var(--color-yellow-600)',
  restarting:  'var(--color-yellow-600)',
  pausing:     'var(--color-coral-600)',
  failed:      'var(--color-coral-600)',
  undeploying: 'var(--muted-foreground)',
  inactive:    'var(--muted-foreground)',
  undeployed:  'var(--muted-foreground)',
};

export function HistoryStatusBadge({ status }: { status: DeployHistoryStatus }) {
  return (
    <StatusBadge color={STATUS_COLOR[status]}>
      {statusLabel(status)}
    </StatusBadge>
  );
}
