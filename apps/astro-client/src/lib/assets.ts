/**
 * Build a URL for a static asset hosted on the CDN (prod/preview)
 * or served locally by Vite from the repo-root assets/ folder.
 *
 * @param path - Asset path relative to the assets root, e.g. "integrations/light/github.svg"
 */
export function getAssetUrl(path: string): string {
  const base = import.meta.env.VITE_ASSETS_URL;
  return base ? `${base}/${path}` : `/assets/${path}`;
}

/**
 * Get the URL for an integration icon by its canonical ID.
 *
 * @param id - Canonical integration ID, e.g. "github", "slack"
 * @param variant - "light" or "dark" (default: "light")
 */
export function getIntegrationIconUrl(id: string, variant: "light" | "dark" = "light"): string {
  return getAssetUrl(`integrations/${variant}/${id}.svg`);
}
