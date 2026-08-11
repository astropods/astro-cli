import { describe, expect, it } from "vitest";
import { billingBannerCopy, billingStoppedHint } from "./billing-copy";

describe("billingStoppedHint", () => {
  // Every other reason already has a card, so "add one" cannot resolve it.
  it("only tells the owner to add a card when credits ran out", () => {
    expect(billingStoppedHint("credits_exhausted")).toContain("Add a payment method");
    for (const reason of ["payment_failed", "dunning", "uncollectible", "balance_alert"]) {
      expect(billingStoppedHint(reason)).not.toContain("Add a payment method");
    }
  });

  it("points a failed charge at updating the card", () => {
    expect(billingStoppedHint("payment_failed")).toContain("Update your payment method");
    expect(billingStoppedHint("uncollectible")).toContain("Update your payment method");
  });

  it("names the spend threshold rather than the card", () => {
    expect(billingStoppedHint("balance_alert")).toContain("spend threshold");
  });

  // A build that predates a new server reason still has to say something true.
  it("falls back to copy that holds for any reason", () => {
    expect(billingStoppedHint(undefined)).toBe(
      "Stopped by billing. Resolve billing to start it again.",
    );
    expect(billingStoppedHint("some_future_reason")).toBe(
      "Stopped by billing. Resolve billing to start it again.",
    );
  });
});

describe("billingBannerCopy", () => {
  it("infers exhaustion only when the server sent no reason", () => {
    expect(billingBannerCopy(undefined, true, false, true)?.title).toBe(
      "Free credits used up",
    );
    // A stated reason outranks the raw facts: a written-off account cannot lift
    // force_suspended by adding a card.
    expect(billingBannerCopy("uncollectible", true, false, true)?.title).toBe(
      "Payment could not be collected",
    );
  });

  it("returns null for an unrecognised reason so the caller can fall back", () => {
    expect(billingBannerCopy("some_future_reason", false, true, true)).toBeNull();
  });

  it("softens the payment_failed body while still in the grace period", () => {
    expect(billingBannerCopy("payment_failed", false, true, false)?.body).toContain(
      "grace period",
    );
    expect(billingBannerCopy("payment_failed", false, true, true)?.body).toContain(
      "Your agents are stopped",
    );
  });
});
