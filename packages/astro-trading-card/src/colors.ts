/**
 * Card-specific color derivation.
 *
 * Takes palette swatches (from mmcq.ts) and derives a full CardColors scheme.
 */

import type { CardColors } from "./types";
import type { RGB, Swatch } from "./mmcq";
import { rgbToHsl, hslToHex } from "./mmcq";

/** Simple saturation calculation from RGB (HSV saturation). */
function saturation(r: number, g: number, b: number): number {
  const max = Math.max(r, g, b) / 255;
  const min = Math.min(r, g, b) / 255;
  if (max === 0) return 0;
  return (max - min) / max;
}

/**
 * Pick card colors from a palette of swatches.
 *
 * - accent: the most vibrant dominant color
 * - accentLight: a lighter variant of a secondary color
 * - background: accent hue darkened to ~12% lightness (still has color)
 * - foreground: accent hue lightened to ~92% lightness
 */
export function pickCardColors(swatches: Swatch[]): CardColors | null {
  if (swatches.length === 0) return null;

  // Score by: saturation * sqrt(population share) — balances vibrancy with dominance
  const totalPop = swatches.reduce((sum, s) => sum + s.population, 0);
  const scored = swatches.map((s) => ({
    ...s,
    score: saturation(s.r, s.g, s.b) * Math.sqrt(s.population / totalPop),
  }));
  scored.sort((a, b) => b.score - a.score);

  const accent = scored[0];
  const [accentH, accentS] = rgbToHsl(accent.r, accent.g, accent.b);

  // Find a secondary swatch with sufficient color distance
  let secondary = scored[1] ?? accent;
  for (const s of scored.slice(1)) {
    const dr = s.r - accent.r;
    const dg = s.g - accent.g;
    const db = s.b - accent.b;
    if (Math.sqrt(dr * dr + dg * dg + db * db) > 80) {
      secondary = s;
      break;
    }
  }
  const [secH, secS] = rgbToHsl(secondary.r, secondary.g, secondary.b);

  return {
    accent: accent.hex,
    accentLight: hslToHex(secH, Math.min(secS, 0.6), 0.75),
    background: hslToHex(accentH, Math.min(accentS, 0.5), 0.09),
    foreground: hslToHex(accentH, Math.min(accentS, 0.1), 0.96),
    glow: hslToHex(accentH, Math.min(accentS, 0.9), 0.8),
  };
}

/** Parse a hex color string to RGB. Returns null if invalid. */
export function parseHex(hex: string): RGB | null {
  const m = hex.match(/^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i);
  if (!m) return null;
  return { r: parseInt(m[1], 16), g: parseInt(m[2], 16), b: parseInt(m[3], 16) };
}

/**
 * Derive a full CardColors from one or two hex accent colors.
 * Use this when you have known colors (e.g. extracted from SVG fills)
 * rather than raw pixel data.
 */
export function deriveCardColors(accent: string, secondaryAccent?: string): CardColors {
  const rgb = parseHex(accent) ?? { r: 20, g: 184, b: 166 };
  const [h, s] = rgbToHsl(rgb.r, rgb.g, rgb.b);

  let accentLightHex: string;
  if (secondaryAccent) {
    const secRgb = parseHex(secondaryAccent);
    if (secRgb) {
      const [secH, secS] = rgbToHsl(secRgb.r, secRgb.g, secRgb.b);
      accentLightHex = hslToHex(secH, Math.min(secS, 0.6), 0.75);
    } else {
      accentLightHex = hslToHex(h, Math.min(s, 0.4), 0.75);
    }
  } else {
    accentLightHex = hslToHex(h, Math.min(s, 0.4), 0.75);
  }

  return {
    accent,
    accentLight: accentLightHex,
    background: hslToHex(h, Math.min(s, 0.5), 0.09),
    foreground: hslToHex(h, Math.min(s, 0.1), 0.96),
    glow: hslToHex(h, Math.min(s, 0.9), 0.8),
  };
}
