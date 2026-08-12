import type { BillingRecord } from "./api";

/** The fields of a credit or commit that mean anything to an account owner.
 *  The provider returns a dozen more, including internal ids and the name of
 *  the API key that granted it. */
export interface BalanceRow {
  name: string;
  granted?: number;
  remaining?: number;
  expires?: string;
  creditType?: string;
}

interface ProviderSchedule {
  credit_type?: { name?: string };
  schedule_items?: { amount?: number; ending_before?: string }[];
}

export function toBalanceRow(record: BillingRecord): BalanceRow {
  const schedule = record.access_schedule as ProviderSchedule | undefined;
  const items = schedule?.schedule_items ?? [];
  const product = record.product as { name?: string } | undefined;
  return {
    name: (typeof record.name === "string" && record.name) || product?.name || "Credit",
    granted: items.length
      ? items.reduce((sum, item) => sum + (item.amount ?? 0), 0)
      : undefined,
    remaining: typeof record.balance === "number" ? record.balance : undefined,
    // Latest end date across segments: the credit is gone after the last one.
    expires: items
      .map((item) => item.ending_before)
      .filter((d): d is string => !!d)
      .sort()
      .pop(),
    creditType: schedule?.credit_type?.name,
  };
}

/** Amounts come back in the credit type's own unit; "USD (cents)" means the
 *  value is cents. */
export function formatCreditAmount(value: number | undefined, creditType?: string): string {
  if (typeof value !== "number") return "—";
  const amount = /cents/i.test(creditType ?? "") ? value / 100 : value;
  if (/usd/i.test(creditType ?? "")) {
    return amount.toLocaleString(undefined, { style: "currency", currency: "USD" });
  }
  return creditType ? `${amount.toLocaleString()} ${creditType}` : amount.toLocaleString();
}
