import { useEffect, useState } from "react";
import type { CardAvatar } from "astro-trading-card";
import { stripSvgWrapper } from "astro-trading-card";
import { generateIdentity } from "identity-gen";

/**
 * Resolves a `CardAvatar` for the trading card component.
 * Probes the image URL once. Returns `null` while loading, then a stable
 * `CardAvatar` (either `{ url }` or `{ svg }`) that never changes again
 * for the same URL.
 */
export function useCardAvatar(url: string, account: string, name: string): CardAvatar | null {
  const [result, setResult] = useState<CardAvatar | null>(null);

  useEffect(() => {
    let cancelled = false;
    const img = new Image();
    img.onload = () => {
      if (!cancelled) setResult({ url });
    };
    img.onerror = () => {
      if (!cancelled) {
        const svg = generateIdentity({ seed: `${account}/${name}`, size: 128 });
        setResult({ svg: stripSvgWrapper(svg) });
      }
    };
    img.src = url;
    return () => { cancelled = true; };
  }, [url, account, name]);

  return result;
}
