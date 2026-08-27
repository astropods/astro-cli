// Shared constants + helpers for the active-account cookie. Lives in a
// non-server module so both client hooks (use-active-account.tsx) and
// server loaders (api.server.ts, root.tsx) can import the same code
// without duplication. Renaming or changing parsing here updates every
// consumer at once; otherwise drift here is silent and ends with the
// server quietly falling back to the personal account.

export const ACTIVE_ACCOUNT_COOKIE = "astro:active-account";

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
