import type { BillingRecord } from "./api";

/** The fields of a credit or commit that mean anything to an account owner.
 *  The provider returns a dozen more, including internal ids and the name of
 *  the API key that granted it. */
export interface BalanceRow {
  name: string;
  granted?: number;
  remaining?: number;
  expires?: string;
  /** Provider display label, e.g. "USD (cents)". */
  creditType?: string;
  creditTypeId?: string;
}

/** Metronome's built-in fiat unit, in hundredths. astro-server keys on this id
 *  too (internal/billing/metronome/spend.go), because the credit type carries
 *  no precision field and the name is the provider's to reword. */
const USD_CENTS_CREDIT_TYPE_ID = "2714e483-4ff1-48e4-9e25-ac732e8f24f2";

export type CreditUnit =
  | { kind: "money"; currency: string; scale: number }
  | { kind: "other"; label: string };

/** Prefers the credit type's id, falling back to its label for a provider that
 *  sends one without the other. An unrecognized type is not money: reporting
 *  its amount as dollars is the one answer that can't be right. */
export function creditUnit(creditType?: string, creditTypeId?: string): CreditUnit {
  if (creditTypeId === USD_CENTS_CREDIT_TYPE_ID) {
    return { kind: "money", currency: "USD", scale: 100 };
  }
  const label = creditType?.trim() ?? "";
  if (/usd/i.test(label)) {
    return { kind: "money", currency: "USD", scale: /cents?/i.test(label) ? 100 : 1 };
  }
  return { kind: "other", label: label || "credits" };
}

export function formatCreditAmount(
  value: number | undefined,
  creditType?: string,
  creditTypeId?: string,
): string {
  if (typeof value !== "number") return "—";
  const unit = creditUnit(creditType, creditTypeId);
  if (unit.kind === "money") {
    return (value / unit.scale).toLocaleString(undefined, {
      style: "currency",
      currency: unit.currency,
    });
  }
  return creditType ? `${value.toLocaleString()} ${creditType}` : value.toLocaleString();
}

export function toBalanceRow(record: BillingRecord): BalanceRow {
  const items = record.access_schedule?.schedule_items ?? [];
  return {
    name: record.name || record.product?.name || "Credit",
    granted: items.length
      ? items.reduce((sum, item) => sum + (item.amount ?? 0), 0)
      : undefined,
    remaining: record.balance,
    // Latest end date across segments: the credit is gone after the last one.
    expires: items
      .map((item) => item.ending_before)
      .filter((d): d is string => !!d)
      .sort()
      .pop(),
    creditType: record.access_schedule?.credit_type?.name,
    creditTypeId: record.access_schedule?.credit_type?.id,
  };
}

/** Thresholds (SpendThreshold.amount) are stored in the credit type's own
 *  unit, Metronome's USD-cents pricing unit (see spend_thresholds.go), while
 *  every other figure on BillingSpend already arrived in whole dollars.
 *  Converting once here, wherever a threshold enters the UI, keeps every card
 *  and dialog that reads one from disagreeing about its scale. */
export function thresholdDollars(cents: number | undefined): number | undefined {
  return typeof cents === "number" ? cents / 100 : undefined;
}

/** Formats an amount already reduced to whole currency units. */
export function formatMoney(amount: number, currency: string): string {
  return amount.toLocaleString(undefined, { style: "currency", currency });
}
