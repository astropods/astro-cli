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
