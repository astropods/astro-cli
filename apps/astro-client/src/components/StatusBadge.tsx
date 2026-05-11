import { Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

export type StatusBadgeColor = 'success' | 'warning' | 'error' | 'muted';

const COLOR: Record<StatusBadgeColor, { bg: string; bdr: string; fg: string }> = {
  success: { bg: 'color-mix(in oklch, var(--success) 12%, transparent)', bdr: 'color-mix(in oklch, var(--success) 28%, transparent)', fg: 'var(--success)' },
  warning: { bg: 'color-mix(in oklch, var(--warning) 12%, transparent)', bdr: 'color-mix(in oklch, var(--warning) 28%, transparent)', fg: 'var(--warning)' },
  error:   { bg: 'color-mix(in oklch, var(--error) 12%, transparent)',   bdr: 'color-mix(in oklch, var(--error) 28%, transparent)',   fg: 'var(--error)'   },
  muted:   { bg: 'var(--muted)',                                                   bdr: 'var(--border)',                                                  fg: 'var(--muted-foreground)' },
};

export interface StatusBadgeProps {
  color: StatusBadgeColor;
  indicator?: boolean;
  spinning?: boolean;
  outline?: boolean;
  className?: string;
  children: React.ReactNode;
}

export function StatusBadge({ color, indicator = false, spinning = false, outline = false, className, children }: StatusBadgeProps) {
  const s = COLOR[color];
  return (
    <span
      className={cn("inline-flex items-center gap-[5px] px-[10px] py-1 rounded-full font-mono font-normal text-label tracking-[0.06em] border", className)}
      style={{ background: outline ? 'transparent' : s.bg, borderColor: s.bdr, color: s.fg }}
    >
      {indicator && (
        spinning
          ? <Loader2 size={12} className="dp-spin shrink-0" />
          : <span className="size-[5px] rounded-full shrink-0 bg-current" />
      )}
      {children}
    </span>
  );
}
