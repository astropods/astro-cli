import { useMemo, useState, type ReactNode } from "react";
import { Check, ChevronDown } from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import { ProgressBar } from "@/components/ui/progress-bar";
import { useBillingUsage, useBillingDailySpend, useBillingInvoices } from "@/api/queries";
import { useBillingSpend, useRefreshBilling } from "@/api/queries/billing";
import { EmptyState, LoadError, RefreshButton, SectionHeader, Unavailable } from "@/components/settings/SettingsShared";
import { formatNumber } from "@/lib/format-utils";
import { ResourceLimitsSection } from "@/components/settings/ResourceLimitsSection";
import { formatMoney, thresholdDollars } from "@/lib/billing-balances";
import { DEFAULT_CURRENCY, METRIC_COMPUTE, METRIC_GATEWAY, PRODUCT_COMPUTE } from "@/lib/billing-provider";
import { formatDayKey, utcDayKey } from "@/lib/date-utils";
import { GRID_PROPS, SeriesTooltip, yAxisProps } from "@/components/activity/chart-chrome";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { BillingInvoice, BillingSpend, BillingUsageRow, DailySpendPoint } from "@/lib/api";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";

interface Period {
  from: string;
  to: string;
}

// ---------------------------------------------------------------------------
// Billing period selection
// ---------------------------------------------------------------------------

// The window is half-open, so labelling `to` would claim a day it never bills.
function periodLabel(period: Period): string {
  const opts = { month: "short", day: "numeric", timeZone: "UTC" } as const;
  const from = new Date(period.from);
  const lastDay = new Date(new Date(period.to).getTime() - 86_400_000);
  if (Number.isNaN(from.getTime()) || Number.isNaN(lastDay.getTime())) return "This period";
  return `${from.toLocaleDateString(undefined, opts)} – ${lastDay.toLocaleDateString(undefined, opts)}`;
}

function samePeriod(a: Period | undefined, b: Period | undefined): boolean {
  return !!a && !!b && a.from === b.from && a.to === b.to;
}

// The open period is the caller's own, so an invoice covering it is dropped.
function pastPeriods(invoices: BillingInvoice[], current: Period | undefined): Period[] {
  const seen = new Set<string>();
  const out: Period[] = [];
  for (const inv of invoices) {
    if (!inv.start_timestamp || !inv.end_timestamp) continue;
    const period = { from: inv.start_timestamp, to: inv.end_timestamp };
    const key = `${period.from}|${period.to}`;
    if (seen.has(key) || samePeriod(period, current)) continue;
    seen.add(key);
    out.push(period);
  }
  return out.sort((a, b) => b.from.localeCompare(a.from));
}

function PeriodPicker({
  periods,
  selected,
  onSelect,
}: {
  periods: Period[];
  selected: Period | undefined;
  onSelect: (period: Period) => void;
}) {
  if (!selected) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        disabled={periods.length < 2}
        className="inline-flex items-center gap-1.5 rounded-sm border border-border px-3 py-1.5 text-body-sm text-foreground hover:bg-muted disabled:cursor-default disabled:hover:bg-transparent"
      >
        {periodLabel(selected)}
        {periods.length > 1 && <ChevronDown className="size-3.5 text-muted-foreground" aria-hidden />}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {periods.map((period) => (
          <DropdownMenuItem key={`${period.from}|${period.to}`} onSelect={() => onSelect(period)}>
            <Check
              className={cn("size-3.5", !samePeriod(period, selected) && "invisible")}
              aria-hidden
            />
            {periodLabel(period)}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

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
          <Skeleton className="h-[200px] w-full" />
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
    for (const [product, amount] of Object.entries(point.by_product ?? {})) {
      if (product === PRODUCT_COMPUTE) computeDollars += amount;
      else modelsDollars += amount;
    }
  }
  return { modelsDollars, computeDollars };
}

// The gateway metric aggregates cost_usd rather than tokens, so only Compute
// has a raw quantity to show under its dollars.
function computeUnitHours(rows: BillingUsageRow[]): number | null {
  let total = 0;
  let sawAny = false;
  for (const row of rows) {
    if (row.billable_metric_name !== METRIC_COMPUTE || row.value == null) continue;
    sawAny = true;
    total += row.value;
  }
  return sawAny ? total : null;
}

// Uses the real current_period_start/end when both are sent (same draft
// invoice server-side). Otherwise approximates start by stepping back one
// calendar month (Metronome bills on a fixed day), clamped for a day-31
// anchor rolling into the next month.
function billingPeriod(periodStart?: string, periodEnd?: string): Period | undefined {
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
// describe the same billing period. `selected` overrides it for the two
// windowed queries; useBillingSpend has no window and always answers for the
// open period.
function usePeriodUsage(account: string, selected?: Period) {
  const {
    data: spendResp,
    isLoading: spendLoading,
    isLoadingError: spendError,
    refetch: refetchSpend,
  } = useBillingSpend(account);
  const currentPeriod = billingPeriod(
    spendResp?.available ? spendResp.data?.current_period_start : undefined,
    spendResp?.available ? spendResp.data?.current_period_end : undefined,
  );
  const period = selected ?? currentPeriod;
  const isCurrentPeriod = !selected || samePeriod(selected, currentPeriod);
  const {
    data: usageResp,
    isLoading: usageLoading,
    isLoadingError: usageError,
    refetch: refetchUsage,
  } = useBillingUsage(account, period, {
    enabled: !!period,
    isCurrentPeriod,
  });
  const {
    data: dailySpendResp,
    isLoading: dailySpendLoading,
    isLoadingError: dailySpendError,
    refetch: refetchDailySpend,
  } = useBillingDailySpend(account, period, {
    enabled: !!period,
    isCurrentPeriod,
  });
  return {
    spendResp,
    usageResp,
    dailySpendResp,
    period,
    currentPeriod,
    isCurrentPeriod,
    isLoading: spendLoading || (!!period && (usageLoading || dailySpendLoading)),
    spendError,
    usageError,
    dailySpendError,
    refetchSpend,
    refetchUsage,
    refetchDailySpend,
  };
}


// The spend query only ever answers for the open period, so a closed one totals
// its own breakdown rather than borrowing a number about today.
function periodTotal(spend: BillingSpend, isCurrentPeriod: boolean, breakdownTotal: number): number {
  if (!isCurrentPeriod) return breakdownTotal;
  return spend.has_usage_spend ? spend.usage_spend : 0;
}

function totalCaption(limitAmount: number | undefined, isCurrentPeriod: boolean, currency: string): string {
  if (limitAmount != null) return `of ${formatMoney(limitAmount, currency)} spend limit`;
  return isCurrentPeriod ? "No spend limit set" : "Closed billing period";
}

function UsageHeader({
  spend,
  dailySpend,
  rows,
  isCurrentPeriod,
}: {
  spend: BillingSpend;
  dailySpend: DailySpendPoint[];
  rows: BillingUsageRow[];
  isCurrentPeriod: boolean;
}) {
  const currency = spend.currency ?? DEFAULT_CURRENCY;
  const { modelsDollars, computeDollars } = splitSpend(dailySpend);
  // The spend query only answers for today, and the limit is a live cap.
  const totalSpend = periodTotal(spend, isCurrentPeriod, modelsDollars + computeDollars);
  const limitAmount = isCurrentPeriod ? thresholdDollars(spend.limit?.amount) : undefined;
  const cuHours = computeUnitHours(rows);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-heading-1 tabular-nums text-foreground">{formatMoney(totalSpend, currency)}</p>
          <p className="text-body-sm text-muted-foreground">
            {totalCaption(limitAmount, isCurrentPeriod, currency)}
          </p>
        </div>
        <div className="flex gap-6">
          <StreamTotal
            metric={METRIC_COMPUTE}
            label="Compute"
            amount={computeDollars}
            currency={currency}
            unit={cuHours != null ? `${formatNumber(cuHours, 2)} CU-hours` : undefined}
          />
          <StreamTotal metric={METRIC_GATEWAY} label="Models" amount={modelsDollars} currency={currency} />
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
export function periodDayKeys(period: Period): string[] {
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
function buildSpendChart(points: DailySpendPoint[], period: Period) {
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
  [METRIC_COMPUTE]: "opacity-100",
  [METRIC_GATEWAY]: "opacity-40",
};

function fillOpacityClassFor(metric: string): string {
  return METRIC_FILL_OPACITY_CLASS[metric] ?? "opacity-100";
}

function StreamTotal({
  metric,
  label,
  amount,
  currency,
  unit,
}: {
  metric: string;
  label: string;
  amount: number;
  currency: string;
  unit?: string;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex items-center gap-1.5">
        <span
          className={`size-2 shrink-0 rounded-full bg-primary ${fillOpacityClassFor(metric)}`}
          aria-hidden
        />
        <p className="text-body-sm text-muted-foreground">{label}</p>
      </div>
      <p className="text-body-sm font-medium tabular-nums text-foreground">
        {formatMoney(amount, currency)}
      </p>
      {unit && <p className="text-body-sm tabular-nums text-faint-foreground">{unit}</p>}
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

const SPEND_SERIES_COLOR = { Spend: "var(--color-primary)" };

function DailySpendChart({
  points,
  period,
  currency,
}: {
  points: DailySpendPoint[];
  period: Period;
  currency: string;
}) {
  // period is a fresh object each render (billingPeriod constructs one), so
  // depend on its primitive bounds rather than its identity, or this never
  // actually memoizes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const days = useMemo(() => buildSpendChart(points, period), [points, period.from, period.to]);
  const yAxis = useMemo(() => yAxisProps((v: number) => formatNumber(v, 2)), []);

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
      <ResponsiveContainer width="100%" height={200}>
        <BarChart data={days} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
          <CartesianGrid {...GRID_PROPS} />
          <XAxis
            dataKey="day"
            interval={0}
            tick={EdgeDateTick}
            axisLine={false}
            tickLine={false}
            tickMargin={8}
          />
          <YAxis {...yAxis} />
          <Tooltip
            cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }}
            content={
              <SeriesTooltip
                colors={SPEND_SERIES_COLOR}
                format={(v) => formatMoney(v, currency)}
                includeZero
              />
            }
          />
          <Bar dataKey="value" name="Spend" fill="var(--color-primary)" radius={[3, 3, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
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
  const models = groupedSpend(rows, METRIC_GATEWAY);
  if (!models) return null;
  return <SpendTable title="Models" rows={models} currency={currency} />;
}

// ---------------------------------------------------------------------------

function Section({ children }: { children: ReactNode }) {
  return <div className="border-b border-border py-6 first:pt-0 last:border-0">{children}</div>;
}

// One availability check for header, chart, and breakdown; they share the
// same three queries, so checking separately in each stacked duplicate notices.
function PeriodSpend({
  usage,
  isCurrentPeriod,
}: {
  usage: ReturnType<typeof usePeriodUsage>;
  isCurrentPeriod: boolean;
}) {
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
  } = usage;

  if (isLoading) {
    return (
      <>
        <Section>
          <HeaderSkeleton />
        </Section>
        <Section>
          <ChartsSkeleton />
        </Section>
      </>
    );
  }
  if (spendError) {
    return (
      <Section>
        <LoadError onRetry={() => refetchSpend()} />
      </Section>
    );
  }
  if (!spendResp?.available || !spendResp.data) return <Unavailable />;

  const spend = spendResp.data;
  const rows = usageResp?.available ? usageResp.data ?? [] : [];
  const dailySpend = dailySpendResp?.available ? dailySpendResp.data ?? [] : [];
  return (
    <>
      <Section>
        <UsageHeader
          spend={spend}
          dailySpend={dailySpend}
          rows={rows}
          isCurrentPeriod={isCurrentPeriod}
        />
      </Section>
      {period ? (
        usageError || dailySpendError ? (
          <Section>
            <LoadError
              message="Couldn't load daily usage."
              onRetry={() => {
                refetchUsage();
                refetchDailySpend();
              }}
            />
          </Section>
        ) : (
          <>
            <Section>
              <DailySpendChart points={dailySpend} period={period} currency={spend.currency ?? DEFAULT_CURRENCY} />
            </Section>
            {groupedSpend(rows, METRIC_GATEWAY) && (
              <Section>
                <SpendBreakdown rows={rows} currency={spend.currency ?? DEFAULT_CURRENCY} />
              </Section>
            )}
          </>
        )
      ) : (
        <Section>
          {/* Header totals hold without a window; a daily series needs one. */}
          <EmptyState message="Daily usage isn't available until the provider reports a billing period." />
        </Section>
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
  const [selected, setSelected] = useState<Period | undefined>(undefined);
  const usage = usePeriodUsage(account, selected);
  const { refresh, isRefreshing } = useRefreshBilling(account);
  const { data: invoicesResp } = useBillingInvoices(account);

  const { currentPeriod, isCurrentPeriod } = usage;
  const periods = currentPeriod
    ? [currentPeriod, ...pastPeriods(invoicesResp?.data ?? [], currentPeriod)]
    : [];

  return (
    <>
      <SectionHeader
        title="Usage"
        subtitle="View your spend distribution."
        action={
          <div className="flex items-center gap-2">
            <PeriodPicker
              periods={periods}
              selected={selected ?? currentPeriod}
              onSelect={setSelected}
            />
            <RefreshButton onRefresh={refresh} busy={isRefreshing} />
          </div>
        }
      />
      <div className="flex flex-col">
        <PeriodSpend usage={usage} isCurrentPeriod={isCurrentPeriod} />
        <Section>
          <ResourceLimitsSection account={account} canRequestIncrease={canRequestIncrease} />
        </Section>
      </div>
    </>
  );
}
