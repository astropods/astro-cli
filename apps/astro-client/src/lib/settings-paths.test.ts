import { describe, it, expect } from "vitest";
import { accountSettingsPath } from "./settings-paths";
import type { Account } from "@/lib/api";

const accounts: Account[] = [
  { id: "acct-1", name: "alice", type: "personal" },
  { id: "acct-2", name: "acme-org", type: "organization", organization_id: "org-2" },
];

describe("accountSettingsPath", () => {
  it("uses the top-level path for personal accounts", () => {
    expect(accountSettingsPath(accounts, "alice", "billing")).toBe("/settings/billing");
  });

  it("uses the org-scoped path for organization accounts", () => {
    expect(accountSettingsPath(accounts, "acme-org", "billing")).toBe("/settings/org/acme-org/billing");
  });

  it("scopes by section", () => {
    expect(accountSettingsPath(accounts, "acme-org", "secrets")).toBe("/settings/org/acme-org/secrets");
    expect(accountSettingsPath(accounts, "alice", "secrets")).toBe("/settings/secrets");
  });

  it("defaults to the personal path for unknown accounts", () => {
    expect(accountSettingsPath(accounts, "ghost", "billing")).toBe("/settings/billing");
  });
});
