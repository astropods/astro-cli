import { useEffect, useMemo, useState } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { Loader2 } from "lucide-react";
import {
  useAccountUsage,
  useBillingUsage,
  useBillingInvoices,
  useBillingBalances,
  useInvoicePdf,
  useQuotaIncreaseRequests,
} from "@/api/queries";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { SpendControls } from "@/components/settings/SpendControls";
import { PlanSummary } from "@/components/settings/PlanSummary";
import { PaymentMethod } from "@/components/settings/PaymentMethod";
import { Button } from "@/components/ui/button";
import { ProgressBar } from "@/components/ui/progress-bar";
import {
  RequestIncreaseDialog,
  formatNumber,
  meterMeta,
} from "@/components/RequestIncreaseDialog";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { cn } from "@/lib/utils";
import type { BillingInvoice, BillingRecord, BillingUsageRow, UsageMeter } from "@/lib/api";
import { formatCreditAmount, toBalanceRow } from "@/lib/billing-balances";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type TabKey = "usage" | "invoices" | "balances" | "quotas";

const TABS: { key: TabKey; label: string }[] = [
  { key: "usage", label: "Usage" },
  { key: "invoices", label: "Invoices" },
  { key: "balances", label: "Credits & Commits" },
  { key: "quotas", label: "Quotas" },
];

const statusBadgeColor: Record<string, StatusBadgeColor> = {
  pending: "warning",
  approved: "success",
  denied: "error",
};

// ---------------------------------------------------------------------------
// Credits and commits
// ---------------------------------------------------------------------------

function BalanceTable({ rows, emptyMessage }: { rows: BillingRecord[]; emptyMessage: string }) {
  const projected = useMemo(() => rows.map(toBalanceRow), [rows]);
  if (!projected.length) {
    return <EmptyState message={emptyMessage} />;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead className="text-right">Granted</TableHead>
          <TableHead className="text-right">Remaining</TableHead>
          <TableHead className="text-right">Expires</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {projected.map((row, i) => (
          <TableRow key={i}>
            <TableCell className="font-medium text-foreground">{row.name}</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatCreditAmount(row.granted, row.creditType)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatCreditAmount(row.remaining, row.creditType)}
            </TableCell>
            <TableCell className="whitespace-nowrap text-right">
              {row.expires ? formatInvoiceDate(row.expires) : "\u2014"}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-border bg-surface px-5 py-4">
      <p className="text-body-sm text-muted-foreground">{message}</p>
    </div>
  );
}

function Unavailable() {
  return (
    <EmptyState message="Billing isn't available for this account yet. Data appears here once billing is enabled." />
  );
}

function TabLoading() {
  return (
    <div className="flex items-center gap-2 py-16 text-body-sm text-muted-foreground">
      <Loader2 size={14} className="animate-spin" />
      Loading...
    </div>
  );
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

function buildUsageChart(rows: BillingUsageRow[]) {
  const metrics: string[] = [];
  const byMetric = new Map<string, Map<string, { ts: string; label: string; day: string; value: number }>>();

  for (const row of rows) {
    const metric = row.billable_metric_name ?? "Usage";
    if (!byMetric.has(metric)) {
      metrics.push(metric);
      byMetric.set(metric, new Map());
    }
    const ts = row.start_timestamp ?? "";
    const days = byMetric.get(metric)!;
    const point = days.get(ts) ?? { ts, label: formatDay(ts), day: formatDayTick(ts), value: 0 };
    point.value += row.value ?? 0;
    days.set(ts, point);
  }

  return {
    series: metrics.map((metric) => {
      const data = Array.from(byMetric.get(metric)!.values()).sort((a, b) => a.ts.localeCompare(b.ts));
      return { metric, data, total: data.reduce((sum, p) => sum + p.value, 0) };
    }),
  };
}

const METRIC_UNITS: Record<string, { unit: string; money: boolean }> = {
  "Compute Units": { unit: "CU-hours", money: false },
  "LLM Usage": { unit: "USD", money: true },
};

function formatMetricTotal(metric: string, total: number): string {
  const spec = METRIC_UNITS[metric];
  if (!spec) return formatNumber(total, 2);
  return spec.money ? `$${formatNumber(total, 2)}` : `${formatNumber(total, 2)} ${spec.unit}`;
}

function formatDay(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function formatDayTick(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : String(d.getUTCDate());
}

function UsageTab({ account }: { account: string }) {
  const { data, isLoading } = useBillingUsage(account);
  const rows = data?.data ?? [];
  const chart = useMemo(() => buildUsageChart(rows), [rows]);

  if (isLoading) return <TabLoading />;
  if (!data?.available) return <Unavailable />;
  if (!rows.length) return <EmptyState message="No usage recorded for this period." />;

  return (
    <div className="flex flex-col gap-8">
      {chart.series.map(({ metric, data: days, total }) => (
        <div key={metric} className="flex flex-col gap-3">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h3 className="text-heading-4 text-foreground">{metric}</h3>
            <span className="text-body-sm text-muted-foreground tabular-nums">
              {formatMetricTotal(metric, total)} this period
            </span>
          </div>
          <div className="rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
            <ResponsiveContainer width="100%" height={200}>
              <BarChart data={days} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
                <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--color-border)" strokeOpacity={0.5} />
                <XAxis
                  dataKey="day"
                  interval={0}
                  tick={{ fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" }}
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
                  labelFormatter={(_label, payload) => payload?.[0]?.payload?.label ?? ""}
                  formatter={(value) => [formatMetricTotal(metric, Number(value ?? 0)), metric]}
                />
                <Bar dataKey="value" name={metric} fill="var(--color-primary)" radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Invoices / Balances
// ---------------------------------------------------------------------------

function invoiceStatusColor(status?: string): StatusBadgeColor {
  switch ((status ?? "").toUpperCase()) {
    case "FINALIZED":
    case "PAID":
      return "success";
    case "VOID":
      return "error";
    default:
      return "warning";
  }
}

function formatInvoiceDate(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

function invoicePeriod(inv: BillingInvoice): string {
  if (inv.start_timestamp && inv.end_timestamp) {
    return `${formatInvoiceDate(inv.start_timestamp)} – ${formatInvoiceDate(inv.end_timestamp)}`;
  }
  return formatInvoiceDate(inv.issued_at) || inv.id || "Invoice";
}

function InvoicePdfModal({
  account,
  invoice,
  onClose,
}: {
  account: string;
  invoice: BillingInvoice | null;
  onClose: () => void;
}) {
  const open = !!invoice;
  const { data: blob, isLoading, error } = useInvoicePdf(account, invoice?.id ?? "", open);
  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!blob) {
      setUrl(null);
      return;
    }
    const objectUrl = URL.createObjectURL(blob);
    setUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [blob]);

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            Invoice
            {invoice && <span className="text-muted-foreground font-normal">· {invoicePeriod(invoice)}</span>}
            {invoice && (
              <StatusBadge color={invoiceStatusColor(invoice.status)}>{invoice.status ?? "—"}</StatusBadge>
            )}
          </DialogTitle>
          <DialogDescription>
            {invoice ? `Invoice ${invoicePeriod(invoice)}, rendered as a PDF.` : "Invoice PDF."}
          </DialogDescription>
        </DialogHeader>
        {isLoading && <TabLoading />}
        {error && <EmptyState message="No PDF is available for this invoice." />}
        {url && (
          <iframe
            src={url}
            title="Invoice PDF"
            className="h-[75vh] w-full rounded-md border border-border bg-surface"
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function InvoicesTab({ account }: { account: string }) {
  const { data, isLoading } = useBillingInvoices(account);
  const [selected, setSelected] = useState<BillingInvoice | null>(null);
  const invoices = data?.data ?? [];

  if (isLoading) return <TabLoading />;
  if (!data?.available) return <Unavailable />;
  if (!invoices.length) return <EmptyState message="No invoices yet." />;

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Period</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Total</TableHead>
            <TableHead className="w-[80px]" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {invoices.map((inv, i) => {
            // Only finalized invoices have a downloadable PDF; drafts 404.
            const hasPdf = (inv.status ?? "").toUpperCase() === "FINALIZED" && !!inv.id;
            return (
              <TableRow
                key={inv.id ?? i}
                className={cn(hasPdf && "cursor-pointer")}
                onClick={() => hasPdf && setSelected(inv)}
              >
                <TableCell className="font-medium">{invoicePeriod(inv)}</TableCell>
                <TableCell>
                  <StatusBadge color={invoiceStatusColor(inv.status)}>{inv.status ?? "—"}</StatusBadge>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatCreditAmount(inv.total, inv.credit_type?.name)}
                </TableCell>
                <TableCell className="text-right text-body-sm">
                  {hasPdf ? (
                    <span className="text-foreground-accent">View invoice</span>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      <InvoicePdfModal account={account} invoice={selected} onClose={() => setSelected(null)} />
    </>
  );
}

function BalancesTab({ account }: { account: string }) {
  const { data, isLoading } = useBillingBalances(account);
  if (isLoading) return <TabLoading />;
  if (!data?.available) return <Unavailable />;
  const credits = data.data?.credits ?? [];
  const commits = data.data?.commits ?? [];
  if (!credits.length && !commits.length) {
    return <EmptyState message="No credits or commits yet." />;
  }
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <h3 className="text-heading-4 text-foreground">Credits</h3>
        <BalanceTable rows={credits} emptyMessage="No credits yet." />
      </div>
      <div className="flex flex-col gap-3">
        <h3 className="text-heading-4 text-foreground">Commits</h3>
        <BalanceTable rows={commits} emptyMessage="No commits yet." />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Quotas
// ---------------------------------------------------------------------------

function QuotaRequestsTable({ account }: { account: string }) {
  const { data, isLoading } = useQuotaIncreaseRequests(account);

  if (isLoading || !data?.requests?.length) {
    return <p className="text-body-sm text-muted-foreground">No quota increase requests.</p>;
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Feature</TableHead>
          <TableHead>Reason</TableHead>
          <TableHead className="text-right">Requested</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Date</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {data.requests.map((req) => (
          <TableRow key={req.id}>
            <TableCell className="font-medium">
              {meterMeta[req.feature_key]?.label ?? req.feature_key}
            </TableCell>
            <TableCell className="text-muted-foreground max-w-[200px] truncate">
              {req.reason}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {req.requested_amount != null ? formatNumber(req.requested_amount, 0) : "—"}
            </TableCell>
            <TableCell>
              <StatusBadge color={statusBadgeColor[req.status] ?? "muted"}>
                {req.status}
              </StatusBadge>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {new Date(req.created_at).toLocaleDateString()}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function QuotaUsageTable({ meters }: { meters: Record<string, UsageMeter> }) {
  const keys = Object.keys(meters);
  if (!keys.length) {
    return <p className="text-body-sm text-muted-foreground">No usage data available.</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Feature</TableHead>
          <TableHead className="text-right">Usage</TableHead>
          <TableHead className="text-right">Limit</TableHead>
          <TableHead className="w-[180px]">Utilization</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {keys.map((key) => {
          const meter = meters[key]!;
          const info = meterMeta[key];
          const label = info?.label ?? key;
          const decimals = info?.decimals ?? 0;
          const hasQuota = meter.quota != null;
          const pct = hasQuota ? Math.min((meter.usage / meter.quota!) * 100, 100) : 0;
          const isHigh = pct >= 90;
          const isMedium = pct >= 75 && !isHigh;
          return (
            <TableRow key={key}>
              <TableCell className="font-medium">
                {label}
                {info?.unit ? <span className="text-muted-foreground"> ({info.unit})</span> : null}
              </TableCell>
              <TableCell className="text-right tabular-nums">{formatNumber(meter.usage, decimals)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {hasQuota ? formatNumber(meter.quota!, 0) : "Unlimited"}
              </TableCell>
              <TableCell>
                {hasQuota ? (
                  <ProgressBar
                    aria-label={`${label} utilization`}
                    aria-valuetext={`${Math.round(pct)}% used`}
                    value={pct}
                    tone={
                      isHigh
                        ? "destructive"
                        : isMedium
                          ? "warning"
                          : "primary"
                    }
                  />
                ) : (
                  <span className="text-body-sm text-muted-foreground">—</span>
                )}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function QuotasTab({ account, canRequestIncrease }: { account: string; canRequestIncrease: boolean }) {
  const { data: usage, isLoading } = useAccountUsage(account);
  const [dialogOpen, setDialogOpen] = useState(false);
  const meters = usage?.meters ?? {};
  const hasMeters = Object.keys(meters).length > 0;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h3 className="text-heading-4 text-foreground">Current usage</h3>
          {canRequestIncrease && (
            <Button size="sm" variant="outline" disabled={!hasMeters} onClick={() => setDialogOpen(true)}>
              Request increase
            </Button>
          )}
        </div>
        {isLoading ? <TabLoading /> : <QuotaUsageTable meters={meters} />}
      </div>

      <div className="flex flex-col gap-3">
        <h3 className="text-heading-4 text-foreground">Quota increase requests</h3>
        <QuotaRequestsTable account={account} />
      </div>

      {canRequestIncrease && hasMeters && (
        <RequestIncreaseDialog
          account={account}
          meters={meters}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------

export function BillingView({
  account,
  canRequestIncrease = true,
}: {
  account: string;
  canRequestIncrease?: boolean;
}) {
  const [activeTab, setActiveTab] = useState<TabKey>("usage");

  return (
    <>
      <SectionHeader
        title="Billing"
        subtitle="Usage, invoices, credits, and quotas for your account"
      />

      <PlanSummary account={account} />

      <PaymentMethod account={account} />

      <SpendControls account={account} />

      <div className="flex flex-col gap-4">
        <div className="flex gap-1 border-b border-border">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                "-mb-px border-b-2 px-3 py-2 text-body-sm transition-colors",
                activeTab === tab.key
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {activeTab === "usage" && <UsageTab account={account} />}
        {activeTab === "invoices" && <InvoicesTab account={account} />}
        {activeTab === "balances" && <BalancesTab account={account} />}
        {activeTab === "quotas" && (
          <QuotasTab account={account} canRequestIncrease={canRequestIncrease} />
        )}
      </div>
    </>
  );
}
