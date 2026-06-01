import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Loader2, Info } from "lucide-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useAccountUsage, useQuotaIncreaseRequests } from "@/api/queries";
import { SectionHeader } from "@/components/settings/SettingsShared";
import type { UsageMeter } from "@/lib/api";
import { formatNumber, RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { Tag } from "@/components/Tag";
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

const CU_UNITS = new Set(["CU-hours"]);

const CATEGORY_DEFS: { label: string; keys: string[] }[] = [
  { label: "Agents",    keys: ["compute", "agent_builds", "agent_deployments", "agents"] },
  { label: "Knowledge", keys: ["knowledge_stores", "knowledge_storage", "knowledge_compute", "knowledge_endpoints"] },
  { label: "Account",   keys: ["members"] },
];
const KNOWN_KEYS = new Set(CATEGORY_DEFS.flatMap((c) => c.keys));

function CuTooltip() {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Info size={12} className="text-faint-foreground cursor-default shrink-0" />
        </TooltipTrigger>
        <TooltipContent>
          1 Compute Unit (CU) = 1 vCPU + 2 GB RAM per hour, per replica.
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

const statusBadgeColor: Record<string, StatusBadgeColor> = {
  pending:  "warning",
  approved: "success",
  denied:   "error",
};

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
  const hasQuota = meter.quota != null;
  const pct = hasQuota ? Math.min((meter.usage / meter.quota!) * 100, 100) : 0;
  const isAtLimit = pct >= 100;
  const isHigh = pct >= 90 && !isAtLimit;
  const isMedium = pct >= 75 && !isHigh && !isAtLimit;

  return (
    <div className="rounded-lg border border-border bg-surface px-5 py-3">
      <div className="flex items-center justify-between">
        <span className="font-mono text-mono-sm text-faint-foreground uppercase tracking-wide">{label}</span>
        {hasQuota && canRequestIncrease && (
          <Button variant="link" className="h-auto p-0 text-body-sm no-underline hover:underline shrink-0" onClick={() => setDialogOpen(true)}>
            Request increase
          </Button>
        )}
      </div>
      <div className="mt-1 flex items-center gap-2">
        <span className="text-2xl font-semibold tabular-nums text-foreground">
          {formatNumber(meter.usage, decimals)}
        </span>
        {hasQuota ? (
          <span className="flex items-center gap-1 text-body-sm text-muted-foreground">
            / {formatNumber(meter.quota!, 0)} {unit ?? "used"}
            {unit && CU_UNITS.has(unit) && <CuTooltip />}
          </span>
        ) : (
          <span className="flex items-center gap-1 text-body-sm text-muted-foreground">
            {unit}
            {unit && CU_UNITS.has(unit) && <CuTooltip />}
          </span>
        )}
        {isAtLimit && <Tag color="coral">Full</Tag>}
      </div>
      {hasQuota ? (
        <div className="mt-2 h-1.5 w-full rounded-full bg-border">
          <div
            className={`h-full rounded-full transition-all ${isAtLimit || isHigh ? "bg-destructive" : isMedium ? "" : "bg-primary"}`}
            style={{ width: `${pct}%`, ...(isMedium ? { background: "var(--warning)" } : {}) }}
          />
        </div>
      ) : (
        <div className="mt-2 text-body-sm text-muted-foreground">Unlimited</div>
      )}
      {hasQuota && canRequestIncrease && (
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
      <div className="rounded-lg border border-border bg-surface px-5 py-3">
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
  const meters = data.meters;

  const uncategorized = Object.keys(meters).filter((k) => !KNOWN_KEYS.has(k));
  const categories = [
    ...CATEGORY_DEFS,
    ...(uncategorized.length ? [{ label: "Other", keys: uncategorized }] : []),
  ];

  return (
    <div className="flex flex-col gap-8">
      <div className="text-[13px] text-muted-foreground">
        Current billing period:{" "}
        <span className="font-medium text-foreground">{periodLabel}</span>
      </div>

      {categories.map(({ label, keys }) => {
        const visible = keys.filter((k) => k in meters);
        if (!visible.length) return null;
        return (
          <div key={label} className="flex flex-col gap-3">
            <h3 className="text-heading-4 text-foreground">{label}</h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {visible.map((key) => {
                const meter = meters[key]!;
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
          </div>
        );
      })}

    </div>
  );
}

function QuotaRequestsTable({ account }: { account: string }) {
  const { data, isLoading } = useQuotaIncreaseRequests(account);

  if (isLoading || !data?.requests?.length) return null;

  return (
    <div className="space-y-2">
      <h3 className="text-heading-4 text-foreground">Quota increase requests</h3>
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
    </div>
  );
}

export function UsageView({ account, canRequestIncrease = true }: { account: string; canRequestIncrease?: boolean }) {
  return (
    <>
      <SectionHeader
        title="Usage"
        subtitle="Resource consumption for your account this billing period"
      />
      <UsageMeters account={account} canRequestIncrease={canRequestIncrease} />
      <QuotaRequestsTable account={account} />
    </>
  );
}
