import { useState } from "react";
import { Loader2, Info } from "lucide-react";
import { useAccountUsage, useQuotaIncreaseRequests } from "@/api/queries";
import { SectionHeader } from "@/components/settings/SettingsShared";
import type { UsageMeter } from "@/lib/api";
import { formatNumber, RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const meterMeta: Record<string, { label: string; unit?: string; decimals?: number }> = {
  compute:             { label: "Compute",              unit: "CU-hours", decimals: 2 },
  agent_builds:        { label: "Agent Builds",         unit: "builds" },
  agent_deployments:   { label: "Deployments" },
  agents:              { label: "Agents" },
  members:             { label: "Members" },
  knowledge_stores:    { label: "Knowledge Stores" },
  knowledge_storage:   { label: "Knowledge Storage",    unit: "GB",       decimals: 2 },
  knowledge_compute:   { label: "Knowledge Compute",    unit: "CU-hours", decimals: 2 },
  knowledge_endpoints: { label: "PrivateLink Endpoints" },
};

const statusStyles: Record<string, string> = {
  pending: "bg-amber-500/10 text-amber-600",
  approved: "bg-green-500/10 text-green-600",
  denied: "bg-destructive/10 text-destructive",
};

function UsageBar({ usage, quota, onRequestIncrease }: { usage: number; quota: number; onRequestIncrease?: () => void }) {
  const pct = Math.min((usage / quota) * 100, 100);
  const isHigh = pct >= 90;
  const isMedium = pct >= 75 && !isHigh;

  return (
    <div className="mt-2.5 space-y-1">
      <div className="h-1.5 w-full rounded-full bg-border">
        <div
          className={`h-full rounded-full transition-all ${
            isHigh ? "bg-destructive" : isMedium ? "bg-amber-500" : "bg-primary"
          }`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
        <span>{formatNumber(usage, 1)} / {formatNumber(quota, 0)} used</span>
        {onRequestIncrease && (
          <button onClick={onRequestIncrease} className="cursor-pointer text-primary hover:underline">
            Request increase
          </button>
        )}
      </div>
    </div>
  );
}

function StatCard({
  label,
  featureKey,
  meter,
  unit,
  decimals = 0,
  account,
  canRequestIncrease,
}: {
  label: string;
  featureKey: string;
  meter: UsageMeter;
  unit?: string;
  decimals?: number;
  account: string;
  canRequestIncrease: boolean;
}) {
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <div className="rounded-lg border border-border bg-surface px-5 py-4">
      <div className="text-[12px] font-medium text-muted-foreground">{label}</div>
      <div className="mt-1 flex items-baseline gap-1.5">
        <span className="text-2xl font-semibold tabular-nums text-foreground">
          {formatNumber(meter.usage, decimals)}
        </span>
        {unit && <span className="text-[12px] text-muted-foreground">{unit}</span>}
      </div>
      {meter.quota != null ? (
        <UsageBar usage={meter.usage} quota={meter.quota} onRequestIncrease={canRequestIncrease ? () => setDialogOpen(true) : undefined} />
      ) : (
        <div className="mt-2.5 text-[11px] text-muted-foreground">Unlimited</div>
      )}
      {meter.quota != null && canRequestIncrease && (
        <RequestIncreaseDialog
          featureKey={featureKey}
          label={label}
          meter={meter}
          account={account}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      )}
    </div>
  );
}

function UsageMeters({ account, canRequestIncrease }: { account: string; canRequestIncrease: boolean }) {
  const { data, isLoading, error } = useAccountUsage(account);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
        <Loader2 size={14} className="animate-spin" />
        Loading usage data...
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-border bg-surface px-5 py-4">
        <p className="text-[13px] text-muted-foreground">
          Unable to load usage data. Usage metering may not be configured.
        </p>
      </div>
    );
  }

  if (!data) return null;

  const periodLabel = new Date(data.period_start).toLocaleDateString(undefined, {
    month: "long",
    year: "numeric",
  });
  const entries = Object.entries(data.meters);
  const hasCompute = "compute" in data.meters;

  return (
    <div className="flex flex-col gap-5">
      <div className="text-[13px] text-muted-foreground">
        Current billing period:{" "}
        <span className="font-medium text-foreground">{periodLabel}</span>
      </div>
      <div className="grid grid-cols-2 gap-3">
        {entries.map(([key, meter]) => {
          const info = meterMeta[key];
          return (
            <StatCard
              key={key}
              label={info?.label ?? key}
              featureKey={key}
              meter={meter}
              unit={info?.unit}
              decimals={info?.decimals}
              account={account}
              canRequestIncrease={canRequestIncrease}
            />
          );
        })}
      </div>
      {hasCompute && (
        <div className="flex gap-2.5 rounded-lg border border-border bg-surface px-4 py-3">
          <Info size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
          <p className="text-[12px] text-muted-foreground">
            <span className="font-medium text-foreground">1 Compute Unit (CU)</span>{" "}
            = 1 vCPU + 2 GB RAM per hour, per replica.
          </p>
        </div>
      )}
    </div>
  );
}

function QuotaRequestsTable({ account }: { account: string }) {
  const { data, isLoading } = useQuotaIncreaseRequests(account);

  if (isLoading || !data?.requests?.length) return null;

  return (
    <div className="space-y-2">
      <h3 className="text-[13px] font-medium text-foreground">Quota Increase Requests</h3>
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
                <span className={`inline-block rounded-full px-2 py-0.5 text-[11px] font-medium ${statusStyles[req.status] ?? "bg-muted text-muted-foreground"}`}>
                  {req.status}
                </span>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {new Date(req.created_at).toLocaleDateString()}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

export function UsageView({ account, canRequestIncrease = true }: { account: string; canRequestIncrease?: boolean }) {
  return (
    <>
      <SectionHeader title="Usage" subtitle="Resource consumption for your account this billing period" />
      <UsageMeters account={account} canRequestIncrease={canRequestIncrease} />
      <QuotaRequestsTable account={account} />
    </>
  );
}
