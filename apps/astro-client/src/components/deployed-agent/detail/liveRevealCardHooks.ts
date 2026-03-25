import { useEffect, useMemo, useState } from "react";
import { resolveCardIntegrations } from "@/lib/integrationIcons";
import type { ResolvedIntegration } from "@/lib/api";
import type { CardColors, CardData } from "astro-trading-card";
import { DEFAULT_COLORS } from "astro-trading-card";
import { extractColorsFromImage, svgToImageSource } from "astro-trading-card/browser";

export function useExtractedColors(avatar: CardData["avatar"], enabled: boolean) {
  const [colors, setColors] = useState<CardColors>(DEFAULT_COLORS);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;

    let source: string | null = null;
    if (avatar?.url) {
      source = avatar.url;
    } else if (avatar?.svg) {
      source = svgToImageSource(avatar.svg);
    }

    if (!source) {
      setColors(DEFAULT_COLORS);
      return;
    }

    extractColorsFromImage(source).then((result) => {
      if (!cancelled) setColors(result ?? DEFAULT_COLORS);
    });

    return () => {
      cancelled = true;
    };
  }, [avatar, enabled]);

  return colors;
}

export function useResolvedIntegrations(integrations: ResolvedIntegration[] | undefined, enabled: boolean) {
  return useMemo(
    () => (enabled && integrations?.length ? resolveCardIntegrations(integrations) : []),
    [enabled, integrations],
  );
}
