/** App-wide banner for an account whose billing is gated. Suspension stops
 *  every agent in the account, not one page's worth, so this sits in the shell
 *  rather than on the billing page. Renders nothing while active, which is the
 *  normal case and the OSS case (the endpoint reports active with no store). */
import { useNavigate } from "react-router";
import { useBillingStatus } from "@/api/queries/billing";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { ActionPanel } from "@/components/ui/status-panel";
import { billingActionLabel, billingBannerCopy } from "@/lib/billing-copy";
import { accountSettingsPath } from "@/lib/settings-paths";

export function BillingStatusBanner({ className }: { className?: string }) {
  const { activeAccount } = useActiveAccount();
  const { accounts } = useAuth();
  const navigate = useNavigate();
  const { data } = useBillingStatus(activeAccount ?? "");

  // The server owns the rule, so the banner, the per-agent status, and the 402
  // cannot disagree about whether an account is gated. A server that predates
  // `gated` sends undefined, and treating that as "not gated" would hide a real
  // suspension; the client and server deploy independently, so reconstruct the
  // rule for that case only. Drop the fallback once no deployed server predates
  // the field.
  const gated =
    data?.gated ?? (data ? data.status !== "active" && (data.enforced || data.workloads_suspended) : false);
  if (!gated) return null;
  if (!data) return null;
  // activeAccount can come from the root loader before AuthProvider fills
  // accounts, and an unresolved account routes to the personal page. Waiting a
  // render beats sending an org owner somewhere their card cannot help.
  if (accounts.length === 0) return null;

  const suspended = data.status === "suspended";
  const copy = billingBannerCopy(data.reason, data.action, suspended);
  // An unrecognised reason means the server gained a status this build predates.
  // Staying silent would hide a stopped account, so fall back to generic copy.
  const { title, body, cta } = copy ?? {
    title: suspended ? "Billing issue, agents stopped" : "Billing issue",
    body: "There is a problem with this account's billing.",
    cta: billingActionLabel(data.action),
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
