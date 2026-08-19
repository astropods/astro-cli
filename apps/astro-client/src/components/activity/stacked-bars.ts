/** Which series is the visible cap of a stacked bar for one row.
 *
 * Recharts stacks in declaration order, so the last series *with a value* is on
 * top. Rounding the last declared one instead puts the corner on a zero-height
 * rect and leaves the real top square.
 */
export function topSegmentKey(
  row: Record<string, unknown> | undefined,
  stackOrder: string[],
): string {
  if (!row) return "";
  let top = "";
  for (const key of stackOrder) {
    if (Number(row[key] ?? 0) > 0) top = key;
  }
  return top;
}
