/** The billing provider's own vocabulary, in one place so a rename is one edit
 *  and a comparison cannot drift from the value it is compared against. */

/** Compute is a quantity metric (cu_hours); the gateway one aggregates upstream
 *  cost in dollars, which is why only Compute has a unit to show. */
export const METRIC_COMPUTE = "Compute Units";
export const METRIC_GATEWAY = "LLM Usage";

export const PRODUCT_COMPUTE = "Compute Units";

/** Says whether the invoice is closed, never whether anyone paid it. */
export const INVOICE_DRAFT = "DRAFT";
export const INVOICE_FINALIZED = "FINALIZED";
export const INVOICE_VOID = "VOID";
export const INVOICE_PAID = "PAID";

/** The only field that reports whether an invoice was collected. */
export const EXTERNAL_PAID = "PAID";
export const EXTERNAL_PARTIALLY_PAID = "PARTIALLY_PAID";
export const EXTERNAL_PAYMENT_FAILED = "PAYMENT_FAILED";
export const EXTERNAL_UNCOLLECTIBLE = "UNCOLLECTIBLE";
export const EXTERNAL_VOID = "VOID";
export const EXTERNAL_DELETED = "DELETED";
export const EXTERNAL_SKIPPED = "SKIPPED";
export const EXTERNAL_INVALID_REQUEST = "INVALID_REQUEST_ERROR";

export const DEFAULT_CURRENCY = "USD";

/** Provider strings, not enums, so every comparison normalises. */
export function normalizeStatus(status?: string | null): string {
  return (status ?? "").toUpperCase();
}
