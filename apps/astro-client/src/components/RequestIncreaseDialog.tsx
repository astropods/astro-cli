import { useMemo, useState } from "react";
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

// Display metadata for quota/usage feature keys. Shared by the request dialog's
// feature picker and the quota-requests table.
export const meterMeta: Record<string, { label: string; unit?: string; decimals?: number }> = {
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

type FixedProps = {
  /** Fixed feature — the picker is hidden. */
  featureKey: string;
  label: string;
  meter: UsageMeter;
  meters?: never;
};

type PickerProps = {
  /** Picker mode — user selects the feature from `meters`. */
  featureKey?: undefined;
  label?: undefined;
  meter?: undefined;
  meters: Record<string, UsageMeter>;
};

type RequestIncreaseDialogProps = (FixedProps | PickerProps) & {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function RequestIncreaseDialog({
  featureKey,
  label,
  meter,
  meters,
  account,
  open,
  onOpenChange,
}: RequestIncreaseDialogProps) {
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState("");
  const mutation = useRequestQuotaIncrease(account);

  // Picker mode: the dialog owns the selected feature. Default to the first
  // feature that has a quota (an unlimited feature has nothing to raise).
  const featureKeys = useMemo(() => (meters ? Object.keys(meters) : []), [meters]);
  const [selectedKey, setSelectedKey] = useState<string>(() => {
    if (featureKey) return featureKey;
    const withQuota = featureKeys.find((k) => meters?.[k]?.quota != null);
    return withQuota ?? featureKeys[0] ?? "";
  });

  const activeKey = featureKey ?? selectedKey;
  const activeMeter: UsageMeter = meter ?? meters?.[activeKey] ?? { usage: 0 };
  const activeLabel = label ?? meterMeta[activeKey]?.label ?? activeKey;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setReason("");
      setAmount("");
      mutation.reset();
    }
    onOpenChange(next);
  };

  const handleSubmit = () => {
    if (!activeKey) return;
    mutation.mutate(
      {
        feature_key: activeKey,
        current_usage: activeMeter.usage,
        current_quota: activeMeter.quota,
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
            {featureKey
              ? `Request additional ${activeLabel.toLowerCase()} quota for your account.`
              : "Request additional quota for your account."}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-2">
          {!featureKey && (
            <div>
              <label className="text-[12px] font-medium text-foreground">Feature</label>
              <select
                value={activeKey}
                onChange={(e) => setSelectedKey(e.target.value)}
                className="mt-1 w-full rounded border border-border bg-surface px-3 py-1.5 text-[13px] outline-none focus:ring-1 focus:ring-primary"
              >
                {featureKeys.map((key) => (
                  <option key={key} value={key}>
                    {meterMeta[key]?.label ?? key}
                  </option>
                ))}
              </select>
            </div>
          )}
          <div className="grid grid-cols-2 gap-3 text-[13px]">
            <div>
              <span className="text-muted-foreground">Current usage</span>
              <p className="font-medium">{formatNumber(activeMeter.usage, 1)}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Current quota</span>
              <p className="font-medium">
                {activeMeter.quota != null ? formatNumber(activeMeter.quota, 0) : "Unlimited"}
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
            disabled={!reason.trim() || !activeKey || mutation.isPending}
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
