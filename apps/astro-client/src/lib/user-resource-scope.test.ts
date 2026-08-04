import { describe, expect, it } from "vitest";
import {
  canonicalizeUserResourceAccounts,
  resolvePageAccount,
  resolveUserResourceScope,
} from "./user-resource-scope";

describe("canonicalizeUserResourceAccounts", () => {
  it("collapses a complete selection to the canonical all-accounts value", () => {
    expect(canonicalizeUserResourceAccounts(
      ["beta", "alpha", "beta", "foreign"],
      ["alpha", "beta"],
    )).toEqual([]);
  });
});

describe("resolveUserResourceScope", () => {
  it("uses an explicit, canonical selected-account scope", () => {
    expect(resolveUserResourceScope(["beta", "alpha", "beta", "foreign"], ["alpha", "beta", "gamma"]))
      .toEqual({ accounts: ["alpha", "beta"], all: false });
  });

  it("treats an empty, stale, or complete selection as all memberships", () => {
    expect(resolveUserResourceScope(["foreign"], ["beta", "alpha"]))
      .toEqual({ accounts: ["alpha", "beta"], all: true });
    expect(resolveUserResourceScope(["beta", "alpha"], ["alpha", "beta"]))
      .toEqual({ accounts: ["alpha", "beta"], all: true });
  });
});

describe("resolvePageAccount", () => {
  it("keeps a valid page-local account without changing the fallback", () => {
    expect(resolvePageAccount("team", ["personal", "team"], "personal")).toBe("team");
    expect(resolvePageAccount("foreign", ["personal", "team"], "personal")).toBe("personal");
  });
});
