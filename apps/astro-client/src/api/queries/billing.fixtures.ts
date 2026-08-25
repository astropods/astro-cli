import type { BillingDataResponse, BillingSpend } from "@/lib/api";

// Neutral defaults (no credit, no thresholds, no spend); callers override
// whatever they're actually testing.
export function buildBillingSpend(partial: Partial<BillingSpend> = {}): BillingSpend {
  return {
    plan: "no_credit",
    current_spend: 0,
    has_current_spend: true,
    usage_spend: 0,
    has_usage_spend: true,
    credit_remaining: 0,
    has_credit: false,
    has_last_invoice: false,
    current_period_end: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

export function buildSpendResponse(
  partial: Partial<BillingSpend> = {},
): BillingDataResponse<BillingSpend> {
  return { available: true, data: buildBillingSpend(partial) };
}
