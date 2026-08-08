import manifest from "@astropods/brand-icons/icons.json";

/**
 * An icon's `id` is our brand name, but callers look one up by whatever
 * identifier their data carries — a registrable domain, a driver name, a
 * provider string. `domains` and `aliases` on a manifest entry declare those
 * extra keys next to the icon they belong to.
 */
interface IconEntry {
  id: string;
  domains?: string[];
  aliases?: string[];
}

const ICONS = (manifest as { icons: IconEntry[] }).icons;

/**
 * Ids of the first-party brand icons, read from the brand-icons package
 * manifest — the same list its build processes into
 * `assets/integrations/{light,dark}`. Gates rendering to shipped icons so we
 * never request a missing asset (no 404 / broken-image flash).
 */
export const INTEGRATION_ICON_IDS: ReadonlySet<string> = new Set(ICONS.map((icon) => icon.id));

const ICON_ID_BY_KEY: ReadonlyMap<string, string> = new Map(
  ICONS.flatMap((icon) => [
    [icon.id, icon.id] as const,
    ...(icon.domains ?? []).map((d) => [d.toLowerCase(), icon.id] as const),
    ...(icon.aliases ?? []).map((a) => [a.toLowerCase(), icon.id] as const),
  ]),
);

/** Whether a first-party brand icon is shipped for `id` (both light and dark). */
export function hasIntegrationIcon(id: string): boolean {
  return INTEGRATION_ICON_IDS.has(id.toLowerCase());
}

/**
 * The icon id for any identifier a brand is known by, or null if none ships.
 * Map lookup, so an identifier colliding with an inherited object key can't
 * resolve to one.
 */
export function resolveIconId(key: string): string | null {
  return ICON_ID_BY_KEY.get(key.toLowerCase()) ?? null;
}
