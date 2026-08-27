import { describe, it, expect } from "vitest";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue, resolveActiveAccount } from "./active-account";
import type { Account, AuthResponse } from "./api";

// readCookieValue is the single cookie parser shared between server loaders
// (api.server.ts, root.tsx) and the client hook (use-active-account.tsx).
// Edge cases that would matter in production live here.
describe("readCookieValue", () => {
  it("returns the value when the cookie is present alone", () => {
    expect(readCookieValue(`${ACTIVE_ACCOUNT_COOKIE}=acme`, ACTIVE_ACCOUNT_COOKIE)).toBe("acme");
  });

  it("returns the value when surrounded by other cookies", () => {
    const header = `session=abc; ${ACTIVE_ACCOUNT_COOKIE}=acme; theme=dark`;
    expect(readCookieValue(header, ACTIVE_ACCOUNT_COOKIE)).toBe("acme");
  });

  it("decodes URL-encoded values (spaces, etc.)", () => {
    expect(readCookieValue(`${ACTIVE_ACCOUNT_COOKIE}=my%20org`, ACTIVE_ACCOUNT_COOKIE)).toBe("my org");
  });

  it("preserves '=' characters that appear inside the value", () => {
    // Reassembles the value from the trailing fragments after the first '='.
    expect(readCookieValue(`${ACTIVE_ACCOUNT_COOKIE}=a=b=c`, ACTIVE_ACCOUNT_COOKIE)).toBe("a=b=c");
  });

  it("trims leading whitespace before the cookie name", () => {
    // Browsers serialize `Cookie: a=1; b=2` with a space after the semicolon.
    expect(readCookieValue(` ${ACTIVE_ACCOUNT_COOKIE}=acme`, ACTIVE_ACCOUNT_COOKIE)).toBe("acme");
  });

  it("returns null when the cookie is absent", () => {
    expect(readCookieValue("session=abc", ACTIVE_ACCOUNT_COOKIE)).toBeNull();
  });

  it("returns null for null/undefined/empty headers", () => {
    expect(readCookieValue(null, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
    expect(readCookieValue(undefined, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
    expect(readCookieValue("", ACTIVE_ACCOUNT_COOKIE)).toBeNull();
  });

  it("does not partial-match cookie names", () => {
    // A cookie named `${ACTIVE_ACCOUNT_COOKIE}-other` must NOT satisfy a
    // lookup for ACTIVE_ACCOUNT_COOKIE itself.
    expect(readCookieValue(`${ACTIVE_ACCOUNT_COOKIE}-other=foo`, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
  });

  it("propagates URIError on malformed URL encoding", () => {
    expect(() =>
      readCookieValue(`${ACTIVE_ACCOUNT_COOKIE}=bad%ZZ`, ACTIVE_ACCOUNT_COOKIE),
    ).toThrow(URIError);
  });
});

const account = (name: string, type: string, organizationID?: string): Account =>
  ({ id: name, name, type, organization_id: organizationID }) as Account;

const auth = (accounts: Account[], organizationID?: string) =>
  ({ accounts, organization_id: organizationID }) as AuthResponse;

const PERSONAL = account("testuser", "personal", "wos-personal");
const ACME = account("acme", "organization", "wos-acme");

describe("resolveActiveAccount", () => {
  it("keeps the cookie's account when the session is scoped to it", () => {
    expect(resolveActiveAccount(auth([PERSONAL, ACME], "wos-acme"), "acme")).toBe(ACME);
  });

  it("follows the session when the cookie names an organization it is not scoped to", () => {
    expect(resolveActiveAccount(auth([PERSONAL, ACME], "wos-personal"), "acme")).toBe(PERSONAL);
  });

  it("keeps the cookie's account when the session claims no organization", () => {
    expect(resolveActiveAccount(auth([PERSONAL, ACME]), "acme")).toBe(ACME);
  });

  it("keeps an account that has no organization of its own", () => {
    const unlinked = account("legacy-org", "organization");
    expect(resolveActiveAccount(auth([PERSONAL, unlinked], "wos-personal"), "legacy-org")).toBe(unlinked);
  });

  it("falls back to the personal account when the session's organization has no account", () => {
    expect(resolveActiveAccount(auth([PERSONAL, ACME], "wos-ghost"), "acme")).toBe(PERSONAL);
  });

  it("ignores a cookie naming an account the user left", () => {
    expect(resolveActiveAccount(auth([PERSONAL, ACME], "wos-acme"), "ghost-org")).toBe(ACME);
  });

  it("returns undefined when the user has no accounts", () => {
    expect(resolveActiveAccount(auth([]), "acme")).toBeUndefined();
  });
});
