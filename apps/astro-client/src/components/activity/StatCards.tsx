import type { AccountObservabilitySummaryResponse } from "@/lib/api";
import { formatCost, formatCompact } from "@/lib/format-utils";
import { MetricCard } from "@/components/MetricCard";
import type { ActivityRange } from "./ranges";

interface StatCardsProps {
  data?: AccountObservabilitySummaryResponse | null;
  loading: boolean;
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

export function StatCards({ data, loading, showChange, range }: StatCardsProps) {
  const cost = data?.totals?.cost_usd ?? 0;
  const requests = data?.totals?.requests ?? 0;
  const tokens = (data?.totals?.input_tokens ?? 0) + (data?.totals?.output_tokens ?? 0);

  const changeLabel = showChange && RANGE_LABELS[range] ? `vs. ${RANGE_LABELS[range]}` : undefined;
  const sparklineDates = data?.cost_over_time?.map((d) => d.date);
  const costSparkline = data?.sparklines?.cost ?? data?.cost_over_time?.map((d) => d.models.reduce((s, m) => s + m.cost_usd, 0));

  const shared = { changeLabel, showChange, loading, sparklineDates };

  return (
    <div className="mb-6 grid grid-cols-1 gap-3 @sm:grid-cols-3">
      <MetricCard label="SPEND" value={formatCost(cost)} changePct={data?.change?.cost_pct} sparkline={costSparkline} formatSparkValue={formatCost} {...shared} />
      <MetricCard label="REQUESTS" value={formatCompact(requests)} changePct={data?.change?.requests_pct} sparkline={data?.sparklines?.requests} formatSparkValue={fmtRequests} {...shared} />
      <MetricCard label="TOKENS" value={formatCompact(tokens)} changePct={data?.change?.tokens_pct} sparkline={data?.sparklines?.tokens} formatSparkValue={fmtTokens} {...shared} />
    </div>
  );
}
