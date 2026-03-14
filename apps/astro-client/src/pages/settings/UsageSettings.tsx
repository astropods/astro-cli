import { useState } from "react";
import { Loader2, Info } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { useAccountUsage, useQuotaIncreaseRequests, useRequestQuotaIncrease } from "@/api/queries";
import type { UsageMeter } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function formatNumber(value: number, decimals = 1): string {
  if (value === 0) return "0";
  if (value < 0.01) return "< 0.01";
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  });
}

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
            className="text-primary hover:underline"
          >
            Request increase
          </button>
        )}
      </div>
    </div>
  );
}

function RequestIncreaseDialog({
  featureKey,
  label,
  meter,
  account,
  open,
  onOpenChange,
}: {
  featureKey: string;
  label: string;
  meter: UsageMeter;
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState("");
  const mutation = useRequestQuotaIncrease(account);

  const handleSubmit = () => {
    mutation.mutate(
      {
        feature_key: featureKey,
        current_usage: meter.usage,
        current_quota: meter.quota,
        requested_amount: amount ? parseFloat(amount) : undefined,
        reason,
      },
      {
        onSuccess: () => {
          onOpenChange(false);
          setReason("");
          setAmount("");
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Request Quota Increase</DialogTitle>
          <DialogDescription>
            Request additional {label.toLowerCase()} quota for your account.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-2">
          <div className="grid grid-cols-2 gap-3 text-[13px]">
            <div>
              <span className="text-muted-foreground">Current usage</span>
              <p className="font-medium">{formatNumber(meter.usage, 1)}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Current quota</span>
              <p className="font-medium">
                {meter.quota != null ? formatNumber(meter.quota, 0) : "Unlimited"}
              </p>
            </div>
          </div>
          <div>
            <label className="text-[12px] font-medium text-foreground">
              Requested amount
              <span className="text-muted-foreground font-normal"> (optional)</span>
            </label>
            <input
              type="number"
              min={0}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="Leave blank for admin to decide"
              className="mt-1 w-full rounded border border-border bg-surface px-3 py-1.5 text-[13px] outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div>
            <label className="text-[12px] font-medium text-foreground">
              Reason
            </label>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Why do you need more quota?"
              rows={3}
              className="mt-1 w-full rounded border border-border bg-surface px-3 py-1.5 text-[13px] outline-none focus:ring-1 focus:ring-primary resize-none"
            />
          </div>
          {mutation.error && (
            <p className="text-[12px] text-destructive">
              {mutation.error.message}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={!reason.trim() || mutation.isPending}
          >
            {mutation.isPending ? (
              <>
                <Loader2 size={12} className="mr-1 animate-spin" />
                Submitting...
              </>
            ) : (
              "Submit Request"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-[12px]">
          <thead>
            <tr className="border-b border-border bg-surface">
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Feature</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Reason</th>
              <th className="px-3 py-2 text-right font-medium text-muted-foreground">Requested</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Date</th>
            </tr>
          </thead>
          <tbody>
            {data.requests.map((req) => (
              <tr key={req.id} className="border-b border-border last:border-0">
                <td className="px-3 py-2 font-medium">
                  {featureLabels[req.feature_key] ?? req.feature_key}
                </td>
                <td className="px-3 py-2 text-muted-foreground max-w-[200px] truncate">
                  {req.reason}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">
                  {req.requested_amount != null
                    ? formatNumber(req.requested_amount, 0)
                    : "\u2014"}
                </td>
                <td className="px-3 py-2">
                  <span
                    className={`inline-block rounded-full px-2 py-0.5 text-[11px] font-medium ${statusStyles[req.status] ?? "bg-muted text-muted-foreground"}`}
                  >
                    {req.status}
                  </span>
                </td>
                <td className="px-3 py-2 text-muted-foreground">
                  {new Date(req.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
