/** Copy for the server's billing gating reasons, in one place so the account
 *  banner and the per-agent status cannot contradict each other. The server
 *  ranks the reasons (billing/status.go); the client only renders them. */

export interface BillingBannerCopy {
  title: string;
  body: string;
  cta: string;
}

/** Long-form copy for the app-wide banner. Credit exhaustion is split from the
 *  payment reasons because the fix differs: add a first card, not fix an
 *  existing one. Null for an unrecognised reason, so callers can fall back. */
export function billingBannerCopy(
  reason: string | undefined,
  creditsExhausted: boolean,
  hasPaymentMethod: boolean,
  suspended: boolean,
): BillingBannerCopy | null {
  // Infer from raw facts only when the server sent no reason. Inferring over a
  // stated one tells a written-off account to add a card, which does not lift
  // force_suspended.
  if (reason === "credits_exhausted" || (!reason && creditsExhausted && !hasPaymentMethod)) {
    return {
      title: "Free credits used up",
      body: "Your agents are stopped. Add a payment method to switch to pay-as-you-go and start them again.",
      cta: "Add payment method",
    };
  }
  if (reason === "uncollectible") {
    return {
      title: "Payment could not be collected",
      body: "Your agents are stopped after repeated failed charges. Update your payment method to restore them.",
      cta: "Update payment method",
    };
  }
  if (reason === "balance_alert") {
    return {
      title: "Spending limit reached",
      body: "Your agents are stopped because the account hit its spend threshold.",
      cta: "View billing",
    };
  }
  if (reason === "dunning" || reason === "payment_failed") {
    return suspended
      ? {
          title: "Payment failed",
          body: "Your agents are stopped. Update your payment method to restore them.",
          cta: "Update payment method",
        }
      : {
          title: "Payment failed",
          body: "We could not charge your card. Update it soon, or agents are stopped once the grace period ends.",
          cta: "Update payment method",
        };
  }
  return null;
}

/** One line for a tooltip on a billing-stopped agent. Only credits_exhausted
 *  implies no card on file; the other reasons already have one. An unknown
 *  reason gets copy that holds whichever reason it turns out to be. */
export function billingStoppedHint(reason: string | undefined): string {
  switch (reason) {
    case "credits_exhausted":
      return "Stopped by billing. Add a payment method to start it again.";
    case "balance_alert":
      return "Stopped by billing. The account hit its spend threshold.";
    case "uncollectible":
    case "dunning":
    case "payment_failed":
      return "Stopped by billing. Update your payment method to start it again.";
    default:
      return "Stopped by billing. Resolve billing to start it again.";
  }
}
