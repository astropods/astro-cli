export function logLineColorClass(line: string): string {
  const l = line.toLowerCase();
  if (/✓|connected|ready|healthy|initialized|registered|success|loaded|complete/.test(l)) return "text-green-700";
  if (/error|failed|exception|fatal/.test(l)) return "text-coral-600";
  if (/warn|warning|retry|attempt/.test(l)) return "text-yellow-500";
  return "text-foreground";
}

export function splitLogLineTimestamp(line: string): { timestamp: string | null; message: string } {
  const iso = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$/);
  if (iso) return { timestamp: iso[1], message: iso[2] };
  const basic = line.match(/^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+(.*)$/);
  if (basic) return { timestamp: basic[1], message: basic[2] };
  return { timestamp: null, message: line };
}

export function formatLogTimestamp(timestamp: string | null): string {
  if (!timestamp) return "—";
  const m = timestamp.match(
    /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})?$/,
  );
  if (!m) return timestamp;
  const date = m[1];
  const time = m[2];
  const millis = ((m[3] ?? "") + "000").slice(0, 3);
  return `${date} ${time}.${millis}`;
}
