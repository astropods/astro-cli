import type { Account, AuthResponse } from "./api";

// Shared constants + helpers for the active-account cookie. Lives in a
// non-server module so both client hooks (use-active-account.tsx) and
// server loaders (api.server.ts, root.tsx) can import the same code
// without duplication. Renaming or changing parsing here updates every
// consumer at once; otherwise drift here is silent and ends with the
// server quietly falling back to the personal account.

export const ACTIVE_ACCOUNT_COOKIE = "astro:active-account";

/**
 * Resolves which account the caller is actually working in, from the cookie and
 * the session's own organization claim.
 *
 * The two can disagree: re-scoping the session is a separate act from moving the
 * cookie, and three callers (the deploy vault, blueprint creation, org settings)
 * re-scope without touching it. A year-long cookie also outlives the session it
 * was written for, and a fresh login lands on the personal organization. The
 * session claim wins, because the server refuses reads on any account it is not
 * scoped to, so a cookie the session cannot back is not a scope at all.
 */
export function resolveActiveAccount(
  auth: Pick<AuthResponse, "accounts" | "organization_id">,
  cookieAccount: string | null,
): Account | undefined {
  const accounts = auth.accounts ?? [];
  if (accounts.length === 0) return undefined;

  const sessionOrg = auth.organization_id || "";
  const requested = cookieAccount
    ? accounts.find((account) => account.name === cookieAccount)
    : undefined;
  // An account with no organization of its own cannot be shown to disagree, and
  // an unscoped session claims nothing, so neither overrides the cookie.
  if (requested && (!sessionOrg || !requested.organization_id || requested.organization_id === sessionOrg)) {
    return requested;
  }

  return (
    (sessionOrg ? accounts.find((account) => account.organization_id === sessionOrg) : undefined) ??
    accounts.find((account) => account.type === "personal") ??
    accounts[0]
  );
}

/**
 * Parse a single cookie value from a raw `Cookie` header (server) or the
 * `document.cookie` string (client). Returns the decoded value or null if
 * the cookie isn't present.
 */
export function readCookieValue(cookieHeader: string | null | undefined, name: string): string | null {
  if (!cookieHeader) return null;
  for (const part of cookieHeader.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return decodeURIComponent(v.join("="));
  }
  return null;
}
