import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { getActiveAccount, loadAccountScoped } from "./api.server";
import { ApiClient, type AuthResponse } from "./api";

// AuthResponse has a lot of required fields we don't care about for these
// helpers — they only read `accounts`. Cast a partial fixture as the full
// type rather than building a full response per test.
const asAuth = (partial: Pick<AuthResponse, "accounts">) => partial as AuthResponse;

// readCookieValue's own unit tests live in `active-account.test.ts` next to
// where the helper is defined. This file covers `getActiveAccount` (cookie
// → account resolution) and `loadAccountScoped` (the loader wrapper).

// Build a minimal Request the loader helpers can read cookies off. Node's
// native Request type works in vitest's jsdom env without polyfills.
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

describe("loadAccountScoped", () => {
  const accounts = [{ id: "p", name: "personal-user", type: "personal" as const }];

  beforeEach(() => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockResolvedValue(asAuth({ accounts }));
  });

  afterEach(() => vi.restoreAllMocks());

  it("calls the fetcher with the resolved api + account name and returns its data", async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: ["a", "b"] });
    const result = await loadAccountScoped(req(), fetcher);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0]?.[1]).toBe("personal-user");
    expect(result).toEqual({ account: "personal-user", data: { items: ["a", "b"] } });
  });

  it("returns { account: null, data: null } when the active account cannot be resolved", async () => {
    vi.spyOn(ApiClient.prototype, "getCurrentUser").mockRejectedValue(new Error("auth"));
    const fetcher = vi.fn();
    const result = await loadAccountScoped(req(), fetcher);
    expect(fetcher).not.toHaveBeenCalled();
    expect(result).toEqual({ account: null, data: null });
  });

  it("returns the account but `data: null` when the fetcher throws", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("boom"));
    const result = await loadAccountScoped(req(), fetcher);
    expect(result).toEqual({ account: "personal-user", data: null });
  });
});
