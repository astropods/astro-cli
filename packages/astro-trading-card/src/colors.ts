/**
 * Card-specific color derivation.
 *
 * Uses target-based swatch selection (inspired by Android's Palette API)
 * to pick the best swatch for each role from the MMCQ palette.
 */

import type { CardColors } from "./types";
import type { RGB, Swatch } from "./mmcq";
import { rgbToHsl, hslToHex } from "./mmcq";

// ─── Target-based swatch selection ──────────────────────────────────────────

interface SwatchTarget {
  targetSaturation: number;
  minSaturation: number;
  maxSaturation: number;
  targetLightness: number;
  minLightness: number;
  maxLightness: number;
}

const VIBRANT: SwatchTarget = {
  targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
  targetLightness: 0.5,  minLightness: 0.3,   maxLightness: 0.7,
};

const LIGHT_VIBRANT: SwatchTarget = {
  targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
  targetLightness: 0.74, minLightness: 0.55,  maxLightness: 0.9,
};

const DARK_VIBRANT: SwatchTarget = {
  targetSaturation: 1.0, minSaturation: 0.35, maxSaturation: 1.0,
  targetLightness: 0.26, minLightness: 0.1,   maxLightness: 0.45,
};

const MUTED: SwatchTarget = {
  targetSaturation: 0.3, minSaturation: 0.0,  maxSaturation: 0.4,
  targetLightness: 0.5,  minLightness: 0.3,   maxLightness: 0.7,
};

// Scoring weights (matching Android Palette defaults)
const WEIGHT_SATURATION = 0.24;
const WEIGHT_LIGHTNESS = 0.52;
const WEIGHT_POPULATION = 0.24;

/** Score how well a swatch matches a target profile. Returns 0-1. */
function scoreForTarget(
  swatch: Swatch,
  target: SwatchTarget,
  maxPopulation: number,
): number {
  const [, s, l] = rgbToHsl(swatch.r, swatch.g, swatch.b);

  // Reject swatches outside the target bounds
  if (s < target.minSaturation || s > target.maxSaturation) return 0;
  if (l < target.minLightness || l > target.maxLightness) return 0;

  const satScore = 1 - Math.abs(s - target.targetSaturation);
  const lumScore = 1 - Math.abs(l - target.targetLightness);
  const popScore = maxPopulation > 0 ? swatch.population / maxPopulation : 0;

  return satScore * WEIGHT_SATURATION
       + lumScore * WEIGHT_LIGHTNESS
       + popScore * WEIGHT_POPULATION;
}

/** Pick the best swatch for a given target, excluding already-used swatches. */
function pickForTarget(
  swatches: Swatch[],
  target: SwatchTarget,
  maxPopulation: number,
  used: Set<number>,
): { swatch: Swatch; index: number } | null {
  let best: { swatch: Swatch; index: number; score: number } | null = null;
  for (let i = 0; i < swatches.length; i++) {
    if (used.has(i)) continue;
    const score = scoreForTarget(swatches[i], target, maxPopulation);
    if (score > 0 && (!best || score > best.score)) {
      best = { swatch: swatches[i], index: i, score };
    }
  }
  return best;
}

// ─── Public API ─────────────────────────────────────────────────────────────

/**
 * Pick card colors from a palette of swatches using target-based selection.
 *
 * Each role (vibrant, light vibrant, dark vibrant, muted) independently picks
 * the best-matching swatch. The "accent" is the vibrant swatch (or the most
 * populated swatch as fallback).
 */
export function pickCardColors(swatches: Swatch[]): CardColors | null {
  if (swatches.length === 0) return null;

  const maxPop = Math.max(...swatches.map((s) => s.population));
  const used = new Set<number>();

  // Pick vibrant first — this becomes our "accent"
  const vibrant = pickForTarget(swatches, VIBRANT, maxPop, used);
  if (vibrant) used.add(vibrant.index);

  const lightVibrant = pickForTarget(swatches, LIGHT_VIBRANT, maxPop, used);
  if (lightVibrant) used.add(lightVibrant.index);

  const darkVibrant = pickForTarget(swatches, DARK_VIBRANT, maxPop, used);
  if (darkVibrant) used.add(darkVibrant.index);

  const muted = pickForTarget(swatches, MUTED, maxPop, used);

  // Accent is vibrant, falling back to the most populated swatch
  const accent = vibrant?.swatch ?? swatches[0];
  const [accentH, accentS] = rgbToHsl(accent.r, accent.g, accent.b);

  // For accentLight, prefer light vibrant, then fall back to a derived color
  const accentLightSwatch = lightVibrant?.swatch;
  let accentLightHex: string;
  if (accentLightSwatch) {
    const [slH, slS] = rgbToHsl(accentLightSwatch.r, accentLightSwatch.g, accentLightSwatch.b);
    accentLightHex = hslToHex(slH, Math.min(slS, 0.6), 0.75);
  } else {
    accentLightHex = hslToHex(accentH, Math.min(accentS, 0.4), 0.75);
  }

  return {
    accent: accent.hex,
    accentLight: accentLightHex,
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
