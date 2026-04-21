import { hash, createRng } from "./rng";
import {
  buildPolygonPath,
  edgeStyles,
  type EdgeStyle,
  type PolygonParams,
} from "./polygon";
import { paletteNames, palettes, type PaletteName } from "./theme";
import { buildEyes, eyeStyles, type EyeStyle, type EyeParams } from "./eyes";

export interface IdentityOptions {
  /** Seed string used to deterministically generate the identity. */
  seed: string;
  /** Width/height of the SVG in pixels (default: 128). */
  size?: number;
}

export interface CustomIdentityOptions {
  size?: number;
  bgPalette: PaletteName;
  bgShade: (typeof shadeKeys)[number];
  fgPalette: PaletteName;
  fgShade: (typeof shadeKeys)[number];
  eyePalette: PaletteName;
  eyeShade: (typeof shadeKeys)[number];
  sides: number;
  edgeStyle: EdgeStyle;
  rotation: number;
  radius: number;
  spikeDepth: number;
  curveAmount: number;
  leftEyeStyle: EyeStyle;
  rightEyeStyle: EyeStyle;
  eyeSpacing: number;
  eyeSize: number;
}

/** Structured record of every choice `generateIdentity` makes for a given seed.
 *  Used by the Go port's parity tests to verify identical decisions. */
export interface IdentityChoices {
  size: number;
  bgPalette: PaletteName;
  bgShade: (typeof shadeKeys)[number];
  fgPalette: PaletteName;
  fgShade: (typeof shadeKeys)[number];
  eyePalette: PaletteName;
  eyeShade: (typeof shadeKeys)[number];
  sides: number;
  edgeStyle: EdgeStyle;
  rotation: number;
  radius: number;
  spikeDepth: number;
  curveAmount: number;
  leftEyeStyle: EyeStyle;
  rightEyeStyle: EyeStyle;
  eyeSpacing: number;
  eyeSize: number;
}

/** Pick a value from an array using the next rng output. */
function pick<T>(arr: readonly T[], rng: () => number): T {
  return arr[Math.floor(rng() * arr.length)];
}

/** Pick a value from an array that doesn't match any of the excluded values. */
function pickExcluding<T>(arr: readonly T[], rng: () => number, exclude: T[]): T {
  const filtered = arr.filter((v) => !exclude.includes(v));
  const pool = filtered.length > 0 ? filtered : arr;
  return pool[Math.floor(rng() * pool.length)];
}

/** Shade index in the shadeKeys array. */
function shadeIndex(shade: typeof shadeKeys[number]): number {
  return shadeKeys.indexOf(shade);
}

/** Get shades that are at least `minDistance` steps away from all used shades. */
function shadesWithContrast(
  used: (typeof shadeKeys[number])[],
  minDistance: number,
): (typeof shadeKeys[number])[] {
  return shadeKeys.filter((s) => {
    const si = shadeIndex(s);
    return used.every((u) => Math.abs(si - shadeIndex(u)) >= minDistance);
  });
}

/** Map rng output to a range [min, max]. */
function range(rng: () => number, min: number, max: number): number {
  return min + rng() * (max - min);
}

/** Shade keys ordered light → dark for easy indexing. */
const shadeKeys = [
  50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950,
] as const;

/**
 * Generates a deterministic SVG identity from a seed string, returning both the
 * SVG and the structured choices the RNG produced. Shared implementation used
 * by `generateIdentity` and parity-fixture tooling.
 */
export function generateIdentityWithChoices(
  options: IdentityOptions,
): { svg: string; choices: IdentityChoices } {
  const { seed, size = 128 } = options;
  const rng = createRng(hash(seed));

  // Pick palettes and shades, ensuring sufficient lightness contrast between layers.
  // minDistance of 3 means at least 3 shade steps apart (~e.g. 200 vs 500).
  const MIN_SHADE_DISTANCE = 3;

  const bgPaletteName = pick(paletteNames, rng);
  const bgShade = pick(shadeKeys, rng);
  const bgColor = palettes[bgPaletteName][bgShade];

  const fgPaletteName = pick(paletteNames, rng);
  const fgPool = shadesWithContrast([bgShade], MIN_SHADE_DISTANCE);
  const fgShade = fgPool.length > 0 ? pick(fgPool, rng) : pickExcluding(shadeKeys, rng, [bgShade]);
  const fgColor = palettes[fgPaletteName][fgShade];

  // Derive polygon parameters
  const sides = Math.floor(range(rng, 3, 9)); // 3–8
  const edgeStyle: EdgeStyle = pick(edgeStyles, rng);
  const rotation = range(rng, 0, 2 * Math.PI);
  const radius = range(rng, 0.55, 0.9);
  const spikeDepth = range(rng, 0.3, 0.7);
  const curveAmount = range(rng, 0.2, 0.5);

  const params: PolygonParams = {
    sides,
    edgeStyle,
    rotation,
    radius,
    spikeDepth,
    curveAmount,
  };

  const path = buildPolygonPath(params, size);

  // Eyes — ensure sufficient lightness contrast against both bg and polygon
  const eyePaletteName = pick(paletteNames, rng);
  const eyePool = shadesWithContrast([bgShade, fgShade], MIN_SHADE_DISTANCE);
  const eyeShade = eyePool.length > 0 ? pick(eyePool, rng) : pickExcluding(shadeKeys, rng, [bgShade, fgShade]);
  const eyeColor = palettes[eyePaletteName][eyeShade];
  const leftEyeStyle = pick(eyeStyles, rng);
  // ~10% chance of mismatched eyes
  const rightEyeStyle = rng() < 0.1 ? pickExcluding(eyeStyles, rng, [leftEyeStyle]) : leftEyeStyle;
  const eyeSpacing = range(rng, 0.15, 0.35);
  const eyeSize = range(rng, 0.04, 0.1);
  const eyeParams: EyeParams = {
    leftStyle: leftEyeStyle,
    rightStyle: rightEyeStyle,
    spacing: eyeSpacing,
    eyeSize,
  };
  const eyes = buildEyes(eyeParams, size, eyeColor);

  const svg = [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">`,
    `  <rect width="${size}" height="${size}" fill="${bgColor}" />`,
    `  <path d="${path}" fill="${fgColor}" />`,
    `  ${eyes}`,
    `</svg>`,
  ].join("\n");

  const choices: IdentityChoices = {
    size,
    bgPalette: bgPaletteName,
    bgShade,
    fgPalette: fgPaletteName,
    fgShade,
    eyePalette: eyePaletteName,
    eyeShade,
    sides,
    edgeStyle,
    rotation,
    radius,
    spikeDepth,
    curveAmount,
    leftEyeStyle,
    rightEyeStyle,
    eyeSpacing,
    eyeSize,
  };

  return { svg, choices };
}

/**
 * Generates a deterministic SVG identity from a seed string.
 */
export function generateIdentity(options: IdentityOptions): string {
  return generateIdentityWithChoices(options).svg;
}

/**
 * Generates an SVG identity from explicit trait values (no seed / RNG).
 */
export function generateCustomIdentity(opts: CustomIdentityOptions): string {
  const size = opts.size ?? 128;

  const bgColor = palettes[opts.bgPalette][opts.bgShade];
  const fgColor = palettes[opts.fgPalette][opts.fgShade];
  const eyeColor = palettes[opts.eyePalette][opts.eyeShade];

  const params: PolygonParams = {
    sides: opts.sides,
    edgeStyle: opts.edgeStyle,
    rotation: opts.rotation,
    radius: opts.radius,
    spikeDepth: opts.spikeDepth,
    curveAmount: opts.curveAmount,
  };

  const path = buildPolygonPath(params, size);

  const eyeParams: EyeParams = {
    leftStyle: opts.leftEyeStyle,
    rightStyle: opts.rightEyeStyle,
    spacing: opts.eyeSpacing,
    eyeSize: opts.eyeSize,
  };
  const eyes = buildEyes(eyeParams, size, eyeColor);

  return [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">`,
    `  <rect width="${size}" height="${size}" fill="${bgColor}" />`,
    `  <path d="${path}" fill="${fgColor}" />`,
    `  ${eyes}`,
    `</svg>`,
  ].join("\n");
}
