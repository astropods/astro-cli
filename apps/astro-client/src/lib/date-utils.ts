// dayKeysForRange returns "YYYY-MM-DD" UTC date keys for the trailing
// `days` window ending today, inclusive. Used to zero-fill chart axes so
// the X-axis spans the full requested range regardless of where activity
// actually started.
//
// UTC, not local-time: charts on Insights are server-side date-keyed
// (Langfuse returns UTC bucket dates), so the client must walk UTC to
// align rows. A separate local-time variant lives in
// `agent-detail/charts/aggregate-token-buckets.ts` for the local-day
// token-usage bars — don't try to merge them.
export function dayKeysForRange(days: number): string[] {
  const today = new Date();
  today.setUTCHours(0, 0, 0, 0);
  const keys: string[] = [];
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setUTCDate(d.getUTCDate() - i);
    const y = d.getUTCFullYear();
    const m = String(d.getUTCMonth() + 1).padStart(2, "0");
    const day = String(d.getUTCDate()).padStart(2, "0");
    keys.push(`${y}-${m}-${day}`);
  }
  return keys;
}
