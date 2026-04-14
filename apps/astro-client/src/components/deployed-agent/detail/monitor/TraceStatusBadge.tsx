import { StatusBadge, type StatusBadgeColor } from '@/components/StatusBadge';
import type { TraceStatus } from './MonitorTab';

const STATUS_MAP: Record<TraceStatus, { color: StatusBadgeColor; label: string }> = {
  success: { color: 'success', label: 'Success' },
  error:   { color: 'error',   label: 'Error'   },
  timeout: { color: 'warning', label: 'Timeout' },
};

export function TraceStatusBadge({ status }: { status: TraceStatus }) {
  const { color, label } = STATUS_MAP[status];
  return <StatusBadge color={color}>{label}</StatusBadge>;
}
