/**
 * Modified Median Cut Quantization (MMCQ)
 *
 * Extracts a palette of dominant colors from raw RGBA pixel data.
 * Browser-agnostic — accepts a flat Uint8ClampedArray (e.g. from canvas getImageData).
 */

export interface RGB {
  r: number;
  g: number;
  b: number;
}

export interface Swatch extends RGB {
  /** Number of pixels in this cluster. */
  population: number;
  /** Hex string (e.g. "#a4c2f4"). */
  hex: string;
}

/** Convert RGB 0-255 to hex. */
function rgbToHex(r: number, g: number, b: number): string {
  return (
    "#" +
    ((1 << 24) | (r << 16) | (g << 8) | b).toString(16).slice(1)
  );
}

/**
 * A "color box" — an axis-aligned bounding box in RGB space
 * containing a subset of sampled pixels.
 */
interface ColorBox {
  pixels: RGB[];
  rMin: number;
  rMax: number;
  gMin: number;
  gMax: number;
  bMin: number;
  bMax: number;
}

function makeBox(pixels: RGB[]): ColorBox {
  let rMin = 255, rMax = 0;
  let gMin = 255, gMax = 0;
  let bMin = 255, bMax = 0;
  for (const p of pixels) {
    if (p.r < rMin) rMin = p.r;
    if (p.r > rMax) rMax = p.r;
    if (p.g < gMin) gMin = p.g;
    if (p.g > gMax) gMax = p.g;
    if (p.b < bMin) bMin = p.b;
    if (p.b > bMax) bMax = p.b;
  }
  return { pixels, rMin, rMax, gMin, gMax, bMin, bMax };
}

/** Split a box at the median along its longest axis. */
function splitBox(box: ColorBox): [ColorBox, ColorBox] {
  const rRange = box.rMax - box.rMin;
  const gRange = box.gMax - box.gMin;
  const bRange = box.bMax - box.bMin;

  let axis: "r" | "g" | "b";
  if (rRange >= gRange && rRange >= bRange) axis = "r";
  else if (gRange >= rRange && gRange >= bRange) axis = "g";
  else axis = "b";

  box.pixels.sort((a, b) => a[axis] - b[axis]);

  const mid = Math.floor(box.pixels.length / 2);
  return [
    makeBox(box.pixels.slice(0, mid)),
    makeBox(box.pixels.slice(mid)),
  ];
}

/** Average the pixels in a box to produce a swatch. */
function averageBox(box: ColorBox): Swatch {
  let rSum = 0, gSum = 0, bSum = 0;
  for (const p of box.pixels) {
    rSum += p.r;
    gSum += p.g;
    bSum += p.b;
  }
  const n = box.pixels.length;
  const r = Math.round(rSum / n);
  const g = Math.round(gSum / n);
  const b = Math.round(bSum / n);
  return { r, g, b, population: n, hex: rgbToHex(r, g, b) };
}

/** Volume of a box in RGB space. */
function boxVolume(box: ColorBox): number {
  return (box.rMax - box.rMin + 1) * (box.gMax - box.gMin + 1) * (box.bMax - box.bMin + 1);
}

/**
 * Sample RGBA pixel data down to an array of RGB values.
 * Skips transparent pixels and optionally downsamples.
 */
function samplePixels(data: Uint8ClampedArray, quality: number): RGB[] {
  const pixels: RGB[] = [];
  const step = Math.max(1, Math.floor(quality));
  for (let i = 0; i < data.length; i += 4 * step) {
    const a = data[i + 3];
    if (a < 128) continue; // skip transparent
    const r = data[i];
    const g = data[i + 1];
    const b = data[i + 2];
    // Skip near-white and near-black — they aren't useful palette colors
    if (r > 240 && g > 240 && b > 240) continue;
    if (r < 15 && g < 15 && b < 15) continue;
    pixels.push({ r, g, b });
  }
  return pixels;
}

/**
 * Extract a color palette from raw RGBA pixel data using MMCQ.
 *
 * @param data - Flat RGBA pixel array (e.g. from `ctx.getImageData().data`).
 * @param paletteSize - Number of colors to extract (default: 6).
 * @param quality - Pixel sampling step; 1 = every pixel, 5 = every 5th (default: 5).
 * @returns Array of swatches sorted by population (most dominant first).
 */
export function extractPalette(
  data: Uint8ClampedArray,
  paletteSize: number = 6,
  quality: number = 5,
): Swatch[] {
  const pixels = samplePixels(data, quality);
  if (pixels.length === 0) return [];

  // Start with one box containing all pixels, then iteratively split
  const boxes: ColorBox[] = [makeBox(pixels)];

  while (boxes.length < paletteSize) {
    // Pick the box with the largest volume to split
    let maxVol = -1;
    let maxIdx = 0;
    for (let i = 0; i < boxes.length; i++) {
      if (boxes[i].pixels.length < 2) continue;
      const vol = boxVolume(boxes[i]);
      if (vol > maxVol) {
        maxVol = vol;
        maxIdx = i;
      }
    }
    // Can't split further
    if (maxVol <= 0) break;

    const [a, b] = splitBox(boxes[maxIdx]);
    boxes.splice(maxIdx, 1, a, b);
  }

  return boxes
    .filter((b) => b.pixels.length > 0)
    .map(averageBox)
    .sort((a, b) => b.population - a.population);
}

/** Convert RGB 0-255 to HSL (h: 0-360, s: 0-1, l: 0-1). */
export function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  if (max === min) return [0, 0, l];
  const d = max - min;
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h = 0;
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
  else if (max === g) h = ((b - r) / d + 2) / 6;
  else h = ((r - g) / d + 4) / 6;
  return [h * 360, s, l];
}

/** Convert HSL (h: 0-360, s: 0-1, l: 0-1) to hex. */
export function hslToHex(h: number, s: number, l: number): string {
  const hue2rgb = (p: number, q: number, t: number) => {
    if (t < 0) t += 1; if (t > 1) t -= 1;
    if (t < 1 / 6) return p + (q - p) * 6 * t;
    if (t < 1 / 2) return q;
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
    return p;
  };
  h /= 360;
  let r: number, g: number, b: number;
  if (s === 0) {
    r = g = b = l;
  } else {
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
    const p = 2 * l - q;
    r = hue2rgb(p, q, h + 1 / 3);
    g = hue2rgb(p, q, h);
    b = hue2rgb(p, q, h - 1 / 3);
  }
  return rgbToHex(Math.round(r * 255), Math.round(g * 255), Math.round(b * 255));
}
