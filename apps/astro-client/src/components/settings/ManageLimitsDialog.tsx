import { useEffect, useRef, useState } from "react";
import { linkifyEmail } from "@/lib/linkify-email";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useBillingSpend, useSetBillingSpendThresholds } from "@/api/queries/billing";
import { getApiErrorMessage } from "@/lib/api";
import { formatMoney, thresholdDollars } from "@/lib/billing-balances";
import { canManageAccountBilling } from "@/lib/billing-copy";
import { useAuth } from "@/lib/auth";

const AMOUNT_PLACEHOLDER = "0.00";

function parseAmount(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === "") return null;
  const n = Number(trimmed);
  return Number.isNaN(n) ? NaN : n;
}

function formatAmount(amount: number | undefined): string {
  return amount == null ? "" : String(amount);
}

// Seeds warning/limit from saved values on open; skips reseeding once the
// account has typed, so a slow load can't clobber in-flight input.
function useRowInputs(
  currentWarning: number | undefined,
  currentLimit: number | undefined,
  open: boolean,
  ready: boolean,
) {
  const [warningInput, setWarningInputState] = useState(formatAmount(currentWarning));
  const [limitInput, setLimitInputState] = useState(formatAmount(currentLimit));
  const touched = useRef(false);

  useEffect(() => {
    if (!open) {
      touched.current = false;
      return;
    }
    if (ready && !touched.current) {
      setWarningInputState(formatAmount(currentWarning));
      setLimitInputState(formatAmount(currentLimit));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, ready]);

  const setWarningInput = (value: string) => {
    touched.current = true;
    setWarningInputState(value);
  };
  const setLimitInput = (value: string) => {
    touched.current = true;
    setLimitInputState(value);
  };

  return { warningInput, setWarningInput, limitInput, setLimitInput };
}

// The message that needs acting on sits closest to the control it is about.
function LimitField({
  id,
  label,
  removeLabel,
  placeholder,
  prefix,
  disabled,
  value,
  onChange,
  onRemove,
  helperText,
  error,
  notice,
}: {
  id: string;
  label: string;
  removeLabel: string;
  placeholder: string;
  prefix?: string;
  disabled: boolean;
  value: string;
  onChange: (value: string) => void;
  onRemove: () => void;
  helperText: string;
  error?: string | null;
  notice?: string | null;
}) {
  const messageId = `${id}-message`;
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between">
        <Label size="md" className="mb-0" htmlFor={id}>
          {label}
        </Label>
        {value !== "" && (
          <button
            type="button"
            aria-label={removeLabel}
            className="text-body-sm text-muted-foreground hover:text-foreground"
            onClick={onRemove}
          >
            Remove
          </button>
        )}
      </div>
      <p className="text-body-sm text-muted-foreground">{helperText}</p>
      <div className="relative">
        {prefix && (
          <span className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-body-sm text-muted-foreground">
            {prefix}
          </span>
        )}
        <Input
          id={id}
          inputMode="decimal"
          placeholder={placeholder}
          className={prefix ? "w-full pl-6" : "w-full"}
          disabled={disabled}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          aria-invalid={!!error}
          aria-describedby={error || notice ? messageId : undefined}
        />
      </div>
      {error ? (
        <p id={messageId} className="text-body-sm text-destructive">
          {error}
        </p>
      ) : (
        notice && (
          <p id={messageId} className="text-body-sm text-warning">
            {notice}
          </p>
        )
      )}
    </div>
  );
}

// Account-wide alert threshold and spend limit. Per-metric (Compute, AI
// Gateway) caps are intentionally not exposed here; see the changelog for
// why. The provider and the account may still hold one from before this
// change, but this dialog only ever shows and writes the one control.
export function ManageLimitsDialog({
  account,
  open,
  onOpenChange,
}: {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { data, isLoading } = useBillingSpend(account);
  const { role, personalAccount } = useAuth();
  const canManage = canManageAccountBilling(account, personalAccount?.name, role);
  const saveSpend = useSetBillingSpendThresholds(account);

  const spend = data?.available ? data.data : undefined;
  const currentWarning = thresholdDollars(spend?.warning?.amount);
  const currentLimit = thresholdDollars(spend?.limit?.amount);
  const currentSpend = spend?.has_usage_spend ? spend.usage_spend : 0;
  const currency = spend?.currency ?? "USD";
  const ready = !isLoading;

  const spendRow = useRowInputs(currentWarning, currentLimit, open, ready);

  const warning = parseAmount(spendRow.warningInput);
  const limit = parseAmount(spendRow.limitInput);

  // A field reports the cross-field conflict only once both parse, so a
  // half-typed "-" doesn't also claim it is above the spend limit.
  const invalid = (v: number | null) => v != null && (Number.isNaN(v) || v < 0);
  const AMOUNT_ERROR = "Enter an amount of zero or more.";

  let warningError: string | null = invalid(warning) ? AMOUNT_ERROR : null;
  const limitError: string | null = invalid(limit) ? AMOUNT_ERROR : null;
  if (!warningError && !limitError && warning != null && limit != null && warning >= limit) {
    warningError = `Agents already pause at your ${formatMoney(limit, currency)} spend limit, so you never get this alert. Enter an amount lower than the spend limit.`;
  }
  const blockingError = warningError ?? limitError;

  // Not blocking: valid, but the provider re-evaluates the account the
  // moment this saves (SetCustomerSpendThreshold resets the alert rather
  // than waiting for its next scheduled check), so a limit under what's
  // already been spent this period pauses agents right away, not at the
  // next period rollover.
  const pausesNow = !blockingError && limit != null && limit < currentSpend;

  const spendChanged =
    spendRow.warningInput !== formatAmount(currentWarning) || spendRow.limitInput !== formatAmount(currentLimit);

  async function onSave() {
    if (blockingError || !spendChanged) return;

    try {
      const result = await saveSpend.mutateAsync({
        warning: warning == null ? null : Math.round(warning * 100),
        limit: limit == null ? null : Math.round(limit * 100),
      });
      // Write can succeed while the provider still fails to lift a live
      // suspension; nothing retries that, so don't claim plain success.
      if (result.limit_lift_failed) {
        toast.warning("Limits saved, but agents are still paused. Try raising the limit again in a moment.");
      } else {
        toast.success("Limits saved");
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(linkifyEmail(getApiErrorMessage(err, "Could not save limits.")));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Manage limits</DialogTitle>
          <DialogDescription>
            Control how much you spend on pay-as-you-go each billing period.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-5 py-2">
          <LimitField
            id="alert-at"
            label="Alert threshold"
            removeLabel="Remove alert threshold"
            placeholder={AMOUNT_PLACEHOLDER}
            prefix="$"
            disabled={!canManage}
            value={spendRow.warningInput}
            onChange={spendRow.setWarningInput}
            onRemove={() => spendRow.setWarningInput("")}
            helperText="We notify you when spend reaches this amount. No interruptions to your agents."
            error={warningError}
          />

          <LimitField
            id="spend-limit"
            label="Spend limit"
            removeLabel="Remove spend limit"
            placeholder={AMOUNT_PLACEHOLDER}
            prefix="$"
            disabled={!canManage}
            value={spendRow.limitInput}
            onChange={spendRow.setLimitInput}
            onRemove={() => spendRow.setLimitInput("")}
            helperText="Agents pause when spend reaches this amount. You won't be charged past it."
            error={limitError}
            notice={
              pausesNow
                ? `Spend this period is already ${formatMoney(currentSpend, currency)}. Saving this limit pauses your agents immediately.`
                : null
            }
          />

          {!canManage && (
            <p className="text-body-sm text-muted-foreground">Only owners and admins can change limits.</p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={onSave}
            disabled={!canManage || isLoading || saveSpend.isPending || !spendChanged}
          >
            {saveSpend.isPending ? "Saving…" : "Save limits"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
