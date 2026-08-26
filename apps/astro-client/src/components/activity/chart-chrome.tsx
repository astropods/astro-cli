import { useCallback, useMemo, useState } from "react";
import { cn } from "@/lib/utils";

/** Shared chrome for the activity charts, so the axes, tooltip and legend
 *  cannot drift apart across the pages that render them side by side. */

export const AXIS_TICK = {
  fill: "var(--color-muted-foreground)",
  fontSize: 11,
  fontFamily: "var(--font-mono)",
} as const;

export const GRID_PROPS = {
  strokeDasharray: "3 3",
  vertical: false as const,
  stroke: "var(--color-border)",
  strokeOpacity: 0.5,
} as const;

export const X_AXIS_BASE = {
  dataKey: "date",
  tick: AXIS_TICK,
  axisLine: false,
  tickLine: false,
  tickMargin: 8,
  minTickGap: 40,
} as const;

export function yAxisProps(tickFormatter: (v: number) => string) {
  return {
    tickFormatter,
    tick: AXIS_TICK,
    axisLine: false,
    tickLine: false,
    tickMargin: 4,
    width: 56,
  } as const;
}

/** Tooltip for a stacked or multi-series chart.
 *
 *  Zero-valued series are dropped: every series carries a key on every row so
 *  segments hold their slot, which without this fills the tooltip with rows
 *  reading nothing. A single-series chart opts out with `includeZero`: there
 *  the zero is the reading. */
export function SeriesTooltip({
  active,
  payload,
  label,
  colors,
  names,
  format,
  includeZero = false,
}: {
  active?: boolean;
  payload?: { name: string; value: number }[];
  label?: string;
  colors: Record<string, string>;
  names?: Record<string, string>;
  format: (v: number) => string;
  includeZero?: boolean;
}) {
  const shown = (payload ?? []).filter((p) => includeZero || (p.value ?? 0) > 0);
  if (!active || shown.length === 0) return null;
  const total = shown.reduce((sum, p) => sum + (p.value ?? 0), 0);
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 shadow-lg backdrop-blur">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      {shown.map((p) => (
        <div key={p.name} className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: colors[p.name] }} />
          <span className="font-mono text-body-sm text-muted-foreground">{names?.[p.name] ?? p.name}</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{format(p.value)}</span>
        </div>
      ))}
      {shown.length > 1 && (
        <div className="mt-1 flex items-center gap-2 border-t border-border pt-1">
          <span className="font-mono text-body-sm text-muted-foreground">Total</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{format(total)}</span>
        </div>
      )}
    </div>
  );
}

/** Hide state for a clickable legend. Hiding every series is refused — an empty
 *  chart is indistinguishable from having no data. */
export function useHiddenSeries(keys: string[]) {
  const [hidden, setHidden] = useState<Set<string>>(new Set());
  const visible = useMemo(() => keys.filter((k) => !hidden.has(k)), [keys, hidden]);
  const toggle = useCallback((key: string) => {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      if (next.size >= keys.length) return prev;
      return next;
    });
  }, [keys.length]);
  return { hidden, visible, toggle };
}

/** Clickable legend. With `onSelect`, a plain click drills into the series and
 *  shift-click hides it; without, a click hides. */
export function SeriesLegend({
  keys,
  colors,
  names,
  hidden,
  onToggle,
  onSelect,
}: {
  keys: string[];
  colors: Record<string, string>;
  names?: Record<string, string>;
  hidden: Set<string>;
  onToggle: (key: string) => void;
  onSelect?: (key: string) => void;
}) {
  if (keys.length === 0) return null;
  return (
    <div className="mt-3 flex shrink-0 flex-wrap items-center justify-center gap-x-5 gap-y-1.5">
      {keys.map((key) => {
        const isHidden = hidden.has(key);
        return (
          <button
            key={key}
            type="button"
            onClick={(e) => (onSelect && !e.shiftKey ? onSelect(key) : onToggle(key))}
            title={onSelect ? "Click to drill into this category; shift-click to hide it" : undefined}
            aria-pressed={!isHidden}
            className={cn(
              "flex items-center gap-1.5 rounded px-1 text-mono-sm transition-colors",
              "focus-visible:bg-muted focus-visible:outline-none",
              isHidden ? "text-faint-foreground opacity-50" : "text-muted-foreground hover:text-foreground",
            )}
          >
            <span
              className={cn("size-2 rounded-full", isHidden && "opacity-40")}
              style={{ backgroundColor: colors[key] }}
            />
            <span className={cn(isHidden && "line-through")}>{names?.[key] ?? key}</span>
          </button>
        );
      })}
    </div>
  );
}
