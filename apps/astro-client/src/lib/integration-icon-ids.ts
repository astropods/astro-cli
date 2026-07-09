import manifest from "@astropods/brand-icons/icons.json";

/**
 * Ids of the first-party brand icons, read from the brand-icons package
 * manifest — the same list its build processes into
 * `assets/integrations/{light,dark}`. Gates rendering to shipped icons so we
 * never request a missing asset (no 404 / broken-image flash).
 */
export const INTEGRATION_ICON_IDS: ReadonlySet<string> = new Set(
  manifest.icons.map((icon) => icon.id),
);

/** Whether a first-party brand icon is shipped for `id` (both light and dark). */
export function hasIntegrationIcon(id: string): boolean {
  return INTEGRATION_ICON_IDS.has(id.toLowerCase());
}
