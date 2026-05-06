/** Strip http:// or https:// prefix from a URL for display in the prefixed input. */
export function stripProtocol(url: string): string {
  return url.replace(/^https?:\/\//, "");
}

/** Normalise a raw input value to a full https:// URL, or "" if empty. */
export function withProtocol(raw: string): string {
  const stripped = stripProtocol(raw).trim();
  return stripped ? `https://${stripped}` : "";
}
