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
 * Get the CDN URL for an account's avatar image.
 * In production, points directly to the CDN. In local dev, falls back to a static placeholder.
 *
 * @param handle - Account handle, e.g. "janesmith"
 * @param version - Avatar version for cache-busting (from account API response)
 */
export function getAvatarUrl(handle: string, version?: number): string {
  const base = import.meta.env.VITE_ASSETS_URL;
  const url = base
    ? `${base}/avatars/${encodeURIComponent(handle)}.jpg`
    : `/assets/avatars/${encodeURIComponent(handle)}.jpg`;
  return version ? `${url}?v=${version}` : url;
}

/**
 * URL for the shared default avatar placeholder.
 * Used as the ultimate fallback if the CDN image fails to load.
 */
export function getFallbackAvatarUrl(): string {
  return getAssetUrl("placeholders/accounts/avatar_01.svg");
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
