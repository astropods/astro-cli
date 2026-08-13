import { describe, expect, it } from "vitest";
import { billingActionLabel, billingBannerCopy, billingStoppedHint } from "./billing-copy";

describe("billingStoppedHint", () => {
  // add_card is the one action that means no card is on file, so it is the one
  // hint that may tell the owner to add one.
  it("only tells the owner to add a card for add_card", () => {
    expect(billingStoppedHint("add_card")).toContain("Add a payment method");
    for (const action of ["update_card", "contact_support", "view_billing"]) {
      expect(billingStoppedHint(action)).not.toContain("Add a payment method");
    }
  });

  it("points update_card at the existing card", () => {
    expect(billingStoppedHint("update_card")).toContain("Update your payment method");
  });

  // The server returns contact_support for a spend limit because only an
  // operator can raise it. Naming the threshold without naming the fix left the
  // owner with nothing to do.
  it("tells a spend-limit account to contact support", () => {
    expect(billingStoppedHint("contact_support")).toContain("Contact support");
  });

  // A build that predates a new server action still has to say something true.
  it("falls back to copy that holds for any action", () => {
    expect(billingStoppedHint(undefined)).toBe(
      "Stopped by billing. Resolve billing to start it again.",
    );
    expect(billingStoppedHint("some_future_action")).toBe(
      "Stopped by billing. Resolve billing to start it again.",
    );
  });
});

describe("billingBannerCopy", () => {
  // The reason picks the wording; there is nothing to infer, because
  // computeStatus always states one for a gated account.
  it("renders wording from the server's reason", () => {
    expect(billingBannerCopy("credits_exhausted", "add_card", true)?.title).toBe(
      "Free credits used up",
    );
    expect(billingBannerCopy("uncollectible", "update_card", true)?.title).toBe(
      "Payment could not be collected",
    );
    expect(billingBannerCopy(undefined, "view_billing", true)).toBeNull();
  });

  // The server decides what lifts a gate. Deriving the button from the reason
  // put "View billing" on balance_alert, where only support can lift it.
  it("takes the button from the server's action, not the reason", () => {
    expect(billingBannerCopy("credits_exhausted", "add_card", true)?.cta).toBe(
      "Add payment method",
    );
    // Same reason, different server verdict: the button follows the verdict.
    expect(billingBannerCopy("credits_exhausted", "update_card", true)?.cta).toBe(
      "Update payment method",
    );
    expect(billingActionLabel(undefined)).toBe("View billing");
    expect(billingActionLabel("some_future_action")).toBe("View billing");
  });

  // The app has no support route, so the button cannot carry this instruction.
  // It has to appear in the body, matching the server's details for the action.
  it("tells a spend-threshold account to contact support", () => {
    expect(billingBannerCopy("balance_alert", "contact_support", true)?.body).toContain(
      "Contact support",
    );
  });

  it("returns null for an unrecognised reason so the caller can fall back", () => {
    expect(billingBannerCopy("some_future_reason", "view_billing", true)).toBeNull();
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
