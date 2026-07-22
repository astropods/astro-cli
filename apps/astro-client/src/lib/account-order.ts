/**
 * The order accounts are listed in wherever we show an account list (the org
 * switcher and the agent switcher): the personal account first, then the rest
 * alphabetically by name. This is the single source of truth for that order, so
 * both switchers stay in sync when it changes.
 */
export function comparePersonalFirst(
  a: { type: string; name: string },
  b: { type: string; name: string },
): number {
  if (a.type !== b.type) {
    if (a.type === "personal") return -1;
    if (b.type === "personal") return 1;
  }
  return a.name.localeCompare(b.name);
}
