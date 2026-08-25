import { useMemo, type ReactNode } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Text,
} from "recharts";
import type { XAxisTickContentProps } from "recharts/types/util/types";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ProgressBar } from "@/components/ui/progress-bar";
import { useBillingUsage, useBillingDailySpend } from "@/api/queries";
import { useBillingSpend } from "@/api/queries/billing";
import { EmptyState, LoadError, SectionHeader, Unavailable } from "@/components/settings/SettingsShared";
import { formatNumber } from "@/lib/format-utils";
import { ResourceLimitsSection } from "@/components/settings/ResourceLimitsSection";
import { formatMoney, thresholdDollars } from "@/lib/billing-balances";
import { formatDayKey, utcDayKey } from "@/lib/date-utils";
import type { BillingSpend, BillingUsageRow, DailySpendPoint } from "@/lib/api";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";

// ---------------------------------------------------------------------------
// Header: total spend against the account's own limit, split by stream.
// ---------------------------------------------------------------------------

function HeaderSkeleton() {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-9 w-40" />
          <Skeleton className="h-4 w-32" />
        </div>
        <div className="flex gap-6">
          <Skeleton className="h-9 w-24" />
          <Skeleton className="h-9 w-24" />
        </div>
      </div>
      <Skeleton className="h-1.5 w-full" />
    </div>
  );
}

function ChartsSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      {[0, 1].map((i) => (
        <div key={i} className="flex flex-col gap-3">
          <div className="flex items-baseline justify-between gap-2">
            <Skeleton className="h-5 w-32" />
            <Skeleton className="h-4 w-28" />
          </div>
          <Card className="p-5">
            <Skeleton className="h-[200px] w-full" />
          </Card>
        </div>
      ))}
    </div>
  );
}

// Reads each stream's own rated total straight from the daily breakdown's
// by_product, rather than subtracting Models from the period total to get
// Compute: that subtraction could only ever be as exact as the assumption
// that nothing else shares the total, and the breakdown now carries a real
// per-product figure for both streams instead of one and a leftover.
function splitSpend(points: DailySpendPoint[]) {
  let modelsDollars = 0;
  let computeDollars = 0;
  for (const point of points) {
    const byProduct = point.by_product ?? {};
    modelsDollars += byProduct["LLM Usage"] ?? 0;
    computeDollars += byProduct["Compute Units"] ?? 0;
  }
  return { modelsDollars, computeDollars };
}

// Uses the real current_period_start/end when both are sent (same draft
// invoice server-side). Otherwise approximates start by stepping back one
// calendar month (Metronome bills on a fixed day), clamped for a day-31
// anchor rolling into the next month.
function billingPeriod(periodStart?: string, periodEnd?: string): { from: string; to: string } | undefined {
  if (!periodEnd) return undefined;
  const end = new Date(periodEnd);
  if (Number.isNaN(end.getTime())) return undefined;

  if (periodStart) {
    const start = new Date(periodStart);
    if (!Number.isNaN(start.getTime())) return { from: start.toISOString(), to: end.toISOString() };
  }

  const start = new Date(end);
  start.setUTCMonth(start.getUTCMonth() - 1);
  // setUTCMonth rolls a day-31 anchor into the next month; clamp it back.
  if (start.getUTCDate() !== end.getUTCDate()) start.setUTCDate(0);
  return { from: start.toISOString(), to: end.toISOString() };
}

// Spend, usage, and daily spend share one window so the header and charts
// describe the same billing period.
function usePeriodUsage(account: string) {
  const {
    data: spendResp,
    isLoading: spendLoading,
    isLoadingError: spendError,
    refetch: refetchSpend,
  } = useBillingSpend(account);
  const period = billingPeriod(
    spendResp?.available ? spendResp.data?.current_period_start : undefined,
    spendResp?.available ? spendResp.data?.current_period_end : undefined,
  );
  const {
    data: usageResp,
    isLoading: usageLoading,
    isLoadingError: usageError,
    refetch: refetchUsage,
  } = useBillingUsage(account, period, {
    enabled: !!period,
  });
  const {
    data: dailySpendResp,
    isLoading: dailySpendLoading,
    isLoadingError: dailySpendError,
    refetch: refetchDailySpend,
  } = useBillingDailySpend(account, period, {
    enabled: !!period,
  });
  return {
    spendResp,
    usageResp,
    dailySpendResp,
    period,
    isLoading: spendLoading || (!!period && (usageLoading || dailySpendLoading)),
    spendError,
    usageError,
    dailySpendError,
    refetchSpend,
    refetchUsage,
    refetchDailySpend,
  };
}

function UsageHeader({ spend, dailySpend }: { spend: BillingSpend; dailySpend: DailySpendPoint[] }) {
  const currency = spend.currency ?? "USD";
  const totalSpend = spend.has_usage_spend ? spend.usage_spend : 0;
  const limitAmount = thresholdDollars(spend.limit?.amount);
  const { modelsDollars, computeDollars } = splitSpend(dailySpend);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-heading-1 tabular-nums text-foreground">{formatMoney(totalSpend, currency)}</p>
          <p className="text-body-sm text-muted-foreground">
            {limitAmount != null ? `of ${formatMoney(limitAmount, currency)} spend limit` : "No spend limit set"}
          </p>
        </div>
        <div className="flex gap-6">
          <StreamTotal metric="Compute Units" label="Compute" amount={computeDollars} currency={currency} />
          <StreamTotal metric="LLM Usage" label="Models" amount={modelsDollars} currency={currency} />
        </div>
      </div>
      {limitAmount != null && (
        <ProgressBar aria-label="Total spend" value={totalSpend} max={limitAmount} tone="primary" />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Daily spend chart
// ---------------------------------------------------------------------------

function startOfTomorrowUTC(): Date {
  const now = new Date();
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1));
}

// Every calendar day in [period.from, period.to), oldest first, clamped to
// today: period.to is often still in the future (Metronome's draft invoice
// carries the scheduled cycle end, not "now"), and a day that hasn't
// happened yet has no rows to zero-fill or a real date to label the axis with.
// Exported for direct unit testing: Recharts' rendered axis ticks aren't
// reliably queryable in jsdom, so the date math is tested as a pure function.
export function periodDayKeys(period: { from: string; to: string }): string[] {
  const keys: string[] = [];
  const to = new Date(Math.min(new Date(period.to).getTime(), startOfTomorrowUTC().getTime()));
  for (const d = new Date(period.from); d < to; d.setUTCDate(d.getUTCDate() + 1)) {
    keys.push(utcDayKey(d));
  }
  return keys;
}

// Zero-filled per calendar day across the whole period, so a sparse period
// doesn't shrink the axis to however many days actually have a point.
// Points outside the window are dropped, not appended, so a stray point
// can't widen the axis past the period it belongs to.
function buildSpendChart(points: DailySpendPoint[], period: { from: string; to: string }) {
  const dayKeys = periodDayKeys(period);
  const byDay = new Map(dayKeys.map((key) => [key, { ts: key, day: formatDayKey(key), value: 0 }]));

  for (const point of points) {
    const at = new Date(point.day);
    if (Number.isNaN(at.getTime())) continue;
    const bucket = byDay.get(utcDayKey(at));
    if (bucket) bucket.value += point.amount ?? 0;
  }

  return Array.from(byDay.values()).sort((a, b) => a.ts.localeCompare(b.ts));
}

// The header's Compute/Models dots are the same primary token at different
// opacity; kept as a Tailwind class since the swatch is real DOM, not a
// recharts prop.
const METRIC_FILL_OPACITY_CLASS: Record<string, string> = {
  "Compute Units": "opacity-100",
  "LLM Usage": "opacity-40",
};

function fillOpacityClassFor(metric: string): string {
  return METRIC_FILL_OPACITY_CLASS[metric] ?? "opacity-100";
}

function StreamTotal({
  metric,
  label,
  amount,
  currency,
}: {
  metric: string;
  label: string;
  amount: number;
  currency: string;
}) {
  return (
    <div className="flex items-center gap-2">
      <span
        className={`size-2 rounded-full bg-primary ${fillOpacityClassFor(metric)}`}
        aria-hidden
      />
      <div>
        <p className="text-body-sm text-muted-foreground">{label}</p>
        <p className="text-body-sm font-medium tabular-nums text-foreground">
          {formatMoney(amount, currency)}
        </p>
      </div>
    </div>
  );
}

// Only the first and last bar get a date label; denser overlaps or forces
// rotation, and the tooltip covers the rest. interval={0} keeps every tick
// live so visibleTicksCount equals the bar count.
function EdgeDateTick({ index, visibleTicksCount, payload, x, y, verticalAnchor }: XAxisTickContentProps) {
  if (index !== 0 && index !== visibleTicksCount - 1) return null;
  return (
    <Text
      x={x}
      y={y}
      textAnchor={index === 0 ? "start" : "end"}
      verticalAnchor={verticalAnchor}
      fill="var(--color-muted-foreground)"
      fontSize={11}
      fontFamily="var(--font-mono)"
    >
      {payload.value}
    </Text>
  );
}

function DailySpendChart({
  points,
  period,
  currency,
}: {
  points: DailySpendPoint[];
  period: { from: string; to: string };
  currency: string;
}) {
  // period is a fresh object each render (billingPeriod constructs one), so
  // depend on its primitive bounds rather than its identity, or this never
  // actually memoizes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const days = useMemo(() => buildSpendChart(points, period), [points, period.from, period.to]);

  if (!points.length) return <EmptyState message="No usage recorded for this period." />;

  const total = days.reduce((sum, d) => sum + d.value, 0);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-heading-4 text-foreground">Spend</h3>
        <span className="text-body-sm text-muted-foreground tabular-nums">
          {formatMoney(total, currency)} this period
        </span>
      </div>
      <Card className="p-5">
        <ResponsiveContainer width="100%" height={200}>
          <BarChart data={days} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
            <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--color-border)" strokeOpacity={0.5} />
            <XAxis
              dataKey="day"
              interval={0}
              tick={EdgeDateTick}
              axisLine={false}
              tickLine={false}
              tickMargin={8}
            />
            <YAxis
              tickFormatter={(v: number) => formatNumber(v, 2)}
              tick={{ fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" }}
              axisLine={false}
              tickLine={false}
              tickMargin={4}
              width={56}
            />
            <Tooltip
              cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }}
              contentStyle={{
                background: "var(--color-popover)",
                border: "1px solid var(--color-border)",
                borderRadius: 8,
                fontSize: 12,
                color: "var(--color-popover-foreground)",
              }}
              labelFormatter={(_label, payload) => payload?.[0]?.payload?.day ?? ""}
              formatter={(value) => [formatMoney(Number(value ?? 0), currency), "Spend"]}
            />
            <Bar dataKey="value" name="Spend" fill="var(--color-primary)" radius={[3, 3, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </Card>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Agents / Models spend, grouped from the billing provider's own breakdown
// ---------------------------------------------------------------------------

// `groups` keys on the dimension's own values (e.g. a model name; see
// Metronome's UsageListResponse). Reads empty until astro-server requests
// this grouping — nothing here fabricates an unrated number.
function groupedSpend(rows: BillingUsageRow[], metricName: string): { label: string; amount: number }[] | null {
  const totals = new Map<string, number>();
  let sawAny = false;
  for (const row of rows) {
    if (row.billable_metric_name !== metricName || !row.groups) continue;
    for (const [label, amount] of Object.entries(row.groups)) {
      if (amount == null) continue;
      sawAny = true;
      totals.set(label, (totals.get(label) ?? 0) + amount);
    }
  }
  if (!sawAny) return null;
  return Array.from(totals, ([label, amount]) => ({ label, amount })).sort((a, b) => b.amount - a.amount);
}

function SpendTable({
  title,
  rows,
  currency,
}: {
  title: string;
  rows: { label: string; amount: number }[];
  currency: string;
}) {
  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-heading-4 text-foreground">{title}</h3>
      <Table>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.label}>
              <TableCell className="font-medium">{row.label}</TableCell>
              <TableCell className="text-right tabular-nums">{formatMoney(row.amount, currency)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

// Per-agent spend isn't built: grouping compute by agent_name yields
// cu_hours, and pricing those into dollars is a step astro-server doesn't
// do yet. Add it here, next to Models, once that lands.
function SpendBreakdown({ rows, currency }: { rows: BillingUsageRow[]; currency: string }) {
  const models = groupedSpend(rows, "LLM Usage");
  if (!models) return null;
  return <SpendTable title="Models" rows={models} currency={currency} />;
}

// ---------------------------------------------------------------------------

function PeriodSection({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-col gap-3">
      <h2 className="text-heading-4 text-foreground">This billing period</h2>
      <Card className="p-5">{children}</Card>
    </div>
  );
}

// One availability check for header, chart, and breakdown; they share the
// same three queries, so checking separately in each stacked duplicate notices.
function PeriodSpend({ account }: { account: string }) {
  const {
    spendResp,
    usageResp,
    dailySpendResp,
    period,
    isLoading,
    spendError,
    usageError,
    dailySpendError,
    refetchSpend,
    refetchUsage,
    refetchDailySpend,
  } = usePeriodUsage(account);

  if (isLoading) {
    return (
      <>
        <PeriodSection>
          <HeaderSkeleton />
        </PeriodSection>
        <ChartsSkeleton />
      </>
    );
  }
  if (spendError) {
    return (
      <PeriodSection>
        <LoadError onRetry={() => refetchSpend()} />
      </PeriodSection>
    );
  }
  if (!spendResp?.available || !spendResp.data) return <Unavailable />;

  const spend = spendResp.data;
  const rows = usageResp?.available ? usageResp.data ?? [] : [];
  const dailySpend = dailySpendResp?.available ? dailySpendResp.data ?? [] : [];
  return (
    <>
      <PeriodSection>
        <UsageHeader spend={spend} dailySpend={dailySpend} />
      </PeriodSection>
      {period ? (
        usageError || dailySpendError ? (
          <LoadError
            message="Couldn't load daily usage."
            onRetry={() => {
              refetchUsage();
              refetchDailySpend();
            }}
          />
        ) : (
          <>
            <DailySpendChart points={dailySpend} period={period} currency={spend.currency ?? "USD"} />
            <SpendBreakdown rows={rows} currency={spend.currency ?? "USD"} />
          </>
        )
      ) : (
        // Header totals hold without a window; a daily series needs one.
        <EmptyState message="Daily usage isn't available until the provider reports a billing period." />
      )}
    </>
  );
}

export function UsageView({
  account,
  canRequestIncrease = true,
}: {
  account: string;
  canRequestIncrease?: boolean;
}) {
  return (
    <>
      <SectionHeader title="Usage" subtitle="View your spend distribution." />
      <div className="flex flex-col gap-8">
        <PeriodSpend account={account} />
        <ResourceLimitsSection account={account} canRequestIncrease={canRequestIncrease} />
      </div>
    </>
  );
}
