import { cn } from '@/lib/utils';

export type TagColor = 'default' | 'teal' | 'blue' | 'yellow' | 'coral' | 'accent';

const COLOR: Record<TagColor, { bg: string; fgClass: string }> = {
  default: { bg: 'color-mix(in oklch, var(--color-slate-500) 12%, transparent)',                              fgClass: 'text-muted-foreground'                          },
  teal:    { bg: 'color-mix(in oklch, var(--success) 12%, transparent)',                                      fgClass: 'text-success'                                   },
  blue:    { bg: 'color-mix(in oklch, var(--color-indigo-500) 14%, transparent)',                             fgClass: 'text-indigo-600 dark:text-indigo-400'           },
  yellow:  { bg: 'color-mix(in oklch, var(--color-yellow-500) 12%, transparent)',                             fgClass: 'text-yellow-700 dark:text-yellow-400'           },
  coral:   { bg: 'color-mix(in oklch, var(--color-coral-600) 10%, transparent)',                              fgClass: 'text-coral-600 dark:text-coral-400'             },
  accent:  { bg: 'color-mix(in oklch, var(--card-accent, var(--color-slate-500)) 14%, transparent)',          fgClass: 'text-[var(--card-muted,var(--muted-foreground))]' },
};

export interface TagProps {
  color?: TagColor;
  children: React.ReactNode;
  className?: string;
}

export function Tag({ color = 'default', children, className }: TagProps) {
  const s = COLOR[color];
  return (
    <span
      className={cn('inline-flex items-center justify-center rounded font-mono text-[11px] leading-none px-2 py-1 normal-case font-normal', s.fgClass, className)}
      style={{ background: s.bg }}
    >
      {children}
    </span>
  );
}
