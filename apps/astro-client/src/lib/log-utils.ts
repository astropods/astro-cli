const ISO_TS_RE = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$/;
const BASIC_TS_RE = /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+(.*)$/;
const FMT_TS_RE = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})?$/;

export const LOG_SUCCESS_RE = /✓|\bconnected\b|\bready\b|\bhealthy\b|\binitialized\b|\bregistered\b|\bsuccess\b|\bloaded\b|\bcomplete\b/i;
export const LOG_ERROR_RE = /\berror\b|\bfailed\b|\bexception\b|\bfatal\b/i;
export const LOG_WARN_RE = /\bwarn\b|\bwarning\b|\bretry\b|\battempt\b/i;

export function logLineColorClass(line: string): string {
  if (LOG_SUCCESS_RE.test(line)) return "text-green-700";
  if (LOG_ERROR_RE.test(line)) return "text-coral-600";
  if (LOG_WARN_RE.test(line)) return "text-yellow-600";
  return "text-foreground";
}

export function splitLogLineTimestamp(line: string): { timestamp: string | null; message: string } {
  const iso = line.match(ISO_TS_RE);
  if (iso) return { timestamp: iso[1], message: iso[2] };
  const basic = line.match(BASIC_TS_RE);
  if (basic) return { timestamp: basic[1], message: basic[2] };
  return { timestamp: null, message: line };
}

export function formatLogTimestamp(timestamp: string | null): string {
  if (!timestamp) return "—";
  const m = timestamp.match(FMT_TS_RE);
  if (!m) return timestamp;
  const date = m[1];
  const time = m[2];
  const millis = ((m[3] ?? "") + "000").slice(0, 3);
  return `${date} ${time}.${millis}`;
}
