import { describe, expect, it } from "vitest";
import {
  billingActionLabel,
  billingBannerCopy,
  billingStoppedHint,
  canManageAccountBilling,
  canManageBilling,
} from "./billing-copy";

describe("canManageBilling", () => {
  it("passes only for an org admin or owner role", () => {
    expect(canManageBilling("admin")).toBe(true);
    expect(canManageBilling("owner")).toBe(true);
    expect(canManageBilling("member")).toBe(false);
    expect(canManageBilling(null)).toBe(false);
    expect(canManageBilling(undefined)).toBe(false);
  });
});

describe("canManageAccountBilling", () => {
  it("lets a personal account's owner manage its own billing regardless of role", () => {
    expect(canManageAccountBilling("brendan", "brendan", null)).toBe(true);
    expect(canManageAccountBilling("brendan", "brendan", "member")).toBe(true);
  });

  it("falls back to the org-role check for any other account", () => {
    expect(canManageAccountBilling("acme", "brendan", "admin")).toBe(true);
    expect(canManageAccountBilling("acme", "brendan", "owner")).toBe(true);
    expect(canManageAccountBilling("acme", "brendan", "member")).toBe(false);
    expect(canManageAccountBilling("acme", "brendan", null)).toBe(false);
  });

  it("does not treat an unresolved personal account name as matching every account", () => {
    expect(canManageAccountBilling("acme", undefined, "member")).toBe(false);
  });
});

describe("billingStoppedHint", () => {
  it("only tells the owner to add a card for add_card", () => {
    expect(billingStoppedHint("add_card")).toContain("Add a payment method");
    for (const action of ["update_card", "contact_support", "view_billing"]) {
      expect(billingStoppedHint(action)).not.toContain("Add a payment method");
    }
  });

  it("points update_card at the existing card", () => {
    expect(billingStoppedHint("update_card")).toContain("Update your payment method");
  });

  it("tells a non-self-serve account to contact support", () => {
    expect(billingStoppedHint("contact_support")).toContain("Contact support");
  });

  it("falls back to the non-self-serve copy for an unknown action", () => {
    const fallback = "Stopped by billing. Contact support to start it again.";
    expect(billingStoppedHint(undefined)).toBe(fallback);
    expect(billingStoppedHint("some_future_action")).toBe(fallback);
  });
});

describe("billingBannerCopy", () => {
  it("renders wording from the server's reason", () => {
    expect(billingBannerCopy("credits_exhausted", "add_card", true)?.title).toBe(
      "Free credits used up",
    );
    expect(billingBannerCopy("uncollectible", "update_card", true)?.title).toBe(
      "Payment could not be collected",
    );
    expect(billingBannerCopy(undefined, "view_billing", true)).toBeNull();
  });

  it("takes the button from the server's action, not the reason", () => {
    expect(billingBannerCopy("credits_exhausted", "add_card", true)?.cta).toBe(
      "Add payment method",
    );
    expect(billingBannerCopy("credits_exhausted", "update_card", true)?.cta).toBe(
      "Update payment method",
    );
    expect(billingActionLabel(undefined)).toBe("View billing");
    expect(billingActionLabel("some_future_action")).toBe("View billing");
  });

  it("tells a spend-threshold account to contact support", () => {
    expect(billingBannerCopy("balance_alert", "contact_support", true)?.body).toContain(
      "Contact support",
    );
  });

  it("separates a limit the account set from an alert only support can lift", () => {
    const own = billingBannerCopy("usage_limit", "raise_usage_limit", true);
    expect(own?.body).toContain("a limit it set");
    expect(own?.cta).toBe("Change usage limit");
    expect(billingBannerCopy("balance_alert", "contact_support", true)?.body).toContain(
      "Contact support",
    );
  });

  it("names a missing plan rather than blaming a payment method", () => {
    const copy = billingBannerCopy("not_provisioned", "contact_support", true);
    expect(copy?.title).toBe("Billing is not set up");
    expect(copy?.body).toContain("no billing plan covers this account");
  });

  it("returns null for an unrecognised reason so the caller can fall back", () => {
    expect(billingBannerCopy("some_future_reason", "view_billing", true)).toBeNull();
  });

  it("asks for confirmation rather than a new card when a charge needs authentication", () => {
    const copy = billingBannerCopy("payment_failed", "complete_payment", true);
    expect(copy?.title).toContain("confirmation");
    expect(copy?.body).not.toContain("payment method");
    expect(copy?.body).toContain("bank");
  });

  it("softens the payment_failed body while still in the grace period", () => {
    expect(billingBannerCopy("payment_failed", "update_card", false)?.body).toContain(
      "grace period",
    );
    expect(billingBannerCopy("payment_failed", "update_card", true)?.body).toContain(
      "Your agents are stopped",
    );
  });
});
