import { useState } from "react";
import { cn } from "@/lib/utils";
import { formatNumber, RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";

export interface UsageCardProps {
  label: string;
  value: number;
  quota: number | undefined;
  unit: string;
  account: string;
  loading?: boolean;
  className?: string;
}

export function UsageCard({ label, value, quota, unit, account, loading, className }: UsageCardProps) {
  const pct = quota ? Math.min((value / quota) * 100, 100) : 0;
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <>
      <div className={cn("relative rounded-[10px] border border-border bg-surface p-[12px_14px]", className)}>
        <span className="mb-4 block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
          {label}
        </span>
        {quota != null && (
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            className="absolute top-[12px] right-[14px] font-sans text-body-sm text-primary hover:text-primary/80 transition-colors"
          >
            Request increase
          </button>
        )}
        {loading ? (
          <div className="flex items-center gap-3 animate-pulse">
            <div className="h-6 w-2/5 rounded bg-muted shrink-0" />
            <div className="flex-1 h-1.5 rounded-full bg-muted" />
          </div>
        ) : (
          <>
            <div className="flex items-start gap-3">
              <div className="flex items-baseline gap-1.5 shrink-0">
                <span className="font-sans text-heading-2 font-bold text-foreground">
                  {formatNumber(value, 1)}
                </span>
                <span className="font-sans text-body-sm text-muted-foreground">
                  / {quota ?? "—"} {unit}
                </span>
              </div>
              <div className="flex-1 self-center h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full bg-primary transition-[width] duration-300"
                  style={{ width: `${pct}%` }}
                />
              </div>
            </div>
          </>
        )}
      </div>
      {quota != null && (
        <RequestIncreaseDialog
          featureKey="compute"
          label={label}
          meter={{ usage: value, quota }}
          account={account}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      )}
    </>
  );
}
