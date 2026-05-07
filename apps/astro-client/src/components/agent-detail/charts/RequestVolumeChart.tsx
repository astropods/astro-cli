import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { Loader2 } from "lucide-react";
import { formatCompactNumber, type ChartColors, type RequestVolumePoint } from "./chart-utils";

export type { RequestVolumePoint };

// ---------------------------------------------------------------------------
// Tooltip
// ---------------------------------------------------------------------------

function CustomTooltip({
  active,
  payload,
  label,
  colors,
}: {
  active?: boolean;
  payload?: { name: string; value: number }[];
  label?: string;
  colors: ChartColors;
}) {
  if (!active || !payload?.length || !label) return null;
  const requests = payload.find((p) => p.name === "requests");
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 text-body-sm shadow-lg backdrop-blur supports-[backdrop-filter]:bg-surface/90">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2">
        <span
          className="size-2 shrink-0 rounded-full"
          style={{ backgroundColor: colors.inputFill }}
        />
        <span className="text-muted-foreground">Requests</span>
        <span className="ml-auto font-mono font-medium text-foreground">
          {formatCompactNumber(requests?.value ?? 0)}
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Chart component
// ---------------------------------------------------------------------------

export interface RequestVolumeChartProps {
  points: RequestVolumePoint[];
  colors: ChartColors;
  loading?: boolean;
}

export function RequestVolumeChart({
  points,
  colors,
  loading,
}: RequestVolumeChartProps) {
  return (
    <div className="flex h-full flex-col rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
      {loading ? (
        <div className="flex min-h-[200px] flex-1 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <>
          <div className="min-h-0 flex-1">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart
                data={points}
                margin={{ top: 8, right: 4, bottom: 0, left: 0 }}
              >
                <defs>
                  <linearGradient
                    id="requestAreaGrad"
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="1"
                  >
                    <stop
                      offset="0%"
                      stopColor={colors.inputFill}
                      stopOpacity={0.35}
                    />
                    <stop
                      offset="95%"
                      stopColor={colors.inputFill}
                      stopOpacity={0.03}
                    />
                  </linearGradient>
                </defs>

                <CartesianGrid
                  strokeDasharray="3 3"
                  vertical={false}
                  stroke="var(--color-border)"
                  strokeOpacity={0.5}
                />

                <XAxis
                  dataKey="label"
                  tick={{
                    fill: "var(--color-muted-foreground)",
                    fontSize: 11,
                    fontFamily: "var(--font-mono)",
                  }}
                  axisLine={false}
                  tickLine={false}
                  tickMargin={8}
                  minTickGap={40}
                />

                <YAxis
                  tickFormatter={formatCompactNumber}
                  tick={{
                    fill: "var(--color-muted-foreground)",
                    fontSize: 11,
                    fontFamily: "var(--font-mono)",
                  }}
                  axisLine={false}
                  tickLine={false}
                  tickMargin={4}
                  width={36}
                />

                <Tooltip
                  content={<CustomTooltip colors={colors} />}
                  cursor={{
                    stroke: "var(--color-border)",
                    strokeWidth: 1,
                    strokeDasharray: "4 4",
                  }}
                />

                <Area
                  type="monotone"
                  dataKey="requests"
                  name="requests"
                  fill="url(#requestAreaGrad)"
                  stroke={colors.inputFill}
                  strokeWidth={2}
                  isAnimationActive
                  animationDuration={525}
                  animationEasing="ease-out"
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          {/* Legend */}
          <div className="mt-3 flex items-center justify-center gap-5">
            <div className="flex items-center gap-1.5 text-mono-sm text-muted-foreground">
              <span
                className="size-2 rounded-full"
                style={{ backgroundColor: colors.inputFill }}
              />
              Requests
            </div>
          </div>
        </>
      )}
    </div>
  );
}
