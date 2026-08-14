import { useEffect, useMemo, useState } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Legend,
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
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { SpendControls } from "@/components/settings/SpendControls";
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

// Distinct series colors for the usage chart, drawn from the theme palette.
const SERIES_COLORS = [
  "var(--color-indigo-500)",
  "var(--color-sky-500)",
  "var(--color-emerald-500)",
  "var(--color-amber-500)",
  "var(--color-rose-500)",
  "var(--color-violet-500)",
  "var(--color-teal-500)",
  "var(--color-orange-500)",
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
  const metricSeen = new Set<string>();
  const byDate = new Map<string, Record<string, number | string>>();

  for (const row of rows) {
    const metric = row.billable_metric_name ?? "Usage";
    if (!metricSeen.has(metric)) {
      metricSeen.add(metric);
      metrics.push(metric);
    }
    const ts = row.start_timestamp ?? "";
    let bucket = byDate.get(ts);
    if (!bucket) {
      bucket = { ts, label: formatDay(ts) };
      byDate.set(ts, bucket);
    }
    bucket[metric] = ((bucket[metric] as number) ?? 0) + (row.value ?? 0);
  }

  const data = Array.from(byDate.values()).sort((a, b) =>
    String(a.ts).localeCompare(String(b.ts)),
  );
  const totals = metrics.map((metric) => ({
    metric,
    total: rows
      .filter((r) => (r.billable_metric_name ?? "Usage") === metric)
      .reduce((sum, r) => sum + (r.value ?? 0), 0),
  }));
  return { metrics, data, totals };
}

function formatDay(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function UsageTab({ account }: { account: string }) {
  const { data, isLoading } = useBillingUsage(account);
  const rows = data?.data ?? [];
  const chart = useMemo(() => buildUsageChart(rows), [rows]);

  if (isLoading) return <TabLoading />;
  if (!data?.available) return <Unavailable />;
  if (!rows.length) return <EmptyState message="No usage recorded for this period." />;

  return (
    <div className="flex flex-col gap-6">
      <div className="rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={chart.data} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
            <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--color-border)" strokeOpacity={0.5} />
            <XAxis
              dataKey="label"
              tick={{ fill: "var(--color-muted-foreground)", fontSize: 11, fontFamily: "var(--font-mono)" }}
              axisLine={false}
              tickLine={false}
              tickMargin={8}
              minTickGap={24}
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
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: 8,
                fontSize: 12,
              }}
              formatter={(value) => formatNumber(Number(value ?? 0), 2)}
            />
            <Legend wrapperStyle={{ fontSize: 12 }} />
            {chart.metrics.map((metric, i) => (
              <Bar
                key={metric}
                dataKey={metric}
                stackId="usage"
                fill={SERIES_COLORS[i % SERIES_COLORS.length]}
                fillOpacity={0.85}
                radius={i === chart.metrics.length - 1 ? [3, 3, 0, 0] : [0, 0, 0, 0]}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="flex flex-col gap-3">
        <h3 className="text-heading-4 text-foreground">Totals this period</h3>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Metric</TableHead>
              <TableHead className="text-right">Total</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {chart.totals.map(({ metric, total }) => (
              <TableRow key={metric}>
                <TableCell className="font-medium">{metric}</TableCell>
                <TableCell className="text-right tabular-nums">{formatNumber(total, 2)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
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
        </DialogHeader>
        {isLoading && <TabLoading />}
        {error && <EmptyState message="Couldn't load the invoice PDF." />}
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
