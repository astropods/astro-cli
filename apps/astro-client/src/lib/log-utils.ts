export interface LogEntry {
  timestamp: string | null;
  level: string | null;
  message: string;
}

export type LogLevel = "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" | "FATAL";

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
};

export function normalizeLevel(level: string | null): LogLevel {
  if (!level) return "INFO";
  return LEVEL_MAP[level.toLowerCase()] ?? "INFO";
}

export function levelColorClass(level: string | null): string {
  const normalized = normalizeLevel(level);
  if (normalized === "ERROR" || normalized === "FATAL") return "text-coral-600";
  if (normalized === "WARN") return "text-yellow-600";
  if (normalized === "INFO") return "text-blue-500";
  if (normalized === "DEBUG" || normalized === "TRACE") return "text-faint-foreground";
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
