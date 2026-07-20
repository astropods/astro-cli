// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type DayRange = "7d" | "14d" | "30d";

export const DAY_RANGES: { key: DayRange; label: string; days: number }[] = [
  { key: "7d", label: "7D", days: 7 },
  { key: "14d", label: "14D", days: 14 },
  { key: "30d", label: "30D", days: 30 },
];

export function buildTimeParams(
  days: number,
  options?: { granularity?: string },
) {
  const end = new Date();
  end.setSeconds(0, 0);
  const start = new Date(end);
  start.setDate(start.getDate() - days);
  return {
    start_time: start.toISOString(),
    end_time: end.toISOString(),
    ...(options?.granularity ? { granularity: options.granularity } : {}),
  };
}

export interface TokenUsageBar {
  label: string;
  inputTokens: number;
  outputTokens: number;
}

export interface RequestVolumePoint {
  label: string;
  requests: number;
  errors: number;
  avgLatencyMs: number;
  p95LatencyMs: number;
  minLatencyMs: number;
  maxLatencyMs: number;
}

export interface ChartColors {
  inputFill: string;
  outputFill: string;
}

// ---------------------------------------------------------------------------
// Chart colors — static indigo palette from the theme
// ---------------------------------------------------------------------------

export const CHART_COLORS: { dark: ChartColors; light: ChartColors } = {
  dark: { inputFill: "var(--color-indigo-500)", outputFill: "var(--color-indigo-300)" },
  light: { inputFill: "var(--color-indigo-700)", outputFill: "var(--color-indigo-400)" },
};

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

export function formatCompactNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}
