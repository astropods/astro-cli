import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { getActiveAccount } from "./api.server";
import { ApiClient, type AuthResponse } from "./api";

// AuthResponse has a lot of required fields we don't care about for these
// helpers — they only read `accounts`. Cast a partial fixture as the full
// type rather than building a full response per test.
const asAuth = (partial: Pick<AuthResponse, "accounts">) => partial as AuthResponse;

function req(cookie?: string) {
  return new Request("http://localhost/test", {
    headers: cookie ? { cookie } : undefined,
  });
}

describe("getActiveAccount", () => {
  const accounts = [
    { id: "p", name: "personal-user", type: "personal" as const },
    { id: "a", name: "acme", type: "organization" as const },
    { id: "b", name: "beta", type: "organization" as const },
  ];

  beforeEach(() => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockResolvedValue(asAuth({ accounts }));
  });

  afterEach(() => vi.restoreAllMocks());

  it("returns the cookie-named account when it matches a member account", async () => {
    const ctx = await getActiveAccount(req("astro:active-account=acme"));
    expect(ctx?.accountName).toBe("acme");
  });

  it("falls back to personal when the cookie names an account the user no longer belongs to", async () => {
    const ctx = await getActiveAccount(req("astro:active-account=ghost-org"));
    expect(ctx?.accountName).toBe("personal-user");
  });

  it("falls back to personal when no cookie is set", async () => {
    const ctx = await getActiveAccount(req());
    expect(ctx?.accountName).toBe("personal-user");
  });

  it("falls back to the first account when the user has no personal account", async () => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockResolvedValue(
      asAuth({
        accounts: [
          { id: "a", name: "acme", type: "organization" as const },
          { id: "b", name: "beta", type: "organization" as const },
        ],
      }),
    );
    const ctx = await getActiveAccount(req());
    expect(ctx?.accountName).toBe("acme");
  });

  it("returns null when getCurrentUser throws", async () => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockRejectedValue(new Error("nope"));
    expect(await getActiveAccount(req())).toBeNull();
  });

  it("returns null when the user has no accounts at all", async () => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockResolvedValue(asAuth({ accounts: [] }));
    expect(await getActiveAccount(req())).toBeNull();
  });
});
