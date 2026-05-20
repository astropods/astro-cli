import { useState } from "react";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { formatNumber, RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";

export interface UsageCardProps {
  label: string;
  value: number;
  quota: number | undefined;
  unit: string;
  account: string;
  className?: string;
}

export function UsageCard({ label, value, quota, unit, account, className }: UsageCardProps) {
  const pct = quota ? Math.min((value / quota) * 100, 100) : 0;
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <>
      <Card className={cn("p-[12px_14px] relative dark:bg-surface", className)}>
        <span className="mb-4 block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
          {label}
        </span>
        {quota != null && (
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            className="absolute top-[12px] right-[14px] cursor-pointer font-sans text-body-sm text-primary hover:text-primary/80 transition-colors"
          >
            Request increase
          </button>
        )}
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
      </Card>
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
