import { useState } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
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
import type { UsageMeter } from "@/lib/api";

export function formatNumber(value: number, decimals = 1): string {
  if (value === 0) return "0";
  if (value < 0.01) return "< 0.01";
  return value.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimals,
  });
}

export function RequestIncreaseDialog({
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

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setReason("");
      setAmount("");
      mutation.reset();
    }
    onOpenChange(open);
  };

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
          toast.success("Quota increase requested");
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Request quota increase</DialogTitle>
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
              "Submit request"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
