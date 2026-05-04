// Shared helpers for trace status display, formatting, and normalisation.

export type TraceStatus = "success" | "error" | "timeout";

export const STATUS_CONFIG: Record<TraceStatus, { label: string; bg: string; bdr: string; fg: string }> = {
  success: {
    label: "Success",
    bg: "color-mix(in oklch, var(--color-green-600) 12%, transparent)",
    bdr: "color-mix(in oklch, var(--color-green-600) 28%, transparent)",
    fg: "var(--color-green-600)",
  },
  error: {
    label: "Error",
    bg: "color-mix(in oklch, var(--color-coral-600) 12%, transparent)",
    bdr: "color-mix(in oklch, var(--color-coral-600) 28%, transparent)",
    fg: "var(--color-coral-600)",
  },
  timeout: {
    label: "Timeout",
    bg: "color-mix(in oklch, var(--color-yellow-500) 12%, transparent)",
    bdr: "color-mix(in oklch, var(--color-yellow-500) 28%, transparent)",
    fg: "var(--color-yellow-600)",
  },
};

export function normalizeStatus(raw: string): TraceStatus {
  if (raw === "error" || raw === "failed") return "error";
  if (raw === "timeout") return "timeout";
  return "success";
}

export function formatTimestamp(iso: string, includeSeconds = false): string {
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    ...(includeSeconds && { second: "2-digit" }),
  });
}

/**
 * Format a latency value in milliseconds to a human-readable string.
 * When `precise` is true, values between 1–10 s use two decimal places.
 */
export function formatLatency(ms: number, precise = false): string {
  if (ms >= 10_000) return `${(ms / 1_000).toFixed(1)}s`;
  if (ms >= 1_000) return `${(ms / 1_000).toFixed(precise ? 2 : 1)}s`;
  return `${Math.round(ms)}ms`;
}

export function formatCost(cost: number | undefined): string {
  if (!cost) return "\u2014";
  if (cost < 0.01) return `$${cost.toFixed(4)}`;
  return `$${cost.toFixed(2)}`;
}
