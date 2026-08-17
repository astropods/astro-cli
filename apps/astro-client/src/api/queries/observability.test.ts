import { describe, expect, it } from "vitest";
import { observabilityKeys } from "./keys";

describe("observabilityKeys.insights", () => {
  const params = { q: "acme" };

  // The SSR loader primes this key, so its shape is a contract between the
  // loader and the page: a mismatch turns the primed entry into a silent miss.
  it("keys on account and params", () => {
    expect(observabilityKeys.insights("acme", params)).toEqual([
      "observability",
      "insights",
      "acme",
      params,
    ]);
    expect(observabilityKeys.insights("acme")).toEqual(["observability", "insights", "acme"]);
  });

  it("separates accounts and params", () => {
    expect(observabilityKeys.insights("acme", params)).not.toEqual(
      observabilityKeys.insights("other", params),
    );
    expect(observabilityKeys.insights("acme", { q: "a" })).not.toEqual(
      observabilityKeys.insights("acme", { q: "b" }),
    );
  });
});
