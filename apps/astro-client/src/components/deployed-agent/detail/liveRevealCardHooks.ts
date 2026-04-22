import { useMemo } from "react";
import { resolveCardIntegrations } from "@/lib/integrationIcons";
import type { AvatarColors, ResolvedIntegration } from "@/lib/api";
import type { CardColors } from "astro-trading-card";
import { DEFAULT_COLORS } from "astro-trading-card";

/** Map server-provided AvatarColors to the CardColors shape used by the trading card. */
export function useCardColors(serverColors?: AvatarColors): CardColors {
  return useMemo(() => {
    if (!serverColors) return DEFAULT_COLORS;
    return {
      accent: serverColors.accent,
      accentLight: serverColors.accent_light,
      background: serverColors.background,
      foreground: serverColors.foreground,
      glow: serverColors.glow,
    };
  }, [serverColors]);
}

export function useResolvedIntegrations(integrations: ResolvedIntegration[] | undefined, enabled: boolean) {
  return useMemo(
    () => (enabled && integrations?.length ? resolveCardIntegrations(integrations) : []),
    [enabled, integrations],
  );
}
