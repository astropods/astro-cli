import { describe, expect, it } from "vitest";
import { orgScope, resolvePageAccount } from "./user-resource-scope";

describe("orgScope", () => {
  it("selects the one account the session is scoped to", () => {
    expect(orgScope("acme")).toEqual({ accounts: ["acme"], all: false });
  });

  it("selects nothing before the account resolves, which disables the query", () => {
    expect(orgScope("")).toEqual({ accounts: [], all: false });
  });
});

describe("resolvePageAccount", () => {
  it("keeps a valid page-local account without changing the fallback", () => {
    expect(resolvePageAccount("team", ["personal", "team"], "personal")).toBe("team");
    expect(resolvePageAccount("foreign", ["personal", "team"], "personal")).toBe("personal");
  });
});
