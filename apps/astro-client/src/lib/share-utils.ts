export function getLinkedInShareHref(url: string, name: string) {
  const text = `Check out ${name} on Astro AI:\n\n${url}`;
  return `https://www.linkedin.com/shareArticle?mini=true&url=${encodeURIComponent(url)}&title=${encodeURIComponent(text)}&summary=${encodeURIComponent(text)}`;
}

export function getXShareHref(url: string, name: string) {
  return `https://x.com/intent/tweet?text=${encodeURIComponent(`Check out ${name} on Astro AI:\n\n${url}`)}`;
}
