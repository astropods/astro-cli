import { Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

export type StatusBadgeColor =
  | 'success'
  | 'warning'
  | 'error'
  | 'destructive'
  | 'muted'
  | 'primary';
export type StatusBadgeSize = 'sm' | 'md';

const COLOR: Record<StatusBadgeColor, { bg: string; bdr: string; fg: string }> = {
  success: { bg: 'color-mix(in oklch, var(--success) 12%, transparent)', bdr: 'color-mix(in oklch, var(--success) 28%, transparent)', fg: 'var(--success)' },
  warning: { bg: 'color-mix(in oklch, var(--warning) 12%, transparent)', bdr: 'color-mix(in oklch, var(--warning) 28%, transparent)', fg: 'var(--warning)' },
  error:   { bg: 'color-mix(in oklch, var(--error) 12%, transparent)',   bdr: 'color-mix(in oklch, var(--error) 28%, transparent)',   fg: 'var(--error)'   },
  destructive: {
    bg: 'color-mix(in oklch, var(--destructive) 12%, transparent)',
    bdr: 'color-mix(in oklch, var(--destructive) 28%, transparent)',
    fg: 'var(--destructive)',
  },
  muted:   { bg: 'var(--muted)',                                                   bdr: 'var(--border)',                                                  fg: 'var(--muted-foreground)' },
  // Primary uses CSS variables with fallbacks so consumers (or dark-mode
  // utility classes on the badge itself) can brighten the text/bg without
  // touching the global --primary token.
  primary: {
    bg: 'var(--sb-primary-bg, color-mix(in oklch, var(--primary) 12%, transparent))',
    bdr: 'var(--sb-primary-bdr, color-mix(in oklch, var(--primary) 28%, transparent))',
    fg: 'var(--sb-primary-fg, var(--primary))',
  },
};

// Dark-mode override for the primary variant only: indigo-600/700 is hard to
// read on the dark card surface, so swap text + tint to lighter indigos.
const PRIMARY_DARK_OVERRIDES =
  "dark:[--sb-primary-bg:color-mix(in_oklch,var(--color-indigo-400)_22%,transparent)] dark:[--sb-primary-bdr:color-mix(in_oklch,var(--color-indigo-400)_36%,transparent)] dark:[--sb-primary-fg:var(--color-indigo-300)]";

const SIZE: Record<StatusBadgeSize, string> = {
  sm: 'gap-1 px-2 py-0.5',
  md: 'gap-[5px] px-[10px] py-1',
};

export interface StatusBadgeProps {
  color: StatusBadgeColor;
  size?: StatusBadgeSize;
  indicator?: boolean;
  spinning?: boolean;
  outline?: boolean;
  className?: string;
  children: React.ReactNode;
}

export function StatusBadge({ color, size = 'md', indicator = false, spinning = false, outline = false, className, children }: StatusBadgeProps) {
  const s = COLOR[color];
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border font-mono font-normal text-label tracking-[0.06em]",
        SIZE[size],
        color === 'primary' && PRIMARY_DARK_OVERRIDES,
        className,
      )}
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
