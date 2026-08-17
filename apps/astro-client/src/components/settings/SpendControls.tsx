/** The account's own spend warning and limit, alongside what it has spent this
 *  period. The two ship together because a threshold is meaningless without the
 *  number it is measured against.
 *
 *  Both live in the billing provider and nowhere in our database, so this reads
 *  through and refetches after a write rather than holding a local copy that
 *  could disagree with what actually fires. */
import { useState } from "react";
import { toast } from "sonner";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { InlineBadge } from "@/components/InlineBadge";
import { useBillingSpend, useSetBillingSpendThresholds } from "@/api/queries/billing";
import { getApiErrorMessage } from "@/lib/api";

/** Spend arrives already converted to the currency named alongside it. The
 *  thresholds on the same response do not: they are the provider's raw cents,
 *  because that is the unit the write path sends and the provider stores. The
 *  two units share one response, so each has its own helper. */
function formatMoney(amount: number, currency?: string): string {
  const value = amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return currency?.toUpperCase().includes("USD") === false ? `${value} ${currency}` : `$${value}`;
}

/** The form edits dollars; the API speaks the provider's cents. An empty field
 *  clears the threshold, which is distinct from setting it to zero. */
function toCents(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === "") return null;
  return Math.round(Number(trimmed) * 100);
}

function toDollars(cents: number | undefined): string {
  return cents == null ? "" : String(cents / 100);
}

/** Keyed on the account so a switch discards the form. Settings pages read the
 *  org from a route param, so moving between two orgs re-renders rather than
 *  remounts; without the key an edited number follows the owner to the next
 *  account and the next Save writes it there. */
export function SpendControls({ account }: { account: string }) {
  return <SpendControlsForm key={account} account={account} />;
}

function SpendControlsForm({ account }: { account: string }) {
  const { data, isLoading } = useBillingSpend(account);
  const save = useSetBillingSpendThresholds(account);
  const [warning, setWarning] = useState<string | null>(null);
  const [limit, setLimit] = useState<string | null>(null);

  const spend = data?.available ? data.data : undefined;
  if (isLoading || !spend) return null;

  // null means untouched, so the field shows what the provider holds until the
  // owner edits it. Empty string is an edit that clears the threshold.
  const warningValue = warning ?? toDollars(spend.warning?.amount);
  const limitValue = limit ?? toDollars(spend.limit?.amount);

  function onSave() {
    const next = { warning: toCents(warningValue), limit: toCents(limitValue) };
    if ([next.warning, next.limit].some((v) => v != null && (Number.isNaN(v) || v < 0))) {
      toast.error("Enter an amount of zero or more.");
      return;
    }
    if (next.warning != null && next.limit != null && next.warning >= next.limit) {
      toast.error("The warning has to be below the limit, or the limit stops the account first.");
      return;
    }
    save.mutate(next, {
      // Back to reading through: the write seeds the cache with what it stored,
      // so the fields show the stored amount rather than the text that was typed.
      onSuccess: () => {
        setWarning(null);
        setLimit(null);
        toast.success("Spend controls saved");
      },
      onError: (err) => toast.error(getApiErrorMessage(err, "Could not save spend controls.")),
    });
  }

  return (
    <Card className="mb-6 flex flex-col gap-4 p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-heading-4 text-foreground">Spend controls</h3>
        {spend.has_usage_spend ? (
          <span className="text-body-sm text-muted-foreground">
            {formatMoney(spend.usage_spend, spend.currency)} used this period
          </span>
        ) : (
          spend.has_current_spend && (
            <span className="text-body-sm text-muted-foreground">
              {formatMoney(spend.current_spend, spend.currency)} this period
            </span>
          )
        )}
      </div>

      <div className="flex flex-wrap items-end gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="spend-warning">Warn me at</Label>
          <div className="flex items-center gap-2">
            <Input
              id="spend-warning"
              inputMode="decimal"
              placeholder="No warning"
              className="w-36"
              value={warningValue}
              onChange={(e) => setWarning(e.target.value)}
            />
            {spend.warning?.in_alarm && <InlineBadge variant="soft">Reached</InlineBadge>}
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="spend-limit">Stop agents at</Label>
          <div className="flex items-center gap-2">
            <Input
              id="spend-limit"
              inputMode="decimal"
              placeholder="No limit"
              className="w-36"
              value={limitValue}
              onChange={(e) => setLimit(e.target.value)}
            />
            {spend.limit?.in_alarm && <InlineBadge variant="soft">Reached</InlineBadge>}
          </div>
        </div>

        <Button onClick={onSave} disabled={save.isPending}>
          {save.isPending ? "Saving" : "Save"}
        </Button>
      </div>

      <p className="text-body-sm text-muted-foreground">
        A warning tells you and changes nothing. A limit stops every agent in this account until the
        next billing period, or until you raise it. Both apply to this account only, and both measure
        usage before any credit is applied, so they can trigger while your bill is still zero.
      </p>
    </Card>
  );
}
