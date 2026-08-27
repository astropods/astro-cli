import { describe, it, expect } from "vitest";
import {
  accountSettingsPath,
  settingsScopePath,
  settingsSectionFromPath,
} from "./settings-paths";
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

describe("settingsSectionFromPath", () => {
  it("reads the personal section", () => {
    expect(settingsSectionFromPath("/settings/billing")).toBe("billing");
  });

  it("reads the org section", () => {
    expect(settingsSectionFromPath("/settings/org/acme-org/audit-log")).toBe("audit-log");
  });

  it("falls back to account off a settings path", () => {
    expect(settingsSectionFromPath("/settings")).toBe("account");
    expect(settingsSectionFromPath("/agents")).toBe("account");
  });
});

describe("settingsScopePath", () => {
  it("keeps a section that exists in both scopes", () => {
    expect(settingsScopePath(accounts, "acme-org", "billing")).toBe("/settings/org/acme-org/billing");
    expect(settingsScopePath(accounts, "alice", "audit-log")).toBe("/settings/audit-log");
  });

  it("maps the personal Account page onto the org General page", () => {
    expect(settingsScopePath(accounts, "acme-org", "account")).toBe("/settings/org/acme-org/general");
    expect(settingsScopePath(accounts, "alice", "general")).toBe("/settings/account");
  });

  it("falls back to the landing section when the counterpart is missing", () => {
    expect(settingsScopePath(accounts, "acme-org", "connectors")).toBe("/settings/org/acme-org/general");
    expect(settingsScopePath(accounts, "acme-org", "organizations")).toBe("/settings/org/acme-org/general");
    expect(settingsScopePath(accounts, "alice", "members")).toBe("/settings/account");
  });

  it("treats an unknown account as personal", () => {
    expect(settingsScopePath(accounts, "ghost", "usage")).toBe("/settings/usage");
  });
});
