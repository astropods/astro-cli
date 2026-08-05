export interface LogEntry {
  timestamp: string | null;
  level: string | null;
  message: string;
}

export type LogLevel = "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" | "FATAL" | "UNKNOWN";

const LEVEL_MAP: Record<string, LogLevel> = {
  trace:   "TRACE",
  debug:   "DEBUG",
  info:    "INFO",
  warn:    "WARN",
  warning: "WARN",
  error:   "ERROR",
  err:     "ERROR",
  fatal:   "FATAL",
  crit:    "FATAL",
  critical:"FATAL",
  unknown: "UNKNOWN",
};

export function normalizeLevel(level: string | null): LogLevel {
  if (!level) return "INFO";
  return LEVEL_MAP[level.toLowerCase()] ?? "INFO";
}

const ERROR_KEYWORD_RE = /\b(error|fatal|panic|exception)\b/i;
const WARN_KEYWORD_RE = /\b(warn|warning)\b/i;

/**
 * Level read out of the message text. Mirrors the keyword scan astro-server
 * applies to raw pod logs, for backends that report no level of their own
 * (Loki entries without detected_level, K8s pod logs). Heuristic by nature: a
 * line that merely mentions "error" reads as an error.
 */
export function inferLevelFromMessage(message: string): LogLevel | null {
  if (ERROR_KEYWORD_RE.test(message)) return "ERROR";
  if (WARN_KEYWORD_RE.test(message)) return "WARN";
  return null;
}

/**
 * Level to display and filter on: the reported level, else one inferred from
 * the message, else UNKNOWN. An entry with no level is not INFO — treating it
 * as INFO hid real errors from the error filter and its count.
 */
export function entryLevel(entry: LogEntry): LogLevel {
  if (entry.level) return normalizeLevel(entry.level);
  return inferLevelFromMessage(entry.message) ?? "UNKNOWN";
}

/** Badge text for a level. UNKNOWN renders blank: the backend reported no
 *  level, and the level column is sized for a 5-character token. */
export function levelLabel(level: LogLevel): string {
  return level === "UNKNOWN" ? "" : level;
}

export function levelColorClass(level: string | null): string {
  const normalized = normalizeLevel(level);
  if (normalized === "ERROR" || normalized === "FATAL") return "text-coral-600";
  if (normalized === "WARN") return "text-yellow-600";
  if (normalized === "INFO") return "text-blue-500";
  if (normalized === "DEBUG" || normalized === "TRACE" || normalized === "UNKNOWN") return "text-faint-foreground";
  return "text-foreground";
}

const FMT_TS_RE = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})?$/;

export function formatLogTimestamp(timestamp: string | null): string {
  if (!timestamp) return "—";
  const m = timestamp.match(FMT_TS_RE);
  if (!m) return timestamp;
  const date = m[1];
  const time = m[2];
  const millis = ((m[3] ?? "") + "000").slice(0, 3);
  return `${date} ${time}.${millis}`;
}

/** Compact: `MM-DD HH:MM:SS` — drops year and millis. */
export function formatLogTimestampCompact(timestamp: string | null): string {
  if (!timestamp) return "—";
  const m = timestamp.match(FMT_TS_RE);
  if (!m) return timestamp;
  const monthDay = m[1].slice(5); // MM-DD
  return `${monthDay} ${m[2]}`;
}
