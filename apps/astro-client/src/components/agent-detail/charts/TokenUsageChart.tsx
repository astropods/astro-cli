import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { Loader2 } from "lucide-react";
import { formatCompactNumber, type ChartColors, type TokenUsageBar } from "./chart-utils";

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
  const input = payload.find((p) => p.name === "inputTokens");
  const output = payload.find((p) => p.name === "outputTokens");
  const total = (input?.value ?? 0) + (output?.value ?? 0);
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 text-body-sm shadow-lg backdrop-blur supports-[backdrop-filter]:bg-surface/90">
      <p className="mb-1.5 text-mono-sm text-muted-foreground">{label}</p>
      <div className="flex flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: colors.inputFill }} />
          <span className="text-muted-foreground">Input</span>
          <span className="ml-auto font-mono font-medium text-foreground">
            {formatCompactNumber(input?.value ?? 0)}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: colors.outputFill }} />
          <span className="text-muted-foreground">Output</span>
          <span className="ml-auto font-mono font-medium text-foreground">
            {formatCompactNumber(output?.value ?? 0)}
          </span>
        </div>
        <div className="mt-1 flex items-center gap-2 border-t border-border pt-1">
          <span className="text-muted-foreground">Total</span>
          <span className="ml-auto font-mono font-medium text-foreground">
            {formatCompactNumber(total)}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Chart component
// ---------------------------------------------------------------------------

export interface TokenUsageChartProps {
  bars: TokenUsageBar[];
  colors: ChartColors;
  loading?: boolean;
}

export function TokenUsageChart({
  bars,
  colors,
  loading,
}: TokenUsageChartProps) {
  return (
    <div className="rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
      {loading ? (
        <div className="flex h-[300px] items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={bars} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
            <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--color-border)" strokeOpacity={0.5} />
            <XAxis
              dataKey="label"
              tick={{ fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" }}
              axisLine={false}
              tickLine={false}
              tickMargin={8}
              minTickGap={40}
            />
            <YAxis
              tickFormatter={formatCompactNumber}
              tick={{ fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" }}
              axisLine={false}
              tickLine={false}
              tickMargin={4}
              width={52}
            />
            <Tooltip content={<CustomTooltip colors={colors} />} cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }} />
            <Bar dataKey="inputTokens" name="inputTokens" stackId="tokens" fill={colors.inputFill} fillOpacity={0.85} radius={[0, 0, 0, 0]} isAnimationActive animationDuration={525} animationEasing="ease-out" />
            <Bar dataKey="outputTokens" name="outputTokens" stackId="tokens" fill={colors.outputFill} fillOpacity={0.85} radius={[3, 3, 0, 0]} isAnimationActive animationDuration={525} animationEasing="ease-out" />
          </BarChart>
        </ResponsiveContainer>
      )}

      {/* Legend */}
      {!loading && (
        <div className="mt-3 flex items-center justify-center gap-5">
          <div className="flex items-center gap-1.5 text-mono-sm text-muted-foreground">
            <span className="size-2 rounded-full" style={{ backgroundColor: colors.inputFill }} />
            Input tokens
          </div>
          <div className="flex items-center gap-1.5 text-mono-sm text-muted-foreground">
            <span className="size-2 rounded-full" style={{ backgroundColor: colors.outputFill }} />
            Output tokens
          </div>
        </div>
      )}
    </div>
  );
}
