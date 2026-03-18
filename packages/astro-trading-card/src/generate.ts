import type { CardColors, CardData, ResolvedCardColors } from "./types";
import { CARD_DIMENSIONS } from "./types";
import { renderStandard } from "./variants/standard";

/** Default card colors based on the Astro teal theme. */
export const DEFAULT_COLORS: ResolvedCardColors = {
  background: "#0a1614",
  foreground: "#f0f5f4",
  accent: "#14b8a6",
  accentLight: "#5eead4",
  glow: "#99f6e4",
};

/**
 * Generate a trading card SVG string for an agent.
 *
 * @param data - Agent data to display on the card.
 * @returns SVG string.
 */
export function generateCard(data: CardData): string {
  const merged: CardColors = { ...DEFAULT_COLORS, ...data.colors };
  const colors: ResolvedCardColors = {
    ...merged,
    accentLight: merged.accentLight ?? merged.accent,
    glow: merged.glow ?? merged.foreground,
  };
  return renderStandard(data, colors);
}

/**
 * Get the pixel dimensions for the standard card.
 */
export function getCardDimensions() {
  return CARD_DIMENSIONS.standard;
}
