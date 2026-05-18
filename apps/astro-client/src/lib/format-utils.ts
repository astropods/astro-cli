export function formatCost(v: number): string {
  const abs = Math.abs(v);
  const sign = v < 0 ? "-" : "";
  if (abs >= 1_000_000) return `${sign}$${(abs / 1_000_000).toFixed(2)}M`;
  if (abs >= 1_000) return `${sign}$${(abs / 1_000).toFixed(1)}K`;
  if (abs < 0.01 && abs > 0) return `${sign}$${abs.toFixed(4)}`;
  return `${sign}$${abs.toFixed(2)}`;
}

export function formatCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export function formatLatency(ms: number): string {
  if (ms >= 1_000) return `${(ms / 1_000).toFixed(1)}s`;
  return `${ms}ms`;
}

export function formatDateShort(d: string): string {
  const dt = new Date(d.includes("T") ? d : d + "T00:00:00Z");
  return dt.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function formatUptime(seconds: number): string {
  if (seconds < 60) { const v = seconds; return `${v} ${v === 1 ? "second" : "seconds"}`; }
  if (seconds < 3600) { const v = Math.floor(seconds / 60); return `${v} ${v === 1 ? "minute" : "minutes"}`; }
  if (seconds < 86400) { const v = Math.floor(seconds / 3600); return `${v} ${v === 1 ? "hour" : "hours"}`; }
  const v = Math.floor(seconds / 86400);
  return `${v} ${v === 1 ? "day" : "days"}`;
}

export function formatCPU(cores: number): string {
  if (cores < 0.01) return `${(cores * 1000).toFixed(0)}m`;
  return `${cores.toFixed(2)}`;
}

/** Formats a date as "April 22nd, 2026 at 14:30:45" in the given timezone (defaults to browser local). */
export function formatDateLong(dateStr: string, timezone?: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const tz = timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const fmt = (opts: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat('en-US', { timeZone: tz, ...opts }).format(date);
  const dayNum = parseInt(fmt({ day: 'numeric' }), 10);
  const v = dayNum % 100;
  const suffixes = ['th', 'st', 'nd', 'rd'];
  const suffix = suffixes[(v - 20) % 10] ?? suffixes[v] ?? 'th';
  const month = fmt({ month: 'long' });
  const year = fmt({ year: 'numeric' });
  const time = fmt({ hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  return `${month} ${dayNum}${suffix}, ${year} at ${time}`;
}
