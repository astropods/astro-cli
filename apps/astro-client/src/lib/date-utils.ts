// dayKeysForRange returns "YYYY-MM-DD" UTC date keys for the trailing
// `days` window ending on `endKey`, inclusive. Used to zero-fill chart axes so
// the X-axis spans the full requested range regardless of where activity
// actually started.
//
// `endKey` is the last day the data covers, and callers should pass the one the
// server reported (`range.period.end`) rather than letting it default. Assuming
// today is what drew a permanently empty trailing bucket on the rollup-backed
// path, whose windows end at the last complete day — the axis has to end where
// the data ends, not where the clock is.
//
// UTC, not local-time: charts on Insights are server-side date-keyed
// (Langfuse returns UTC bucket dates), so the client must walk UTC to
// align rows. A separate local-time variant lives in
// `agent-detail/charts/aggregate-token-buckets.ts` for the local-day
// token-usage bars — don't try to merge them.
export function dayKeysForRange(days: number, endKey?: string): string[] {
  const end = endKey ? new Date(`${endKey}T00:00:00Z`) : new Date();
  if (Number.isNaN(end.getTime())) return dayKeysForRange(days);
  end.setUTCHours(0, 0, 0, 0);
  const keys: string[] = [];
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(end);
    d.setUTCDate(d.getUTCDate() - i);
    keys.push(utcDayKey(d));
  }
  return keys;
}

// utcDayKey formats a Date as its "YYYY-MM-DD" UTC day.
export function utcDayKey(d: Date): string {
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// dayKeyFromISO converts an RFC3339 timestamp (the server's `period.start` /
// `period.end`) to its UTC day key, or undefined if it isn't parseable —
// callers treat that as "no reported window" and fall back to today.
export function dayKeyFromISO(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? undefined : utcDayKey(d);
}
