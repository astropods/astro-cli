import { useMemo, useState } from "react";
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { formatCost, formatDateShort } from "@/lib/format-utils";
import { dayKeysForRange } from "@/lib/date-utils";
import type { ActiveSpendPoint } from "./use-insights-data";

interface ActiveUsersSpendChartProps {
  data: ActiveSpendPoint[];
  /** Length of the bounded period in days. When set, the X-axis spans every
   *  day in the range (zero-filled on inactive days) so the chart grows and
   *  shrinks with the date-range chip the same way the agent-spend chart does. */
  days?: number;
}


const AXIS_TICK = { fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" } as const;
const GRID_PROPS = { strokeDasharray: "3 3", vertical: false as const, stroke: "var(--color-border)", strokeOpacity: 0.5 } as const;

// Distinct line colors so the two series read as different metrics. Pulled
// from the theme palette rather than the agent/user color map (those churn
// with the entity list; these are stable per-chart). Yellow / red / green
// are reserved for status states across the app, so indigo for users.
//
// Users uses a lighter shade of indigo (400) instead of `--primary`
// (which is indigo-600/700). Spend uses `--muted-foreground` — softer
// than the body foreground so the indigo users line stays the focal
// point, but distinct enough from the indigo to read as a separate
// series in light mode.
const USERS_COLOR = "var(--color-primary-400)";
const SPEND_COLOR = "var(--color-muted-foreground)";

function CustomTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ name: string; value: number; dataKey: string }>;
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  // Only render the rows for series currently in the payload — when the
  // user hides a series via the clickable legend, Recharts drops it from
  // the payload and the tooltip should follow suit. (A hardcoded both-rows
  // tooltip would show the hidden series with a $0 fallback.)
  const usersEntry = payload.find((p) => p.dataKey === "users");
  const costEntry  = payload.find((p) => p.dataKey === "cost");
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 shadow-lg backdrop-blur">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      {usersEntry && (
        <div className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full bg-primary-400" />
          <span className="font-mono text-body-sm text-muted-foreground">By People</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{usersEntry.value}</span>
        </div>
      )}
      {costEntry && (
        <div className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full bg-muted-foreground" />
          <span className="font-mono text-body-sm text-muted-foreground">Total spend</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{formatCost(costEntry.value)}</span>
        </div>
      )}
    </div>
  );
}

export function ActiveUsersSpendChart({ data, days }: ActiveUsersSpendChartProps) {
  // Clickable legend: toggle either series off to compare scales on a
  // single axis. One series always stays visible — hiding both leaves an
  // empty chart, which is silly.
  const [hidden, setHidden] = useState<Set<"users" | "cost">>(new Set());
  function toggle(key: "users" | "cost") {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      if (next.size >= 2) return prev;
      return next;
    });
  }
  const showUsers = !hidden.has("users");
  const showCost = !hidden.has("cost");

  const chartData = useMemo(() => {
    const byDate = new Map(data.map((p) => [p.date, p]));
    const keys = days ? dayKeysForRange(days) : data.map((p) => p.date);
    return keys.map((key) => {
      const point = byDate.get(key);
      return {
        date: formatDateShort(key),
        users: point?.users ?? 0,
        cost:  point?.cost  ?? 0,
      };
    });
  }, [data, days]);
  // Empty when there's no data OR when every day has zero users + zero spend
  // — matches the agent chart's behavior so both panels render the same
  // placeholder for a brand-new account.
  const isEmpty = !data.some((p) => p.users > 0 || p.cost > 0);

  return (
    <Card className="flex h-full flex-col dark:bg-surface p-5">
      <div className="mb-4 shrink-0">
        <h3 className="text-heading-4 text-foreground">People spend over time</h3>
      </div>
      {isEmpty ? (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-body-sm text-faint-foreground">No spend yet</p>
        </div>
      ) : (
        <>
          <div className="min-h-0 flex-1">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData} margin={{ top: 16, right: 16, bottom: 4, left: 0 }}>
                <CartesianGrid {...GRID_PROPS} />
                <XAxis dataKey="date" tick={AXIS_TICK} axisLine={false} tickLine={false} tickMargin={8} minTickGap={40} padding={{ right: 20 }} />
                {/* Left axis: total spend (grey line) */}
                <YAxis
                  yAxisId="cost"
                  orientation="left"
                  tickFormatter={formatCost}
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  tickMargin={4}
                  width={56}
                />
                {/* Right axis: active user count (indigo line) */}
                <YAxis
                  yAxisId="users"
                  orientation="right"
                  tick={AXIS_TICK}
                  axisLine={false}
                  tickLine={false}
                  tickMargin={4}
                  width={36}
                  allowDecimals={false}
                />
                <Tooltip content={<CustomTooltip />} cursor={{ stroke: "var(--color-border)", strokeWidth: 1 }} />
                {showUsers && (
                  <Line yAxisId="users" type="monotone" dataKey="users" stroke={USERS_COLOR} strokeWidth={2} dot={{ r: 2.5, strokeWidth: 0, fill: USERS_COLOR }} activeDot={{ r: 4, strokeWidth: 0, fill: USERS_COLOR }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                )}
                {showCost && (
                  <Line yAxisId="cost"  type="monotone" dataKey="cost"  stroke={SPEND_COLOR} strokeWidth={2} dot={{ r: 2.5, strokeWidth: 0, fill: SPEND_COLOR }} activeDot={{ r: 4, strokeWidth: 0, fill: SPEND_COLOR }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                )}
              </LineChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3 flex items-center justify-center gap-4 text-body-sm">
            {([
              { key: "users", label: "By People", dot: "bg-primary-400", visible: showUsers },
              { key: "cost",  label: "Total spend", dot: "bg-muted-foreground", visible: showCost },
            ] as const).map((s) => (
              <button
                key={s.key}
                type="button"
                onClick={() => toggle(s.key)}
                aria-pressed={s.visible}
                className={cn(
                  "flex items-center gap-1.5 transition-opacity",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded",
                  s.visible ? "text-muted-foreground hover:text-foreground" : "text-faint-foreground opacity-50",
                )}
              >
                <span className={cn("size-2 rounded-full", s.dot, !s.visible && "opacity-40")} />
                <span className={cn(!s.visible && "line-through")}>{s.label}</span>
              </button>
            ))}
          </div>
        </>
      )}
    </Card>
  );
}
