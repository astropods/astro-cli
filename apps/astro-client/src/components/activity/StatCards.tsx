import type { AccountObservabilitySummaryResponse } from "@/lib/api";
import { formatCost, formatCompact } from "@/lib/format-utils";
import { MetricCard } from "@/components/MetricCard";
import type { ActivityRange } from "./ranges";

interface StatCardsProps {
  data?: Pick<AccountObservabilitySummaryResponse, "totals" | "change"> | null;
  showChange: boolean;
  range: ActivityRange;
}

const RANGE_LABELS: Record<ActivityRange, string> = {
  "7d": "last 7 days",
  "14d": "last 14 days",
  "30d": "last 30 days",
  "90d": "last 90 days",
};

// `loading` prop intentionally dropped: MetricCard renders an animate-pulse
// SkeletonBar in place of the value when loading=true, which was the visible
// flash on /insights refresh. placeholderData on the underlying queries keeps
// the previous window's data on screen during refetches; on a cold load the
// cards briefly show "0" / "—" before data lands, which is preferable to a
// shimmer.
export function StatCards({ data, showChange, range }: StatCardsProps) {
  const cost = data?.totals?.cost_usd ?? 0;
  const requests = data?.totals?.requests ?? 0;
  // Prefer total_tokens (new source of truth); fall back to input+output for
  // safety while clients catch up to the new contract.
  const tokens = data?.totals?.total_tokens
    ?? (data?.totals?.input_tokens ?? 0) + (data?.totals?.output_tokens ?? 0);

  // Every UI range has a usable prior window: the server returns all-time
  // trailing data, so change-pct is derivable client-side for every
  // selectable range (the prior window of even the longest range, 90d,
  // sits comfortably inside the all-time payload).
  const changeLabel = showChange && RANGE_LABELS[range] ? `vs. ${RANGE_LABELS[range]}` : undefined;
  // showTrend={false} keeps the TrendIndicator fallback from rendering "— —".
  const shared = { changeLabel, showChange, showTrend: false };

  return (
    <div className="mb-6 grid grid-cols-1 gap-3 @sm:grid-cols-3">
      <MetricCard label="SPEND" value={formatCost(cost)} changePct={data?.change?.cost_pct ?? null} {...shared} />
      <MetricCard label="REQUESTS" value={formatCompact(requests)} changePct={data?.change?.requests_pct ?? null} {...shared} />
      <MetricCard label="TOKENS" value={formatCompact(tokens)} changePct={data?.change?.tokens_pct ?? null} {...shared} />
    </div>
  );
}
