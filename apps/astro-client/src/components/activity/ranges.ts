export type ActivityRange = "7d" | "14d" | "30d" | "all";

export const ACTIVITY_RANGES: { key: ActivityRange; label: string; days?: number }[] = [
  { key: "7d", label: "7D", days: 7 },
  { key: "14d", label: "14D", days: 14 },
  { key: "30d", label: "30D", days: 30 },
  { key: "all", label: "All" },
];

export function buildPeriodParams(range: ActivityRange): { from?: string; to?: string } {
  if (range === "all") return {};
  const days = ACTIVITY_RANGES.find((r) => r.key === range)?.days ?? 7;
  const to = new Date();
  to.setUTCHours(23, 59, 59, 999);
  const from = new Date();
  from.setUTCDate(from.getUTCDate() - (days - 1));
  from.setUTCHours(0, 0, 0, 0);
  return { from: from.toISOString(), to: to.toISOString() };
}
