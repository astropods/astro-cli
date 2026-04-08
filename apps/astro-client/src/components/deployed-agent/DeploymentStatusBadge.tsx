import { Loader2 } from 'lucide-react';
import type { DeployedAgentStatus } from '@/components/DeployedAgentCard';

const BADGE: Record<DeployedAgentStatus, { bg: string; bdr: string; dot: string; label: string; spinning: boolean }> = {
  restarting:  { bg: 'color-mix(in oklch, var(--color-yellow-500) 12%, transparent)', bdr: 'color-mix(in oklch, var(--color-yellow-500) 30%, transparent)', dot: 'var(--color-yellow-600)', label: 'Restarting',  spinning: true  },
  pausing:     { bg: 'color-mix(in oklch, var(--color-coral-600) 12%, transparent)',  bdr: 'color-mix(in oklch, var(--color-coral-600) 28%, transparent)',  dot: 'var(--color-coral-600)',  label: 'Pausing',     spinning: true  },
  resuming:    { bg: 'rgba(21,130,125,0.08)', bdr: 'rgba(21,130,125,0.22)', dot: 'var(--color-teal-600)', label: 'Resuming',    spinning: true  },
  error:       { bg: 'color-mix(in oklch, var(--color-coral-600) 12%, transparent)',  bdr: 'color-mix(in oklch, var(--color-coral-600) 28%, transparent)',  dot: 'var(--color-coral-600)',  label: 'Error',       spinning: false },
  undeploying: { bg: 'var(--muted)', bdr: 'var(--border)', dot: 'var(--faint-foreground)', label: 'Undeploying', spinning: true  },
  deploying:   { bg: 'color-mix(in oklch, var(--color-yellow-500) 12%, transparent)', bdr: 'color-mix(in oklch, var(--color-yellow-500) 28%, transparent)', dot: 'var(--color-yellow-500)', label: 'Deploying',   spinning: true  },
  inactive:    { bg: 'var(--muted)', bdr: 'var(--border)', dot: 'var(--faint-foreground)', label: 'Inactive',    spinning: false },
  active:      { bg: 'rgba(21,130,125,0.08)', bdr: 'rgba(21,130,125,0.22)', dot: 'var(--color-teal-600)', label: 'Active',      spinning: false },
};

export function DeploymentStatusBadge({ status }: { status: DeployedAgentStatus }) {
  const badge = BADGE[status];
  return (
    <span
      className="inline-flex items-center gap-[5px] px-[10px] py-[2px] rounded-full font-mono text-label tracking-[0.06em] border"
      style={{ background: badge.bg, borderColor: badge.bdr, color: badge.dot }}
    >
      {badge.spinning
        ? <Loader2 size={12} style={{ color: badge.dot }} className="dp-spin" />
        : <span className="w-[5px] h-[5px] rounded-full inline-block" style={{ background: badge.dot }} />}
      {badge.label}
    </span>
  );
}
