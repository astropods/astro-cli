import type { CardColors, CardData, CardOptions, CardVariant } from "./types";
import { CARD_DIMENSIONS } from "./types";
import { renderStandard } from "./variants/standard";
import { renderWide } from "./variants/wide";
import { renderCompact } from "./variants/compact";

const DEFAULT_COLORS: CardColors = {
  background: "#1a1a2e",
  foreground: "#e8e8ec",
  accent: "#6366f1",
  accentLight: "#a5b4fc",
};

const renderers: Record<CardVariant, (data: CardData, colors: CardColors) => string> = {
  standard: renderStandard,
  wide: renderWide,
  compact: renderCompact,
};

/**
 * Generate a trading card SVG string for an agent.
 *
 * @param data - Agent data to display on the card.
 * @param options - Card variant and styling options.
 * @returns SVG string.
 */
export function generateCard(data: CardData, options?: CardOptions): string {
  const variant = options?.variant ?? "standard";
  const colors: CardColors = {
    ...DEFAULT_COLORS,
    ...data.colors,
  };
  const render = renderers[variant];
  return render(data, colors);
}

/**
 * Get the pixel dimensions for a card variant.
 */
export function getCardDimensions(variant: CardVariant = "standard") {
  return CARD_DIMENSIONS[variant];
}
