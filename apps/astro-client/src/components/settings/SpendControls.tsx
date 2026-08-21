import { useState } from "react";
import { toast } from "sonner";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { InlineBadge } from "@/components/InlineBadge";
import {
  useBillingSpend,
  useSetBillingSpendThresholds,
  useSetBillingUsageThresholds,
} from "@/api/queries/billing";
import { getApiErrorMessage } from "@/lib/api";
import type { BillingSpend, SpendThreshold } from "@/lib/api";

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

function toCents(input: string): number | null {
  const trimmed = input.trim();
  if (trimmed === "") return null;
  return Math.round(Number(trimmed) * 100);
}

function toDollars(cents: number | undefined): string {
  return cents == null ? "" : String(cents / 100);
}

/** Quantities travel as typed. Applying the spend conversion here would cap the
 *  account at a hundredth of what it asked for. */
function toQuantity(input: string): number | null {
  const trimmed = input.trim();
  return trimmed === "" ? null : Number(trimmed);
}

function toField(amount: number | undefined): string {
  return amount == null ? "" : String(amount);
}

interface Row {
  key: string;
  label: string;
  help: string;
  warning?: SpendThreshold;
  limit?: SpendThreshold;
}

function rowsFor(spend: BillingSpend): Row[] {
  const rows: Row[] = [];
  // A plan that rates everything at zero can never fire a spend control, and a
  // field that provably cannot fire is worse than no field.
  if (spend.plan !== "unlimited") {
    rows.push({
      key: "spend",
      label: "Spend",
      help: "before credit is applied",
      warning: spend.warning,
      limit: spend.limit,
    });
  }
  for (const [key, label] of [
    ["compute", "Compute"],
    ["gateway", "AI Gateway"],
  ] as const) {
    const held = spend.usage?.[key];
    rows.push({
      key,
      label,
      help: held?.unit ?? (key === "compute" ? "CU-hours" : "USD of model usage"),
      warning: held?.warning,
      limit: held?.limit,
    });
  }
  return rows;
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
  const saveSpend = useSetBillingSpendThresholds(account);
  const saveUsage = useSetBillingUsageThresholds(account);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [stillStopped, setStillStopped] = useState(false);

  const spend = data?.available ? data.data : undefined;
  if (isLoading || !spend) return null;

  const rows = rowsFor(spend);
  const pending = saveSpend.isPending || saveUsage.isPending;
  const edited = Object.keys(edits).length > 0;
  const value = (row: Row, kind: "warning" | "limit") =>
    edits[`${row.key}.${kind}`] ??
    (row.key === "spend" ? toDollars(row[kind]?.amount) : toField(row[kind]?.amount));

  function onSave() {
    const parsed = rows.map((row) => {
      const parse = row.key === "spend" ? toCents : toQuantity;
      return { row, warning: parse(value(row, "warning")), limit: parse(value(row, "limit")) };
    });

    for (const { warning, limit } of parsed) {
      if ([warning, limit].some((v) => v != null && (Number.isNaN(v) || v < 0))) {
        toast.error("Enter an amount of zero or more.");
        return;
      }
      if (warning != null && limit != null && warning >= limit) {
        toast.error("The warning has to be below the limit, or the limit stops the account first.");
        return;
      }
    }

    const touched = parsed.filter(
      ({ row }) => `${row.key}.warning` in edits || `${row.key}.limit` in edits,
    );
    // Nothing edited means this is a lift retry: re-send the stored numbers,
    // which the provider treats as a no-op.
    const saving = touched.length > 0 ? touched : stillStopped ? parsed : [];
    if (saving.length === 0) return;

    void Promise.allSettled(
      saving.map(({ row, warning, limit }) =>
        row.key === "spend"
          ? saveSpend.mutateAsync({ warning, limit })
          : saveUsage.mutateAsync({ metric: row.key, warning, limit }),
      ),
    ).then((results) => {
      const failed = results
        .map((result, i) => ({ result, row: saving[i]!.row }))
        .filter(({ result }) => result.status === "rejected");

      // Every field goes back to reading through the cache, whatever happened.
      // A partial save writes some rows and refuses others, and holding the
      // refused text on screen would show a number the provider does not have
      // next to numbers it does.
      setEdits({});

      const stopped = results.some(
        (r) => r.status === "fulfilled" && r.value?.limit_lift_failed === true,
      );
      setStillStopped(stopped);

      if (failed.length === 0) {
        if (stopped) {
          toast.warning("Limits saved, but your agents are still stopped. Save again to retry.");
        } else {
          toast.success("Limits saved");
        }
        return;
      }
      const [first] = failed;
      const reason = getApiErrorMessage(
        (first!.result as PromiseRejectedResult).reason,
        "Could not save limits.",
      );
      toast.error(
        failed.length === results.length
          ? reason
          : `Saved all but ${failed.map(({ row }) => row.label).join(" and ")}. ${reason}`,
      );
    });
  }

  return (
    <Card className="mb-6 flex flex-col gap-4 p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-heading-4 text-foreground">Limits</h3>
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

      {rows.map((row) => (
        <div key={row.key} className="flex flex-wrap items-end gap-4">
          <div className="min-w-36">
            <div className="text-body-sm text-foreground">{row.label}</div>
            <div className="text-body-sm text-muted-foreground">{row.help}</div>
          </div>
          {(["warning", "limit"] as const).map((kind) => (
            <div key={kind} className="flex flex-col gap-1.5">
              <Label htmlFor={`${row.key}-${kind}`}>
                {row.label}: {kind === "warning" ? "warn me at" : "stop agents at"}
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  id={`${row.key}-${kind}`}
                  inputMode="decimal"
                  placeholder={kind === "warning" ? "No warning" : "No limit"}
                  className="w-36"
                  value={value(row, kind)}
                  onChange={(e) =>
                    setEdits((prev) => ({ ...prev, [`${row.key}.${kind}`]: e.target.value }))
                  }
                />
                {row[kind]?.in_alarm && <InlineBadge variant="soft">Reached</InlineBadge>}
              </div>
            </div>
          ))}
        </div>
      ))}

      <div>
        <Button onClick={onSave} disabled={(!edited && !stillStopped) || pending}>
          {pending ? "Saving" : "Save"}
        </Button>
      </div>

      <p className="text-body-sm text-muted-foreground">
        A warning tells you and changes nothing. A limit stops every agent in this account until the
        next billing period, or until you raise it. Spend measures money before any credit is
        applied, so it can trigger while your bill is still zero. The others count usage itself, so
        they apply even on a plan that is billed nothing.
      </p>
    </Card>
  );
}
