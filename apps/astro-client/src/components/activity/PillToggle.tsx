import { type ReactNode } from "react";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";

export interface PillOption<K extends string> {
  key: K;
  label: string;
  ariaLabel?: string;
  /** Rendered before the label — e.g. a Lucide icon. */
  icon?: ReactNode;
  /** Rendered after the label as a faint trailing count. */
  count?: number;
}

type PillSize = "sm" | "md";

interface PillToggleChromeProps {
  children: ReactNode;
  size?: PillSize;
  inline?: boolean;
  className?: string;
}

interface PillToggleProps<K extends string> {
  value: K;
  options: PillOption<K>[];
  onChange: (next: K) => void;
  /** Unique `layoutId` for the sliding indicator. Two pill toggles on the
   *  same page MUST pass different ids or their indicators will fight. */
  layoutId: string;
  /** `sm` (default) = compact mono caps used for the date-range pill.
   *  `md` = roomier with body text + tighter radius, used for view toggles
   *  that sit inside a panel header. */
  size?: PillSize;
  className?: string;
}

const PILL_SIZE: Record<PillSize, { container: string; item: string; indicator: string }> = {
  sm: {
    container: "p-0.5 rounded-sm",
    item: "rounded-sm px-2.5 py-0.5 text-mono-xs font-medium",
    indicator: "rounded-sm",
  },
  md: {
    container: "p-[2px] rounded-[7px]",
    item: "rounded-[5px] px-3 py-1 text-body-sm",
    indicator: "rounded-[5px]",
  },
};

export function PillToggleChrome({
  children,
  size = "sm",
  inline = false,
  className,
}: PillToggleChromeProps) {
  const s = PILL_SIZE[size];
  return (
    <div
      className={cn(
        inline ? "inline-flex" : "flex",
        "items-center border border-border bg-muted dark:bg-surface/60",
        s.container,
        className,
      )}
    >
      {children}
    </div>
  );
}

// Shared chrome for Insights' header pill toggles (date range + view).
// Spring timing matches TimeRangeSelector's original feel — bounce 0.15,
// duration 0.4 — so every pill on the page shares the same motion.
export function PillToggle<K extends string>({
  value,
  options,
  onChange,
  layoutId,
  size = "sm",
  className,
}: PillToggleProps<K>) {
  const s = PILL_SIZE[size];
  return (
    <PillToggleChrome size={size} className={className}>
      {options.map(({ key, label, ariaLabel, icon, count }) => {
        const isActive = key === value;
        return (
          <button
            key={key}
            type="button"
            onClick={() => onChange(key)}
            aria-label={ariaLabel ?? label}
            aria-pressed={isActive}
            className={cn(
              "relative inline-flex items-center gap-1.5 transition-colors",
              s.item,
              isActive
                ? "font-semibold text-foreground"
                : "font-medium text-muted-foreground hover:text-foreground",
            )}
          >
            {isActive && (
              <motion.div
                layoutId={layoutId}
                className={cn("absolute inset-0 bg-card shadow-sm dark:bg-white/10 dark:shadow-none", s.indicator)}
                transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
              />
            )}
            <span className="relative inline-flex items-center gap-1.5">
              {icon}
              {label}
              {count !== undefined && (
                <span className="text-faint-foreground tabular-nums">{count}</span>
              )}
            </span>
          </button>
        );
      })}
    </PillToggleChrome>
  );
}
