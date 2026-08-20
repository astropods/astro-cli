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

type PillSize = "sm" | "md" | "lg";
export type PillToggleSize = PillSize;

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
   *  that sit inside a panel header.
   *  `lg` = `sm` type on a 32px track, for a page header where the pill sits in
   *  a row of form controls and has to match their height. */
  size?: PillSize;
  /** Non-interactive readout rendered inside the track, ahead of the options.
   *  Use it when a value the options resolve to should read as part of the same
   *  control rather than as loose text beside it. */
  leading?: ReactNode;
  className?: string;
}

const PILL_SIZE: Record<
  PillSize,
  { container: string; item: string; indicator: string; leading: string }
> = {
  sm: {
    container: "p-0.5 rounded-sm",
    item: "rounded-sm px-2.5 py-0.5 text-mono-xs font-medium",
    indicator: "rounded-sm",
    leading: "px-2.5 text-mono-xs font-medium",
  },
  md: {
    container: "p-[2px] rounded-[7px]",
    item: "rounded-[5px] px-3 py-1 text-body-sm",
    indicator: "rounded-[5px]",
    leading: "px-3 text-body-sm",
  },
  // Height comes from the track, so the options stretch to fill it and the
  // active indicator covers the full 28px rather than floating in the middle.
  lg: {
    container: "h-8 items-stretch p-0.5 rounded-[8px]",
    item: "rounded-[5px] px-2.5 text-mono-xs font-medium",
    indicator: "rounded-[5px]",
    leading: "px-2.5 text-mono-xs font-medium",
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
        "items-center border border-border bg-muted/50 dark:bg-surface/60",
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
  leading,
  className,
}: PillToggleProps<K>) {
  const s = PILL_SIZE[size];
  // A readout and the options share one track with no rule between them, so
  // color carries the split: the readout takes the darkest step and the options
  // drop one below their usual weight. Without a readout there is nothing to
  // rank against, and the options keep the standard contrast.
  const hasReadout = leading !== undefined && leading !== null;
  return (
    <PillToggleChrome size={size} className={className}>
      {hasReadout && (
        <span
          className={cn(
            "inline-flex items-center whitespace-nowrap text-foreground",
            s.leading,
          )}
        >
          {leading}
        </span>
      )}
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
                ? cn("font-semibold", hasReadout ? "text-muted-foreground" : "text-foreground")
                : cn(
                    "font-medium",
                    hasReadout
                      ? "text-faint-foreground hover:text-muted-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  ),
            )}
          >
            {isActive && (
              <motion.div
                layoutId={layoutId}
                className={cn("absolute inset-0 bg-card shadow-none dark:bg-accent", s.indicator)}
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
