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

const fmtRequests = (v: number) => `${formatCompact(v)} req`;
const fmtTokens = (v: number) => `${formatCompact(v)} tok`;

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

  const changeLabel = showChange && RANGE_LABELS[range] ? `vs. ${RANGE_LABELS[range]}` : undefined;
  const sparklineDates = data?.cost_over_time?.map((d) => d.date);
  const costSparkline = data?.sparklines?.cost ?? data?.cost_over_time?.map((d) => d.models.reduce((s, m) => s + m.cost_usd, 0));

  const shared = { changeLabel, showChange, sparklineDates };

  return (
    <div className="mb-6 grid grid-cols-1 gap-3 @sm:grid-cols-3">
      <MetricCard label="SPEND" value={formatCost(cost)} changePct={data?.change?.cost_pct} sparkline={costSparkline} formatSparkValue={formatCost} {...shared} />
      <MetricCard label="REQUESTS" value={formatCompact(requests)} changePct={data?.change?.requests_pct} sparkline={data?.sparklines?.requests} formatSparkValue={fmtRequests} {...shared} />
      <MetricCard label="TOKENS" value={formatCompact(tokens)} changePct={data?.change?.tokens_pct} sparkline={data?.sparklines?.tokens} formatSparkValue={fmtTokens} {...shared} />
    </div>
  );
}
