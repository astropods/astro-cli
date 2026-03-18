import type { CardColors, CardData, CardOptions } from "./types";
import { CARD_DIMENSIONS } from "./types";
import { renderStandard } from "./variants/standard";

const DEFAULT_COLORS: CardColors = {
  background: "#1a1a2e",
  foreground: "#e8e8ec",
  accent: "#6366f1",
  accentLight: "#a5b4fc",
};

/**
 * Generate a trading card SVG string for an agent.
 *
 * @param data - Agent data to display on the card.
 * @param options - Card variant and styling options.
 * @returns SVG string.
 */
export function generateCard(data: CardData, options?: CardOptions): string {
  const colors: CardColors = {
    ...DEFAULT_COLORS,
    ...data.colors,
  };
  return renderStandard(data, colors);
}

/**
 * Get the pixel dimensions for the standard card.
 */
export function getCardDimensions() {
  return CARD_DIMENSIONS.standard;
}
