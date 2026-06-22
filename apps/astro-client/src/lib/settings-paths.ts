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
