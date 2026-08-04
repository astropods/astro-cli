import { describe, expect, it } from "vitest";
import { removeStaleInsightsAccountParam, resolveInsightsScopeAccount } from "./insights-account-param";

describe("Insights account URL state", () => {
  it("uses a valid page-local account and falls back for a stale one", () => {
    expect(resolveInsightsScopeAccount("team", ["personal", "team"], "personal")).toBe("team");
    expect(resolveInsightsScopeAccount("stale", ["personal", "team"], "personal")).toBe("personal");
  });

  it("removes only a stale account parameter", () => {
    const next = removeStaleInsightsAccountParam(
      new URLSearchParams("account=stale&range=30d"),
      ["personal"],
    );
    expect(next?.toString()).toBe("range=30d");
  });
});
