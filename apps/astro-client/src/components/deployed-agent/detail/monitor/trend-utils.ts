export type WindowKey = "1h" | "24h" | "7d";

export type TimeParams = { start_time: string; end_time: string };

export function buildPreviousWindowParams(current: Record<WindowKey, TimeParams>): Record<WindowKey, TimeParams> {
  const result = {} as Record<WindowKey, TimeParams>;
  (["1h", "24h", "7d"] as WindowKey[]).forEach((key) => {
    const startMs = new Date(current[key].start_time).getTime();
    const endMs = new Date(current[key].end_time).getTime();
    const rangeMs = Math.max(0, endMs - startMs);
    const prevEnd = new Date(startMs);
    const prevStart = new Date(startMs - rangeMs);
    result[key] = {
      start_time: prevStart.toISOString(),
      end_time: prevEnd.toISOString(),
    };
  });
  return result;
}

export function percentChange(current: number | undefined, previous: number | undefined): number | null {
  if (current === undefined || previous === undefined) return null;
  if (!Number.isFinite(current) || !Number.isFinite(previous)) return null;
  if (previous === 0) return null;
  return ((current - previous) / Math.abs(previous)) * 100;
}

