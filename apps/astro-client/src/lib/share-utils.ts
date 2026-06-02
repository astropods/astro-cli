export function getLinkedInShareHref(url: string, name: string) {
  const text = `Check out ${name} on Astro AI:\n\n${url}`;
  return `https://www.linkedin.com/shareArticle?mini=true&url=${encodeURIComponent(url)}&title=${encodeURIComponent(text)}&summary=${encodeURIComponent(text)}`;
}

export function getXShareHref(url: string, name: string) {
  return `https://x.com/intent/tweet?text=${encodeURIComponent(`Check out ${name} on Astro AI:\n\n${url}`)}`;
}

/** Opens an X or LinkedIn share intent for the "just launched a badge" flow.
 *  Distinct from the blueprint Share helpers above: copy is post-deploy framed
 *  ("Just launched X") and uses the feed/post intent endpoints. */
export function openBadgeShareIntent(
  network: "x" | "linkedin",
  args: { launchName: string; blueprintUrl: string },
) {
  const shareText = `Just launched ${args.launchName} on Astro AI!\n\nCheck out the blueprint:\n\n${args.blueprintUrl}`;
  const url = network === "x"
    ? `https://x.com/intent/post?text=${encodeURIComponent(shareText)}`
    : `https://www.linkedin.com/feed/?shareActive=true&mini=true&text=${encodeURIComponent(shareText)}`;
  window.open(url, "_blank", "noopener,noreferrer");
}
