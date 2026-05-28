import { useMemo } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
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
import { buildModelColorMap } from "./model-colors";

interface CostOverTimeProps {
  data: Array<{ date: string; models: Array<{ model: string; cost_usd: number }> }>;
  days?: number;
  colorMap?: Record<string, string>;
  seriesLabels?: Record<string, string>;
  variant?: "bar" | "line";
}

const AXIS_TICK = { fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" } as const;
const X_AXIS_BASE = { dataKey: "date", tick: AXIS_TICK, axisLine: false, tickLine: false, tickMargin: 8, minTickGap: 40 } as const;
const Y_AXIS_PROPS = { tickFormatter: formatCost, tick: AXIS_TICK, axisLine: false, tickLine: false, tickMargin: 4, width: 56 } as const;
const GRID_PROPS = { strokeDasharray: "3 3", vertical: false as const, stroke: "var(--color-border)", strokeOpacity: 0.5 } as const;

function CustomTooltip({
  active,
  payload,
  label,
  colorMap,
  seriesLabels,
}: {
  active?: boolean;
  payload?: { name: string; value: number }[];
  label?: string;
  colorMap: Record<string, string>;
  seriesLabels?: Record<string, string>;
}) {
  if (!active || !payload?.length) return null;
  const total = payload.reduce((s, p) => s + (p.value ?? 0), 0);
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 shadow-lg backdrop-blur">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      {payload.map((p) => (
        <div key={p.name} className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: colorMap[p.name] }} />
          <span className="font-mono text-body-sm text-muted-foreground">{seriesLabels?.[p.name] ?? p.name}</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{formatCost(p.value)}</span>
        </div>
      ))}
      {payload.length > 1 && (
        <div className="mt-1 flex items-center gap-2 border-t border-border pt-1">
          <span className="font-mono text-body-sm text-muted-foreground">Total</span>
          <span className="ml-auto font-mono text-body-sm font-medium text-foreground">{formatCost(total)}</span>
        </div>
      )}
    </div>
  );
}

export function CostOverTimeChart({ data, days, colorMap: externalColorMap, seriesLabels, variant = "bar" }: CostOverTimeProps) {
  const { chartData, allModels, colorMap } = useMemo(() => {
    const models = [...new Set(data.flatMap((d) => d.models.map((m) => m.model)))];
    const cMap = externalColorMap ?? buildModelColorMap(models);
    const byDate = new Map(data.map((d) => [d.date, d]));
    const keys = days ? dayKeysForRange(days) : data.map((d) => d.date);
    const rows = keys.map((key) => {
      const costByModel = byDate.get(key)?.models.reduce<Record<string, number>>(
        (acc, m) => { acc[m.model] = m.cost_usd; return acc; }, {},
      ) ?? {};
      const row: Record<string, string | number> = { date: formatDateShort(key) };
      for (const m of models) row[m] = costByModel[m] ?? 0;
      return row;
    });
    return { chartData: rows, allModels: models, colorMap: cMap };
  }, [data, days, externalColorMap]);

  // Skeleton intentionally removed: account-scoped queries use placeholderData
  // so the previous window's chart stays mounted during cross-key refetches,
  // and the global progress bar signals navigation. On a true cold load we
  // render the empty state briefly before data arrives.
  const isEmpty = data.length === 0;

  return (
    <Card className="flex h-full flex-col dark:bg-surface p-5">
      <div className="mb-4 shrink-0">
        <h3 className="text-heading-4 text-foreground">Agent Spend</h3>
      </div>
      {isEmpty ? (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-body-sm text-faint-foreground">No cost data for this period</p>
        </div>
      ) : (
        <>
          <div className="min-h-0 flex-1">
            <ResponsiveContainer width="100%" height="100%">
              {variant === "line" ? (
                <LineChart data={chartData} margin={{ top: 16, right: 52, bottom: 4, left: 0 }}>
                  <CartesianGrid {...GRID_PROPS} />
                  <XAxis {...X_AXIS_BASE} padding={{ right: 20 }} />
                  <YAxis {...Y_AXIS_PROPS} />
                  <Tooltip content={<CustomTooltip colorMap={colorMap} seriesLabels={seriesLabels} />} cursor={{ stroke: "var(--color-border)", strokeWidth: 1 }} />
                  {allModels.map((model) => (
                    <Line key={model} type="monotone" dataKey={model} stroke={colorMap[model]} strokeWidth={2} dot={{ r: 2.5, strokeWidth: 0, fill: colorMap[model] }} activeDot={{ r: 4, strokeWidth: 0, fill: colorMap[model] }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                  ))}
                </LineChart>
              ) : (
                <BarChart data={chartData} margin={{ top: 16, right: 52, bottom: 4, left: 0 }} barCategoryGap="20%" maxBarSize={56}>
                  <CartesianGrid {...GRID_PROPS} />
                  <XAxis {...X_AXIS_BASE} />
                  <YAxis {...Y_AXIS_PROPS} />
                  <Tooltip content={<CustomTooltip colorMap={colorMap} seriesLabels={seriesLabels} />} cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }} />
                  {allModels.map((model, i) => (
                    <Bar key={model} dataKey={model} stackId="cost" fill={colorMap[model]} fillOpacity={0.85} radius={i === allModels.length - 1 ? [3, 3, 0, 0] : [0, 0, 0, 0]} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                  ))}
                </BarChart>
              )}
            </ResponsiveContainer>
          </div>
          {allModels.length > 0 && (
            <div className="mt-3 shrink-0 flex flex-wrap items-center justify-center gap-x-5 gap-y-1.5">
              {allModels.map((m) => (
                <div key={m} className="flex items-center gap-1.5 text-mono-sm text-muted-foreground">
                  <span className="size-2 rounded-full" style={{ backgroundColor: colorMap[m] }} />
                  {seriesLabels?.[m] ?? m}
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </Card>
  );
}
