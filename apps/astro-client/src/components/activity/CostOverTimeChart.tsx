import { useMemo } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  Rectangle,
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
import {
  GRID_PROPS,
  SeriesLegend,
  SeriesTooltip,
  X_AXIS_BASE,
  useHiddenSeries,
  yAxisProps,
} from "./chart-chrome";
import { topSegmentKey } from "./stacked-bars";

interface CostOverTimeProps {
  data: Array<{ date: string; models: Array<{ model: string; cost_usd: number }> }>;
  days?: number;
  /** Last day the axis covers ("YYYY-MM-DD", UTC), from the server's reported
   *  window. Defaults to today, which is only right when the data does too. */
  endDate?: string;
  colorMap?: Record<string, string>;
  seriesLabels?: Record<string, string>;
  variant?: "bar" | "line";
}

const Y_AXIS_PROPS = yAxisProps(formatCost);

export function CostOverTimeChart({ data, days, endDate, colorMap: externalColorMap, seriesLabels, variant = "bar" }: CostOverTimeProps) {
  const { chartData, allModels, colorMap } = useMemo(() => {
    const models = [...new Set(data.flatMap((d) => d.models.map((m) => m.model)))];
    const cMap = externalColorMap ?? buildModelColorMap(models);
    const byDate = new Map(data.map((d) => [d.date, d]));
    const keys = days ? dayKeysForRange(days, endDate) : data.map((d) => d.date);
    const rows = keys.map((key) => {
      const costByModel = byDate.get(key)?.models.reduce<Record<string, number>>(
        (acc, m) => { acc[m.model] = m.cost_usd; return acc; }, {},
      ) ?? {};
      const row: Record<string, string | number> = { date: formatDateShort(key) };
      for (const m of models) row[m] = costByModel[m] ?? 0;
      return row;
    });
    return { chartData: rows, allModels: models, colorMap: cMap };
  }, [data, days, endDate, externalColorMap]);

  const { hidden, visible: visibleModels, toggle } = useHiddenSeries(allModels);

  // Which segment caps each bar, derived once per row rather than per rendered
  // rectangle. Recharts retains rectangles to animate between them, so the
  // visible set is folded into the Bar key to remount when the cap can move.
  const stackSignature = visibleModels.join("|");
  const rows = useMemo(
    () => chartData.map((row) => ({ ...row, cap: topSegmentKey(row, visibleModels) })),
    [chartData, visibleModels],
  );

  // Skeleton intentionally removed: account-scoped queries use placeholderData
  // so the previous window's chart stays mounted during cross-key refetches,
  // and the global progress bar signals navigation. On a true cold load we
  // render the empty state briefly before data arrives.
  //
  // Also empty when the per-day rows exist but every model's cost is zero
  // (which is what the data-shape looks like for an account that has the
  // date axis materialised but no spend yet — without this, the chart
  // renders a bare grid with no placeholder).
  const isEmpty = !data.some((d) => d.models.some((m) => m.cost_usd > 0));

  return (
    <Card className="flex h-full flex-col p-5">
      <div className="mb-4 shrink-0">
        <h3 className="text-heading-4 text-foreground">Agent spend over time</h3>
      </div>
      {isEmpty ? (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-body-sm text-faint-foreground">No spend yet</p>
        </div>
      ) : (
        <>
          <div className="min-h-0 flex-1">
            <ResponsiveContainer width="100%" height="100%">
              {variant === "line" ? (
                <LineChart data={rows} margin={{ top: 16, right: 52, bottom: 4, left: 0 }}>
                  <CartesianGrid {...GRID_PROPS} />
                  <XAxis {...X_AXIS_BASE} padding={{ right: 20 }} />
                  <YAxis {...Y_AXIS_PROPS} />
                  <Tooltip content={<SeriesTooltip colors={colorMap} names={seriesLabels} format={formatCost} />} cursor={{ stroke: "var(--color-border)", strokeWidth: 1 }} />
                  {visibleModels.map((model) => (
                    <Line key={model} type="monotone" dataKey={model} stroke={colorMap[model]} strokeWidth={2} dot={false} activeDot={{ r: 4, strokeWidth: 0, fill: colorMap[model] }} isAnimationActive animationDuration={500} animationEasing="ease-out" />
                  ))}
                </LineChart>
              ) : (
                <BarChart data={rows} margin={{ top: 16, right: 52, bottom: 4, left: 0 }} barCategoryGap="20%" maxBarSize={56}>
                  <CartesianGrid {...GRID_PROPS} />
                  <XAxis {...X_AXIS_BASE} />
                  <YAxis {...Y_AXIS_PROPS} />
                  <Tooltip content={<SeriesTooltip colors={colorMap} names={seriesLabels} format={formatCost} />} cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }} />
                  {visibleModels.map((model) => (
                    <Bar
                      key={`${model}:${stackSignature}`}
                      dataKey={model}
                      stackId="cost"
                      fill={colorMap[model]}
                      fillOpacity={0.85}
                      shape={(props: { payload?: { cap?: string } }) => (
                        <Rectangle {...props} radius={props.payload?.cap === model ? [3, 3, 0, 0] : 0} />
                      )}
                      isAnimationActive
                      animationDuration={500}
                      animationEasing="ease-out"
                    />
                  ))}
                </BarChart>
              )}
            </ResponsiveContainer>
          </div>
          <SeriesLegend
            keys={allModels}
            colors={colorMap}
            names={seriesLabels}
            hidden={hidden}
            onToggle={toggle}
          />
        </>
      )}
    </Card>
  );
}
