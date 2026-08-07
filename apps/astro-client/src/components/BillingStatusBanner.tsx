/** App-wide banner for an account whose billing is gated. Suspension stops
 *  every agent in the account, not one page's worth, so this sits in the shell
 *  rather than on the billing page. Renders nothing while active, which is the
 *  normal case and the OSS case (the endpoint reports active with no store). */
import { useNavigate } from "react-router";
import { useBillingStatus } from "@/api/queries/billing";
import { useActiveAccount } from "@/hooks/use-active-account";
import { ActionPanel } from "@/components/ui/status-panel";

/** Copy per gating reason. Credit exhaustion is split out from the payment
 *  reasons because the fix is different: add a first card rather than fix an
 *  existing one. */
function bannerCopy(
  reason: string | undefined,
  creditsExhausted: boolean,
  hasPaymentMethod: boolean,
  suspended: boolean,
): { title: string; body: string; cta: string } | null {
  // The server ranks reasons (a write-off outranks exhaustion), so only infer
  // from the raw facts when it sent no reason. Inferring over a stated one tells
  // a written-off account to add a card, which does not lift force_suspended.
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
          body: "We could not charge your card. Update it soon — agents are stopped once the grace period ends.",
          cta: "Update payment method",
        };
  }
  return null;
}

export function BillingStatusBanner({ className }: { className?: string }) {
  const { activeAccount } = useActiveAccount();
  const navigate = useNavigate();
  const { data } = useBillingStatus(activeAccount ?? "");

  // Observe mode computes a status without acting on it, so there is nothing to
  // report — unless enforcement already stopped this account and was then
  // turned off, which leaves it genuinely down.
  const acted = data?.enforced || data?.workloads_suspended;
  if (!data || !acted || data.status === "active") return null;

  const suspended = data.status === "suspended";
  const copy = bannerCopy(
    data.reason,
    data.credits_exhausted,
    data.has_payment_method,
    suspended,
  );
  // An unrecognised reason means the server gained a status this build predates.
  // Staying silent would hide a stopped account, so fall back to generic copy.
  const { title, body, cta } = copy ?? {
    title: suspended ? "Billing issue — agents stopped" : "Billing issue",
    body: "There is a problem with this account's billing.",
    cta: "View billing",
  };

  return (
    <div className={className}>
      <ActionPanel
        title={title}
        primaryLabel={cta}
        onPrimary={() => navigate("/settings/billing")}
        tone={suspended ? "error" : "warning"}
      >
        {body}
      </ActionPanel>
    </div>
  );
}
