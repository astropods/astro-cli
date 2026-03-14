import { Loader2, Info } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { useAccountUsage } from "@/api/queries";
import type { UsageMeter } from "@/lib/api";

function formatNumber(value: number, decimals = 1): string {
  if (value === 0) return "0";
  if (value < 0.01) return "< 0.01";
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  });
}

function UsageBar({ usage, quota }: { usage: number; quota: number }) {
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
      <div className="text-[11px] text-muted-foreground">
        {formatNumber(usage, 1)} / {formatNumber(quota, 0)} used
      </div>
    </div>
  );
}

function StatCard({
  label,
  meter,
  unit,
  decimals = 0,
}: {
  label: string;
  meter: UsageMeter;
  unit?: string;
  decimals?: number;
}) {
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
        <UsageBar usage={meter.usage} quota={meter.quota} />
      ) : (
        <div className="mt-2.5 text-[11px] text-muted-foreground">Unlimited</div>
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
          meter={data.compute_unit_hours}
          unit="CU-hours"
          decimals={2}
        />
        <StatCard
          label="Agent Builds"
          meter={data.agent_builds}
          unit="builds"
        />
        <StatCard
          label="Active Deployments"
          meter={data.active_deployments}
        />
        <StatCard
          label="Registered Agents"
          meter={data.active_agents}
        />
      </div>
      <div className="flex gap-2.5 rounded-lg border border-border bg-surface px-4 py-3">
        <Info size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
        <div className="space-y-1 text-[12px] text-muted-foreground">
          <p>
            <span className="font-medium text-foreground">1 Compute Unit (CU)</span>{" "}
            = 1 vCPU + 2 GB RAM per hour, per replica.
          </p>
          <p>Usage data updates approximately every 5 minutes.</p>
        </div>
      </div>
    </div>
  );
}

export default function UsageSettings() {
  return (
    <>
      <div className="space-y-1">
        <h2 className="text-heading-2 text-foreground">Usage</h2>
        <p className="text-[13px] text-muted-foreground">
          Resource consumption for your account this billing period
        </p>
      </div>
      <UsageContent />
    </>
  );
}
