import type { AccountObservabilitySummaryResponse } from "@/lib/api";
import { formatCost, formatCompact } from "@/lib/format-utils";
import { MetricCard } from "@/components/MetricCard";
import type { ActivityRange } from "./ranges";

interface StatCardsProps {
  data?: AccountObservabilitySummaryResponse | null;
  showChange: boolean;
  range: ActivityRange;
}

const RANGE_LABELS: Record<ActivityRange, string> = {
  "7d": "last 7 days",
  "14d": "last 14 days",
  "30d": "last 30 days",
  "all": "",
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

  // All-time has no meaningful prior window — skip the change badge entirely
  // (don't even reserve its slot) so the card sheds that whole region. The
  // height-stable opacity-fade behavior only kicks in between bounded ranges.
  const changeLabel = showChange && RANGE_LABELS[range] ? `vs. ${RANGE_LABELS[range]}` : undefined;
  // changePct stays passed even on the all-time view (where showChange is
  // false) so MetricCard's `hasChangeApi` branch stays true → the label keeps
  // its mb-2 spacing and the card height matches the bounded-range cards.
  // The badge itself is inline + opacity-faded, so it doesn't reserve height.
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
