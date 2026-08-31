import { useState } from "react";
import { Button } from "@/components/ui/button";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { useAccountUsage, useQuotaIncreaseRequests } from "@/api/queries";
import { EmptyState, LoadError, LoadingRows } from "@/components/settings/SettingsShared";
import { RequestIncreaseDialog, meterMeta } from "@/components/RequestIncreaseDialog";
import { formatNumber } from "@/lib/format-utils";
import { formatMoney } from "@/lib/billing-balances";
import { formatShortDateLocal } from "@/lib/date-utils";
import type { QuotaIncreaseListItem, UsageMeter } from "@/lib/api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const statusBadgeColor: Record<string, StatusBadgeColor> = {
  pending: "warning",
  approved: "success",
  denied: "error",
};

// ---------------------------------------------------------------------------
// The account's resource quotas, and any outstanding request to raise one.
// ---------------------------------------------------------------------------

function QuotaRequestsTable({ requests }: { requests: QuotaIncreaseListItem[] }) {
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
        {requests.map((req) => (
          <TableRow key={req.id}>
            <TableCell className="font-medium">
              {meterMeta[req.feature_key]?.label ?? req.feature_key}
            </TableCell>
            <TableCell className="text-muted-foreground max-w-[200px] truncate">
              {req.reason}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {req.requested_amount == null
                ? "—"
                : meterMeta[req.feature_key]?.money
                  ? formatMoney(req.requested_amount, "USD")
                  : formatNumber(req.requested_amount, 0)}
            </TableCell>
            <TableCell>
              <StatusBadge color={statusBadgeColor[req.status] ?? "muted"}>
                {req.status}
              </StatusBadge>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatShortDateLocal(req.created_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function LimitsMeterGrid({ meters }: { meters: Record<string, UsageMeter> }) {
  const keys = Object.keys(meters);
  if (!keys.length) {
    return <EmptyState message="No usage data available." />;
  }
  return (
    <div className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3">
      {keys.map((key) => {
        const meter = meters[key]!;
        const info = meterMeta[key];
        const label = info?.label ?? key;
        const decimals = info?.decimals ?? 0;
        const hasQuota = meter.quota != null;
        return (
          <div key={key} className="flex items-center justify-between gap-2">
            <span className="text-body-sm text-foreground">{label}</span>
            <span className="text-body-sm tabular-nums text-muted-foreground">
              {formatNumber(meter.usage, decimals)} / {hasQuota ? formatNumber(meter.quota!, 0) : "∞"}
            </span>
          </div>
        );
      })}
    </div>
  );
}

export function ResourceLimitsSection({ account, canRequestIncrease }: { account: string; canRequestIncrease: boolean }) {
  const { data: usage, isLoading, isLoadingError, refetch } = useAccountUsage(account);
  const requestsQuery = useQuotaIncreaseRequests(account);
  const [dialogOpen, setDialogOpen] = useState(false);
  const meters = usage?.meters ?? {};
  const hasMeters = Object.keys(meters).length > 0;
  const requests = requestsQuery.data?.requests ?? [];
  // An empty table under a permanent heading reads as something to act on.
  const showRequests = requestsQuery.isLoading || requestsQuery.isLoadingError || requests.length > 0;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h3 className="text-heading-4 text-foreground">Quotas</h3>
          {canRequestIncrease && (
            <Button size="sm" variant="outline" disabled={!hasMeters} onClick={() => setDialogOpen(true)}>
              Request increase
            </Button>
          )}
        </div>
        {isLoading ? (
          <LoadingRows rows={2} />
        ) : isLoadingError ? (
          <LoadError onRetry={() => refetch()} />
        ) : (
          <LimitsMeterGrid meters={meters} />
        )}
      </div>

      {showRequests && (
        <div className="flex flex-col gap-3">
          <h3 className="text-heading-4 text-foreground">Quota increase requests</h3>
          {requestsQuery.isLoading ? (
            <LoadingRows rows={2} />
          ) : requestsQuery.isLoadingError ? (
            <LoadError onRetry={() => requestsQuery.refetch()} />
          ) : (
            <QuotaRequestsTable requests={requests} />
          )}
        </div>
      )}

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
