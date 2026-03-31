import { MetricCard } from "@/components/MetricCard";

const C = {
  bgDeep: "var(--muted)",
  border: "var(--border)",
  bgAlt: "var(--surface)",
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

function SkeletonBar({ width = "65%", height = 24 }: { width?: string; height?: number }) {
  return (
    <span
      className="dp-pulse"
      style={{
        display: "block",
        width,
        height,
        borderRadius: Math.min(7, height / 2),
        background: `linear-gradient(90deg, ${C.bgDeep} 0%, ${C.border} 45%, ${C.bgDeep} 100%)`,
      }}
    />
  );
}

function formatLatencyHeadlineMs(ms: number | null): string {
  if (ms === null || Number.isNaN(ms) || !Number.isFinite(ms)) return "—";
  return `${Math.round(ms)}ms`;
}

export function MetricCardSkeleton() {
  return (
    <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px", height: 100, boxSizing: "border-box" }}>
      <SkeletonBar width="60%" height={14} />
      <div style={{ marginTop: 8 }}>
        <SkeletonBar width="70%" />
      </div>
      <div style={{ marginTop: 8, display: "flex", gap: 6, alignItems: "center" }}>
        <SkeletonBar width="24%" height={14} />
        <SkeletonBar width="8%" height={14} />
        <SkeletonBar width="30%" height={14} />
      </div>
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
        <MetricCard
          key={card.label}
          label={card.label}
          value={card.value}
          trend={card.trend(selectedWindow)}
          higherIsBetter={card.higherIsBetter}
          loading={summaryLoading}
          trendLoading={trendLoading}
        />
      ))}
    </div>
  );
}
