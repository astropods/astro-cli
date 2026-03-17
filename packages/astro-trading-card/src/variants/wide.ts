import { CARD_DIMENSIONS, type CardColors, type CardData } from "../types";

export function renderWide(data: CardData, colors: CardColors): string {
  const { width, height } = CARD_DIMENSIONS.wide;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <rect width="${width}" height="${height}" fill="${colors.background}"/>
</svg>`;
}
