/** Copy for the server's billing gating reasons, in one place so the account
 *  banner and the per-agent status cannot contradict each other. The server
 *  ranks the reasons (billing/status.go); the client only renders them. */

export interface BillingBannerCopy {
  title: string;
  body: string;
  cta: string;
}

/** Button label for the server's `action`. The server decides what resolves a
 *  gate (middleware.BillingAction); the client only names the button. Deriving
 *  the label from `reason` instead put "View billing" on balance_alert, where
 *  the server says only support can lift it.
 *
 *  contact_support keeps the billing destination because the app has no support
 *  route. The instruction lives in the body copy, which matches the server's
 *  `details` for that action. */
const ACTION_LABEL: Record<string, string> = {
  add_card: "Add payment method",
  update_card: "Update payment method",
  contact_support: "View billing",
  view_billing: "View billing",
};

export function billingActionLabel(action: string | undefined): string {
  return (action && ACTION_LABEL[action]) || "View billing";
}

/** Long-form copy for the app-wide banner. Credit exhaustion is split from the
 *  payment reasons because the fix differs: add a first card, not fix an
 *  existing one. Null for an unrecognised reason, so callers can fall back.
 *
 *  The reason picks the wording, the action picks the button. computeStatus
 *  pairs every non-active status with a reason (billing/status.go), and
 *  Recompute is its only writer, so a gated account always states one. */
export function billingBannerCopy(
  reason: string | undefined,
  action: string | undefined,
  suspended: boolean,
): BillingBannerCopy | null {
  const cta = billingActionLabel(action);
  if (reason === "credits_exhausted") {
    return {
      title: "Free credits used up",
      body: "Your agents are stopped. Add a payment method to switch to pay-as-you-go and start them again.",
      cta,
    };
  }
  if (reason === "uncollectible") {
    return {
      title: "Payment could not be collected",
      body: "Your agents are stopped after repeated failed charges. Update your payment method to restore them.",
      cta,
    };
  }
  if (reason === "balance_alert") {
    return {
      title: "Spending limit reached",
      body: "Your agents are stopped because the account hit its spend threshold. Contact support to raise it.",
      cta,
    };
  }
  if (reason === "dunning" || reason === "payment_failed") {
    return suspended
      ? {
          title: "Payment failed",
          body: "Your agents are stopped. Update your payment method to restore them.",
          cta,
        }
      : {
          title: "Payment failed",
          body: "We could not charge your card. Update it soon, or agents are stopped once the grace period ends.",
          cta,
        };
  }
  return null;
}

/** One line for a tooltip on a billing-stopped agent, keyed on the server's
 *  action for the same reason the banner's button is. The wording is
 *  deployment-scoped where the server's `details` is account-scoped, so the
 *  client still owns the sentence, but not the instruction inside it. An
 *  unknown action gets copy that holds whichever action it turns out to be. */
const ACTION_HINT: Record<string, string> = {
  add_card: "Stopped by billing. Add a payment method to start it again.",
  update_card: "Stopped by billing. Update your payment method to start it again.",
  contact_support: "Stopped by billing. Contact support to raise the spend limit.",
  view_billing: "Stopped by billing. Resolve billing to start it again.",
};

export function billingStoppedHint(action: string | undefined): string {
  return (action && ACTION_HINT[action]) || ACTION_HINT.view_billing;
}
