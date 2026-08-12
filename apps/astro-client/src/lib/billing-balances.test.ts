import { describe, expect, it } from "vitest";
import { formatCreditAmount, toBalanceRow } from "@/lib/billing-balances";
import type { BillingRecord } from "@/lib/api";

// Shaped like a Metronome credit: the fields worth showing are nested, and the
// flat ones are mostly internal.
const credit: BillingRecord = {
  id: "418f4868-0b21-506d-be58-9f239f86b079",
  name: "Signup credit",
  balance: 0,
  created_by: "Matt Station API Full Key",
  priority: 1,
  rate_type: "LIST_RATE",
  type: "CREDIT",
  product: { name: "Signup credit" },
  access_schedule: {
    credit_type: { name: "USD (cents)" },
    schedule_items: [
      {
        amount: 1000,
        starting_at: "2026-08-11T14:00:00+00:00",
        ending_before: "2027-08-11T14:00:00+00:00",
      },
    ],
  },
};

describe("toBalanceRow", () => {
  it("projects the four fields an owner cares about", () => {
    expect(toBalanceRow(credit)).toEqual({
      name: "Signup credit",
      granted: 1000,
      remaining: 0,
      expires: "2027-08-11T14:00:00+00:00",
      creditType: "USD (cents)",
    });
  });

  // A spent credit is 0, not missing. Treating it as absent would render an
  // em dash where the owner needs to see that nothing is left.
  it("keeps a zero balance distinct from an absent one", () => {
    expect(toBalanceRow(credit).remaining).toBe(0);
    expect(toBalanceRow({ ...credit, balance: undefined }).remaining).toBeUndefined();
  });

  it("takes the latest end date across segments", () => {
    const multi = {
      ...credit,
      access_schedule: {
        credit_type: { name: "USD (cents)" },
        schedule_items: [
          { amount: 400, ending_before: "2026-09-01T00:00:00+00:00" },
          { amount: 600, ending_before: "2026-12-01T00:00:00+00:00" },
        ],
      },
    };
    const row = toBalanceRow(multi);
    expect(row.granted).toBe(1000);
    expect(row.expires).toBe("2026-12-01T00:00:00+00:00");
  });

  it("falls back to the product name, then a generic label", () => {
    expect(toBalanceRow({ ...credit, name: undefined }).name).toBe("Signup credit");
    expect(toBalanceRow({ balance: 5 }).name).toBe("Credit");
  });

  it("reports no schedule as absent rather than zero granted", () => {
    expect(toBalanceRow({ balance: 5 }).granted).toBeUndefined();
  });
});

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
