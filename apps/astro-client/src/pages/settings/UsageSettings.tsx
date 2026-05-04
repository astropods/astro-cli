import { useState } from "react";
import type { MetaFunction } from "react-router";
import { Loader2, Info } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { useAccountUsage, useQuotaIncreaseRequests } from "@/api/queries";
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

export const meta: MetaFunction = () => [{ title: "Usage - Settings | Astro" }];

function UsageBar({ usage, quota, onRequestIncrease }: { usage: number; quota: number; onRequestIncrease?: () => void }) {
  const pct = Math.min((usage / quota) * 100, 100);
  const isHigh = pct >= 90;
  const isMedium = pct >= 75 && !isHigh;

  return (
    <div className="mt-2.5 space-y-1">
      <div className="h-1.5 w-full rounded-full bg-border">
        <div
          className={`h-full rounded-full transition-all ${
            isHigh
              ? "bg-destructive"
              : isMedium
                ? "bg-amber-500"
                : "bg-primary"
          }`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
        <span>{formatNumber(usage, 1)} / {formatNumber(quota, 0)} used</span>
        {onRequestIncrease && (
          <button
            onClick={onRequestIncrease}
            className="cursor-pointer text-primary hover:underline"
          >
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
}: {
  label: string;
  featureKey: string;
  meter: UsageMeter;
  unit?: string;
  decimals?: number;
  account: string;
}) {
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <div className="rounded-lg border border-border bg-surface px-5 py-4">
      <div className="text-[12px] font-medium text-muted-foreground">
        {label}
      </div>
      <div className="mt-1 flex items-baseline gap-1.5">
        <span className="text-2xl font-semibold tabular-nums text-foreground">
          {formatNumber(meter.usage, decimals)}
        </span>
        {unit && (
          <span className="text-[12px] text-muted-foreground">{unit}</span>
        )}
      </div>
      {meter.quota != null ? (
        <UsageBar
          usage={meter.usage}
          quota={meter.quota}
          onRequestIncrease={() => setDialogOpen(true)}
        />
      ) : (
        <div className="mt-2.5 text-[11px] text-muted-foreground">Unlimited</div>
      )}
      {meter.quota != null && (
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

function UsageContent() {
  const { personalAccount } = useAuth();
  const accountName = personalAccount?.name ?? "";
  const { data, isLoading, error } = useAccountUsage(accountName);

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

  const periodStart = new Date(data.period_start);
  const periodLabel = periodStart.toLocaleDateString(undefined, {
    month: "long",
    year: "numeric",
  });

  return (
    <div className="flex flex-col gap-5">
      <div className="text-[13px] text-muted-foreground">
        Current billing period:{" "}
        <span className="font-medium text-foreground">{periodLabel}</span>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <StatCard
          label="Compute Usage"
          featureKey="compute"
          meter={data.compute_unit_hours}
          unit="CU-hours"
          decimals={2}
          account={accountName}
        />
        <StatCard
          label="Agent Builds"
          featureKey="agent_builds"
          meter={data.agent_builds}
          unit="builds"
          account={accountName}
        />
        <StatCard
          label="Active Deployments"
          featureKey="agent_deployments"
          meter={data.active_deployments}
          account={accountName}
        />
        <StatCard
          label="Registered Agents"
          featureKey="agents"
          meter={data.active_agents}
          account={accountName}
        />
      </div>
      <div className="flex gap-2.5 rounded-lg border border-border bg-surface px-4 py-3">
        <Info size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
        <p className="text-[12px] text-muted-foreground">
          <span className="font-medium text-foreground">1 Compute Unit (CU)</span>{" "}
          = 1 vCPU + 2 GB RAM per hour, per replica.
        </p>
      </div>
    </div>
  );
}

const featureLabels: Record<string, string> = {
  compute: "Compute",
  agent_builds: "Agent Builds",
  agent_deployments: "Deployments",
  agents: "Agents",
  members: "Members",
};

const statusStyles: Record<string, string> = {
  pending: "bg-amber-500/10 text-amber-600",
  approved: "bg-green-500/10 text-green-600",
  denied: "bg-destructive/10 text-destructive",
};

function QuotaRequestsTable({ account }: { account: string }) {
  const { data, isLoading } = useQuotaIncreaseRequests(account);

  if (isLoading || !data?.requests?.length) return null;

  return (
    <div className="space-y-2">
      <h3 className="text-[13px] font-medium text-foreground">
        Quota Increase Requests
      </h3>
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
                {featureLabels[req.feature_key] ?? req.feature_key}
              </TableCell>
              <TableCell className="text-muted-foreground max-w-[200px] truncate">
                {req.reason}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {req.requested_amount != null
                  ? formatNumber(req.requested_amount, 0)
                  : "\u2014"}
              </TableCell>
              <TableCell>
                <span
                  className={`inline-block rounded-full px-2 py-0.5 text-[11px] font-medium ${statusStyles[req.status] ?? "bg-muted text-muted-foreground"}`}
                >
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

export default function UsageSettings() {
  const { personalAccount } = useAuth();
  const accountName = personalAccount?.name ?? "";

  return (
    <>
      <div className="space-y-1">
        <h2 className="text-heading-2 text-foreground">Usage</h2>
        <p className="text-[13px] text-muted-foreground">
          Resource consumption for your account this billing period
        </p>
      </div>
      <UsageContent />
      <QuotaRequestsTable account={accountName} />
    </>
  );
}
