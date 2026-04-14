import { cn } from '@/lib/utils';

export type TagColor = 'default' | 'teal' | 'blue' | 'yellow' | 'coral';

const COLOR: Record<TagColor, { bg: string; fg: string }> = {
  default: { bg: 'color-mix(in oklch, var(--color-stone-500) 10%, transparent)',  fg: 'var(--muted-foreground)'  },
  teal:    { bg: 'color-mix(in oklch, var(--color-teal-600) 10%, transparent)',   fg: 'var(--color-teal-700)'    },
  blue:    { bg: 'color-mix(in oklch, var(--color-blue-600) 10%, transparent)',   fg: 'var(--color-blue-600)'    },
  yellow:  { bg: 'color-mix(in oklch, var(--color-yellow-500) 10%, transparent)', fg: 'var(--color-yellow-700)'  },
  coral:   { bg: 'color-mix(in oklch, var(--color-coral-600) 10%, transparent)',  fg: 'var(--color-coral-600)'   },
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
      className={cn('inline-flex items-center justify-center rounded font-mono text-[11px] leading-none px-2 py-1 normal-case font-normal', className)}
      style={{ background: s.bg, color: s.fg }}
    >
      {children}
    </span>
  );
}
