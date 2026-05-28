import { useMemo } from "react";
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
// with the entity list; these are stable per-chart). USERS_COLOR matches
// the Unidentified user-classification color so the two related concepts
// share a hue across the UI.
const USERS_COLOR = "var(--warning)";
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
  const users = payload.find((p) => p.dataKey === "users")?.value ?? 0;
  const cost  = payload.find((p) => p.dataKey === "cost")?.value ?? 0;
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 shadow-lg backdrop-blur">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2">
        <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: USERS_COLOR }} />
        <span className="font-mono text-body-sm text-muted-foreground">Users</span>
        <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{users}</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: SPEND_COLOR }} />
        <span className="font-mono text-body-sm text-muted-foreground">Total spend</span>
        <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{formatCost(cost)}</span>
      </div>
    </div>
  );
}

export function ActiveUsersSpendChart({ data, days }: ActiveUsersSpendChartProps) {
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
  const isEmpty = data.length === 0;

  return (
    <Card className="flex h-full flex-col dark:bg-surface p-5">
      <div className="mb-4 shrink-0">
        <h3 className="text-heading-4 text-foreground">Total Spend</h3>
      </div>
      {isEmpty ? (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-body-sm text-faint-foreground">No activity for this period</p>
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
                {/* Right axis: active user count (orange line) */}
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
                <Line yAxisId="users" type="monotone" dataKey="users" stroke={USERS_COLOR} strokeWidth={2} dot={{ r: 2.5, strokeWidth: 0, fill: USERS_COLOR }} activeDot={{ r: 4, strokeWidth: 0, fill: USERS_COLOR }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                <Line yAxisId="cost"  type="monotone" dataKey="cost"  stroke={SPEND_COLOR} strokeWidth={2} dot={{ r: 2.5, strokeWidth: 0, fill: SPEND_COLOR }} activeDot={{ r: 4, strokeWidth: 0, fill: SPEND_COLOR }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
              </LineChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3 flex items-center justify-center gap-4 text-body-sm">
            <div className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: USERS_COLOR }} />
              <span className="text-muted-foreground">Users</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: SPEND_COLOR }} />
              <span className="text-muted-foreground">Total spend</span>
            </div>
          </div>
        </>
      )}
    </Card>
  );
}
