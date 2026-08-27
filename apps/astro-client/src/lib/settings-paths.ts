import type { Account } from "@/lib/api";

/**
 * Path to a Settings subsection scoped to a specific account.
 *
 * Personal accounts use the top-level `/settings/<section>`; organization
 * accounts use the org-scoped `/settings/org/<slug>/<section>`, where the slug
 * is the account `name` — matching the `:orgSlug` route param, which
 * `OrgSettingsLayout` resolves against `Account.name`.
 *
 * When the account is unknown (not in `accounts`), the personal path is used as
 * a safe default. Mirrors how the deploy form and blueprint flows build their
 * scoped Settings links.
 */
export function accountSettingsPath(
  accounts: Account[],
  accountName: string,
  section: string,
): string {
  const account = accounts.find(a => a.name === accountName);
  return !account || account.type === "personal"
    ? `/settings/${section}`
    : `/settings/org/${account.name}/${section}`;
}

const PERSONAL_SECTIONS = new Set([
  "account",
  "billing",
  "usage",
  "notifications",
  "organizations",
  "audit-log",
  "secrets",
  "connectors",
  "api-keys",
  "apps",
  "experiments",
]);

const ORG_SECTIONS = new Set([
  "general",
  "billing",
  "usage",
  "members",
  "audit-log",
  "secrets",
  "api-keys",
  "apps",
  "experiments",
]);

/** Section a Settings URL is currently on, for either scope. */
export function settingsSectionFromPath(pathname: string): string {
  const parts = pathname.split("/").filter(Boolean);
  if (parts[0] !== "settings") return "account";
  return (parts[1] === "org" ? parts[3] : parts[1]) ?? "account";
}

/**
 * Path to the same Settings section in another account's scope, for the scope
 * selector. Sections that exist in both scopes are kept; the personal Account
 * page and the org General page stand in for each other; anything with no
 * counterpart (Connectors, Members) falls back to the scope's landing section.
 */
export function settingsScopePath(
  accounts: Account[],
  accountName: string,
  section: string,
): string {
  const account = accounts.find(a => a.name === accountName);

  if (!account || account.type === "personal") {
    const mapped = section === "general" ? "account" : section;
    return `/settings/${PERSONAL_SECTIONS.has(mapped) ? mapped : "account"}`;
  }

  const mapped = section === "account" ? "general" : section;
  return `/settings/org/${account.name}/${ORG_SECTIONS.has(mapped) ? mapped : "general"}`;
}
