import { describe, expect, it } from "vitest";
import { creditUnit, formatCreditAmount } from "@/lib/billing-balances";

describe("formatCreditAmount", () => {
  // The provider reports USD in cents, so a $10 credit arrives as 1000.
  it("renders a USD cents amount as dollars", () => {
    expect(formatCreditAmount(1000, "USD (cents)")).toBe("$10.00");
    expect(formatCreditAmount(9.1395, "USD (cents)")).toBe("$0.09");
  });

  it("renders zero rather than treating it as missing", () => {
    expect(formatCreditAmount(0, "USD (cents)")).toBe("$0.00");
    expect(formatCreditAmount(undefined, "USD (cents)")).toBe("—");
  });

  // An unknown credit type must not be presented as dollars.
  it("keeps a non-USD credit type as a plain quantity", () => {
    expect(formatCreditAmount(250, "Tokens")).toBe("250 Tokens");
    expect(formatCreditAmount(250)).toBe("250");
  });
});

const USD_CENTS_ID = "2714e483-4ff1-48e4-9e25-ac732e8f24f2";

describe("creditUnit", () => {
  // astro-server keys on the id for the same reason: the name is the
  // provider's to reword.
  it("trusts the id over a label that says otherwise", () => {
    expect(creditUnit("Renamed by the dashboard", USD_CENTS_ID)).toEqual({
      kind: "money",
      currency: "USD",
      scale: 100,
    });
  });

  it("falls back to the label when no id came through", () => {
    expect(creditUnit("USD (cents)")).toEqual({ kind: "money", currency: "USD", scale: 100 });
    expect(creditUnit("USD")).toEqual({ kind: "money", currency: "USD", scale: 1 });
  });

  it("treats an unrecognized type as something other than money", () => {
    expect(creditUnit("Tokens")).toEqual({ kind: "other", label: "Tokens" });
    expect(creditUnit()).toEqual({ kind: "other", label: "credits" });
  });
});
