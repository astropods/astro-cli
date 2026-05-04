import type { MetricsBucket } from "@/lib/api";
import type { TokenUsageBar, RequestVolumePoint } from "./chart-utils";

/**
 * Format a local Date as "YYYY-MM-DD" for use as a grouping key.
 */
function toLocalDateKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/**
 * Format a local date key as a short label like "Apr 28".
 */
function formatDateLabel(key: string): string {
  const [y, m, d] = key.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

interface DayAccumulator {
  input: number;
  output: number;
  requests: number;
  errors: number;
  /** Weighted sum: avg_latency × trace_count for each bucket. */
  latencyWeightedSum: number;
  /** Max p95 seen across buckets in the day. */
  p95Max: number;
}

function emptyDay(): DayAccumulator {
  return { input: 0, output: 0, requests: 0, errors: 0, latencyWeightedSum: 0, p95Max: 0 };
}

/**
 * Group hourly UTC buckets into local-day accumulators.
 */
function groupByLocalDay(buckets: MetricsBucket[]): Map<string, DayAccumulator> {
  const byDay = new Map<string, DayAccumulator>();

  for (const b of buckets) {
    const key = toLocalDateKey(new Date(b.timestamp));
    const acc = byDay.get(key) ?? emptyDay();
    acc.input += b.input_tokens;
    acc.output += b.output_tokens;
    acc.requests += b.trace_count;
    acc.errors += b.error_count;
    acc.latencyWeightedSum += (b.avg_latency_ms || 0) * b.trace_count;
    acc.p95Max = Math.max(acc.p95Max, b.p95_latency_ms || 0);
    if (!byDay.has(key)) byDay.set(key, acc);
  }

  return byDay;
}

/**
 * Generate a padded array of day keys covering the range, ending today.
 */
function dayKeysForRange(days: number): string[] {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const keys: string[] = [];
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setDate(d.getDate() - i);
    keys.push(toLocalDateKey(d));
  }
  return keys;
}

/**
 * Aggregate hourly UTC buckets into local-day token-usage bars.
 */
export function aggregateByLocalDay(
  buckets: MetricsBucket[],
  days: number,
): TokenUsageBar[] {
  const byDay = groupByLocalDay(buckets);
  return dayKeysForRange(days).map((key) => {
    const data = byDay.get(key);
    return {
      label: formatDateLabel(key),
      inputTokens: data?.input ?? 0,
      outputTokens: data?.output ?? 0,
    };
  });
}

/**
 * Aggregate hourly UTC buckets into local-day request-volume points
 * with weighted-average latency and p95 latency.
 */
export function aggregateRequestsByLocalDay(
  buckets: MetricsBucket[],
  days: number,
): RequestVolumePoint[] {
  const byDay = groupByLocalDay(buckets);
  return dayKeysForRange(days).map((key) => {
    const data = byDay.get(key);
    const requests = data?.requests ?? 0;
    return {
      label: formatDateLabel(key),
      requests,
      errors: data?.errors ?? 0,
      avgLatencyMs: requests > 0 ? data!.latencyWeightedSum / requests : 0,
      p95LatencyMs: data?.p95Max ?? 0,
    };
  });
}
