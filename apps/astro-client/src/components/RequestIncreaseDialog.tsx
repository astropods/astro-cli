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
import { getApiErrorMessage, type UsageMeter } from "@/lib/api";
import { formatNumber } from "@/lib/format-utils";
import { formatMoney } from "@/lib/billing-balances";

// Display metadata for quota feature keys, shared by the feature picker and
// the requests table. Metered features (compute, knowledge storage/compute)
// are billing-gated, not requestable here.
// A `money` key carries dollars, not a count.
export const meterMeta: Record<string, { label: string; unit?: string; decimals?: number; money?: boolean }> = {
  agent_builds:        { label: "Agent Builds",         unit: "builds" },
  agent_deployments:   { label: "Deployments" },
  blueprints:          { label: "Blueprints" },
  members:             { label: "Members" },
  knowledge_stores:    { label: "Knowledge Stores" },
  knowledge_endpoints: { label: "PrivateLink Endpoints" },
  spend_limit:         { label: "Spend limit", money: true },
};

export const SPEND_LIMIT_KEY = "spend_limit";

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
  const [reasonTouched, setReasonTouched] = useState(false);
  const [amountTouched, setAmountTouched] = useState(false);
  const mutation = useRequestQuotaIncrease(account);
  const reasonMissing = reasonTouched && !reason.trim();

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
  const money = meterMeta[activeKey]?.money ?? false;
  const amountMissing = money && amountTouched && !amount.trim();
  const formatAmount = (value: number, decimals: number) =>
    money ? formatMoney(value, "USD") : formatNumber(value, decimals);

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setReason("");
      setAmount("");
      setReasonTouched(false);
      setAmountTouched(false);
      mutation.reset();
    }
    onOpenChange(next);
  };

  const handleSubmit = () => {
    if (!activeKey) return;
    if (money && !amount.trim()) {
      setAmountTouched(true);
      return;
    }
    if (!reason.trim()) {
      setReasonTouched(true);
      return;
    }
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
          handleOpenChange(false);
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
            {money
              ? "Tell us the monthly limit you need. The Astro team reviews every request."
              : featureKey
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
              <span className="text-muted-foreground">{money ? "Spend this period" : "Current usage"}</span>
              <p className="font-medium">{formatAmount(activeMeter.usage, 1)}</p>
            </div>
            <div>
              <span className="text-muted-foreground">{money ? "Current ceiling" : "Current quota"}</span>
              <p className="font-medium">
                {activeMeter.quota != null ? formatAmount(activeMeter.quota, 0) : "Unlimited"}
              </p>
            </div>
          </div>
          <div>
            <label className="text-[12px] font-medium text-foreground">
              {money ? "Requested monthly limit" : "Requested amount"}
              {!money && <span className="text-muted-foreground font-normal"> (optional)</span>}
            </label>
            <div className="relative">
              {money && (
                <span className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-[13px] text-muted-foreground">
                  $
                </span>
              )}
              <input
                type="number"
                min={0}
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder={money ? "0.00" : "Leave blank for admin to decide"}
                className={`mt-1 w-full rounded border border-border bg-surface py-1.5 text-[13px] outline-none focus:ring-1 focus:ring-primary ${money ? "pl-6 pr-3" : "px-3"}`}
              />
            </div>
            {amountMissing && <p className="mt-1 text-[12px] text-destructive">An amount is required.</p>}
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
            {reasonMissing && <p className="mt-1 text-[12px] text-destructive">A reason is required.</p>}
          </div>
          {mutation.error && (
            <p className="text-[12px] text-destructive">
              {getApiErrorMessage(mutation.error, "Couldn't submit the request.")}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button size="sm" onClick={handleSubmit} disabled={!activeKey || mutation.isPending}>
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
