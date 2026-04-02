import { useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useRequestQuotaIncrease } from "@/api/queries/usage";

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
      <div className={cn("rounded-[10px] border border-border bg-surface p-[12px_14px]", className)}>
        <div className="mb-2 flex items-center justify-between">
          <span className="font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
            {label}
          </span>
          {quota != null && (
            <button
              type="button"
              onClick={() => setDialogOpen(true)}
              className="font-sans text-body-sm text-primary hover:text-primary/80 transition-colors"
            >
              Request increase
            </button>
          )}
        </div>
        {loading ? (
          <div className="flex flex-col gap-2 animate-pulse">
            <div className="h-6 w-1/2 rounded bg-muted" />
            <div className="h-3.5 w-3/4 rounded bg-muted" />
          </div>
        ) : (
          <>
            <div className="flex items-baseline gap-1.5">
              <span className="font-sans text-heading-2 font-bold text-foreground">
                {value.toFixed(1)}
              </span>
              <span className="font-sans text-body-sm text-muted-foreground">
                / {quota ?? "—"} {unit}
              </span>
            </div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-primary transition-[width] duration-300"
                style={{ width: `${pct}%` }}
              />
            </div>
          </>
        )}
      </div>
      {quota != null && (
        <RequestIncreaseDialog
          account={account}
          label={label}
          usage={value}
          quota={quota}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      )}
    </>
  );
}

function formatNumber(value: number, decimals = 1): string {
  if (value === 0) return "0";
  if (value < 0.01) return "< 0.01";
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  });
}

function RequestIncreaseDialog({
  account,
  label,
  usage,
  quota,
  open,
  onOpenChange,
}: {
  account: string;
  label: string;
  usage: number;
  quota: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState("");
  const mutation = useRequestQuotaIncrease(account);

  const handleSubmit = () => {
    mutation.mutate(
      {
        feature_key: "compute",
        current_usage: usage,
        current_quota: quota,
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
              <p className="font-medium">{formatNumber(usage, 1)}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Current quota</span>
              <p className="font-medium">{formatNumber(quota, 0)}</p>
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
