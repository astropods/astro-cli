const C = {
  bgDeep: "var(--muted)",
  border: "var(--border)",
  faint: "var(--faint-foreground)",
  text: "var(--foreground)",
  success: "var(--color-green-700)",
  coral: "var(--color-coral-600)",
  bgAlt: "var(--surface)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  heading2: "var(--text-heading-2)",
  bodySm: "var(--text-body-sm)",
  label: "var(--text-label)",
} as const;

export type WindowKey = "1h" | "24h" | "7d";

export interface WindowTrend {
  total_traces: number | null;
  error_rate: number | null;
  avg_latency_ms: number | null;
  p95_latency_ms: number | null;
}

interface HeadlineMetricsProps {
  summary: {
    total_traces: number;
    metrics: {
      avg_latency_ms: number;
      p95_latency_ms: number;
      error_rate: number;
    };
  } | null;
  summaryLoading: boolean;
  trendLoading: boolean;
  selectedWindow: WindowKey;
  trends: Record<WindowKey, WindowTrend>;
}

function SkeletonBar({ width = "65%" }: { width?: string }) {
  return (
    <span
      className="dp-pulse"
      style={{
        display: "block",
        width,
        height: 14,
        borderRadius: 7,
        background: `linear-gradient(90deg, ${C.bgDeep} 0%, ${C.border} 45%, ${C.bgDeep} 100%)`,
      }}
    />
  );
}

function formatTrend(v: number | null): string {
  if (v === null || Number.isNaN(v) || !Number.isFinite(v)) return "—";
  return `${Math.abs(v).toFixed(1)}%`;
}

function formatLatencyHeadlineMs(ms: number | null): string {
  if (ms === null || Number.isNaN(ms) || !Number.isFinite(ms)) return "—";
  return `${Math.round(ms)}ms`;
}

function TrendIndicator({
  value,
  higherIsBetter,
}: {
  value: number | null;
  higherIsBetter: boolean;
}) {
  const dir = value === null ? "flat" : value > 0 ? "up" : value < 0 ? "down" : "flat";
  const arrow = dir === "up" ? "\u2191" : dir === "down" ? "\u2193" : "\u2014";
  const isGood = dir === "flat" ? null : higherIsBetter ? dir === "up" : dir === "down";
  const color = isGood === null ? C.faint : isGood ? C.success : C.coral;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6, marginTop: 8 }}>
      <span style={{ fontFamily: S.body, fontSize: T.bodySm, fontWeight: 700, color }}>{formatTrend(value)}</span>
      <span style={{ fontFamily: S.body, fontSize: T.bodySm, fontWeight: 700, color }}>{arrow}</span>
    </div>
  );
}

export function HeadlineMetrics({ summary, summaryLoading, trendLoading, selectedWindow, trends }: HeadlineMetricsProps) {
  const cards = [
    {
      label: "TOTAL REQUESTS",
      value: summary ? String(summary.total_traces) : "—",
      trend: (w: WindowKey) => trends[w].total_traces,
      higherIsBetter: true,
    },
    {
      label: "ERROR RATE",
      value: summary ? `${(summary.metrics.error_rate * 100).toFixed(1)}%` : "—",
      trend: (w: WindowKey) => trends[w].error_rate,
      higherIsBetter: false,
    },
    {
      label: "AVG LATENCY",
      value: summary ? formatLatencyHeadlineMs(summary.metrics.avg_latency_ms) : "—",
      trend: (w: WindowKey) => trends[w].avg_latency_ms,
      higherIsBetter: false,
    },
    {
      label: "P95 LATENCY",
      value: summary ? formatLatencyHeadlineMs(summary.metrics.p95_latency_ms) : "—",
      trend: (w: WindowKey) => trends[w].p95_latency_ms,
      higherIsBetter: false,
    },
  ] as const;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
      {cards.map((card) => (
        <div key={card.label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px" }}>
          <span style={{ display: "block", fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, marginBottom: 8 }}>
            {card.label}
          </span>
          {summaryLoading ? (
            <SkeletonBar width="70%" />
          ) : (
            <span style={{ display: "block", fontFamily: S.body, fontSize: T.heading2, fontWeight: 700, color: C.text }}>{card.value}</span>
          )}
          {trendLoading ? (
            <div style={{ marginTop: 8, display: "flex", gap: 6, alignItems: "center" }}>
              <SkeletonBar width="24%" />
              <SkeletonBar width="8%" />
              <SkeletonBar width="30%" />
            </div>
          ) : (
            <TrendIndicator
              value={card.trend(selectedWindow)}
              higherIsBetter={card.higherIsBetter}
            />
          )}
        </div>
      ))}
    </div>
  );
}

