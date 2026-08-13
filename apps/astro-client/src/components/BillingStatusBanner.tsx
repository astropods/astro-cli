/** App-wide banner for an account whose billing is gated. Suspension stops
 *  every agent in the account, not one page's worth, so this sits in the shell
 *  rather than on the billing page. Renders nothing while active, which is the
 *  normal case and the OSS case (the endpoint reports active with no store). */
import { useNavigate } from "react-router";
import { useBillingStatus } from "@/api/queries/billing";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { ActionPanel } from "@/components/ui/status-panel";
import { billingBannerCopy } from "@/lib/billing-copy";
import { accountSettingsPath } from "@/lib/settings-paths";

export function BillingStatusBanner({ className }: { className?: string }) {
  const { activeAccount } = useActiveAccount();
  const { accounts } = useAuth();
  const navigate = useNavigate();
  const { data } = useBillingStatus(activeAccount ?? "");

  // Observe mode computes a status without acting on it, so there is nothing to
  // report — unless enforcement already stopped this account and was then
  // turned off, which leaves it genuinely down.
  const acted = data?.enforced || data?.workloads_suspended;
  if (!data || !acted || data.status === "active") return null;
  // activeAccount can come from the root loader before AuthProvider fills
  // accounts, and an unresolved account routes to the personal page. Waiting a
  // render beats sending an org owner somewhere their card cannot help.
  if (accounts.length === 0) return null;

  const suspended = data.status === "suspended";
  const copy = billingBannerCopy(
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
        // Scoped to the gated account. The status comes from activeAccount, so a
        // fixed personal path sends an org owner to a page where the card they
        // add cannot lift their org's suspension.
        onPrimary={() => navigate(accountSettingsPath(accounts, activeAccount ?? "", "billing"))}
        tone={suspended ? "error" : "warning"}
      >
        {body}
      </ActionPanel>
    </div>
  );
}
