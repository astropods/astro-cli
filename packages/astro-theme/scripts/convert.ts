/**
 * Hex → OKLCH color conversion script.
 * Converts designer's hex palette values to OKLCH format for Tailwind CSS.
 *
 * Usage: bun scripts/colors/convert.ts
 */

// --- Color Math ---

function hexToSrgb(hex: string): [number, number, number] {
  const h = hex.replace("#", "");
  return [
    parseInt(h.slice(0, 2), 16) / 255,
    parseInt(h.slice(2, 4), 16) / 255,
    parseInt(h.slice(4, 6), 16) / 255,
  ];
}

function srgbToLinear(c: number): number {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function linearToXyz(r: number, g: number, b: number): [number, number, number] {
  return [
    0.4124564 * r + 0.3575761 * g + 0.1804375 * b,
    0.2126729 * r + 0.7151522 * g + 0.0721750 * b,
    0.0193339 * r + 0.1191920 * g + 0.9503041 * b,
  ];
}

function xyzToOklab(x: number, y: number, z: number): [number, number, number] {
  const l_ = Math.cbrt(0.8189330101 * x + 0.3618667424 * y - 0.1288597137 * z);
  const m_ = Math.cbrt(0.0329845436 * x + 0.9293118715 * y + 0.0361456387 * z);
  const s_ = Math.cbrt(0.0482003018 * x + 0.2643662691 * y + 0.6338517070 * z);

  return [
    0.2104542553 * l_ + 0.7936177850 * m_ - 0.0040720468 * s_,
    1.9779984951 * l_ - 2.4285922050 * m_ + 0.4505937099 * s_,
    0.0259040371 * l_ + 0.7827717662 * m_ - 0.8086757660 * s_,
  ];
}

function oklabToOklch(L: number, a: number, b: number): [number, number, number] {
  const C = Math.sqrt(a * a + b * b);
  let h = (Math.atan2(b, a) * 180) / Math.PI;
  if (h < 0) h += 360;
  return [L, C, h];
}

function hexToOklch(hex: string): [number, number, number] {
  const [r, g, b] = hexToSrgb(hex).map(srgbToLinear);
  const [x, y, z] = linearToXyz(r, g, b);
  const [L, a, ob] = xyzToOklab(x, y, z);
  return oklabToOklch(L, a, ob);
}

function formatOklch([L, C, h]: [number, number, number]): string {
  const lStr = (L * 100).toFixed(2) + "%";
  const cStr = C.toFixed(4);
  const hStr = h.toFixed(3);
  // If chroma is near-zero, omit hue (achromatic)
  if (C < 0.0005) {
    return `oklch(${lStr} 0 0)`;
  }
  return `oklch(${lStr} ${cStr} ${hStr})`;
}

// --- Interpolation for missing 950 step ---

function interpolateOklch(
  a: [number, number, number],
  b: [number, number, number],
  t: number
): [number, number, number] {
  return [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
  ];
}

function generateScale(
  colors: Record<string, string>,
  extrapolate950 = true
): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [step, hex] of Object.entries(colors)) {
    result[step] = formatOklch(hexToOklch(hex));
  }
  // Extrapolate 950 from 800→900 trend if not provided
  if (extrapolate950 && !colors["950"] && colors["900"] && colors["800"]) {
    const c800 = hexToOklch(colors["800"]);
    const c900 = hexToOklch(colors["900"]);
    // Extend the 800→900 vector by the same amount
    const c950 = interpolateOklch(c800, c900, 2);
    // Clamp lightness to minimum 0
    c950[0] = Math.max(0, c950[0]);
    result["950"] = formatOklch(c950);
  }
  return result;
}

function generateFullScale(
  anchors: Record<string, string>,
  name: string
): Record<string, string> {
  return generateScale(anchors);
}

// --- Coral palette generation ---
// Designer provides coral-400 (#e07a60) and coral-500 (#c05a40).
// We generate a full scale using the red palette's lightness/chroma distribution
// as a reference curve, anchored to the designer's actual values.

function generateCoralScale(): Record<string, string> {
  const anchor400 = hexToOklch("#e07a60");
  const anchor500 = hexToOklch("#c05a40");
  const coralHue = (anchor400[2] + anchor500[2]) / 2; // ~35

  // Reference: Tailwind red palette lightness and chroma distribution
  // This gives us a natural-looking curve to follow
  const refLightness: Record<number, number> = {
    50: 0.971, 100: 0.936, 200: 0.885, 300: 0.809,
    400: 0.705, 500: 0.639, 600: 0.579, 700: 0.505,
    800: 0.445, 900: 0.396, 950: 0.258,
  };
  const refChroma: Record<number, number> = {
    50: 0.012, 100: 0.029, 200: 0.056, 300: 0.103,
    400: 0.171, 500: 0.212, 600: 0.220, 700: 0.191,
    800: 0.159, 900: 0.126, 950: 0.082,
  };

  // Scale the reference curve to pass through our anchor points
  // Anchor at 400: designer's L=0.688, ref L=0.705 → ratio
  // Anchor at 500: designer's L=0.589, ref L=0.639 → ratio
  const lScale400 = anchor400[0] / refLightness[400];
  const lScale500 = anchor500[0] / refLightness[500];
  const cScale400 = anchor400[1] / refChroma[400];
  const cScale500 = anchor500[1] / refChroma[500];

  const steps = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950];
  const result: Record<string, string> = {};

  for (const s of steps) {
    // Interpolate scale factor between 400 and 500 anchors
    const t = Math.max(0, Math.min(1, (s - 400) / 100));
    const lScale = s <= 400 ? lScale400 : s >= 500 ? lScale500 : lScale400 + (lScale500 - lScale400) * t;
    const cScale = s <= 400 ? cScale400 : s >= 500 ? cScale500 : cScale400 + (cScale500 - cScale400) * t;

    const L = Math.max(0.05, Math.min(0.99, refLightness[s] * lScale));
    const C = Math.max(0, refChroma[s] * cScale);

    result[String(s)] = formatOklch([L, C, coralHue]);
  }

  return result;
}

// --- Input: Designer's palettes ---

const designerTeal: Record<string, string> = {
  "50": "#f0fafa",
  "100": "#ccefee",
  "200": "#99dedc",
  "300": "#57c4c1",
  "400": "#2da5a2",
  "500": "#15827d",
  "600": "#0e6460",
  "700": "#0a4d4a",
  "800": "#073d3c",
  "900": "#041f1f",
};

const designerStone: Record<string, string> = {
  "50": "#f5f1e8",  // remapped from designer's stone-75
  "100": "#ede7d9", // remapped from designer's stone-150
  "200": "#e5dece",
  "300": "#d4c9b5",
  "400": "#b5a48e",
  "500": "#9a8a72",
  "600": "#7e7060",
  "700": "#5c5047",
  "800": "#3d342c",
  "900": "#1e1a15",
};

// --- Additional semantic colors for reference ---
const semanticColors: Record<string, string> = {
  "ink (foreground-light)": "#0d1f1e",
  "ink-muted": "#4a5e5d",
  "ink-faint": "#6b7e7c",
  "bg (dark background)": "#080c0a",
  "surface (dark card)": "#0f1a19",
  "surface-2 (dark elevated)": "#1a2420",
  "text (dark foreground)": "#ede7d9",
};

// --- Reverse conversion: OKLCH → Hex ---

function oklchToOklab(L: number, C: number, h: number): [number, number, number] {
  const hRad = (h * Math.PI) / 180;
  return [L, C * Math.cos(hRad), C * Math.sin(hRad)];
}

function oklabToXyz(L: number, a: number, b: number): [number, number, number] {
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.2914855480 * b;

  const l = l_ * l_ * l_;
  const m = m_ * m_ * m_;
  const s = s_ * s_ * s_;

  return [
    +1.2270138511 * l - 0.5577999807 * m + 0.2812561490 * s,
    -0.0405801784 * l + 1.1122568696 * m - 0.0716766787 * s,
    -0.0763812845 * l - 0.4214819784 * m + 1.5861632204 * s,
  ];
}

function linearToSrgb(c: number): number {
  return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
}

function oklchToHex(L: number, C: number, h: number): string {
  const [oL, oa, ob] = oklchToOklab(L, C, h);
  const [x, y, z] = oklabToXyz(oL, oa, ob);
  const r = Math.round(Math.max(0, Math.min(1, linearToSrgb(+3.2404542 * x - 1.5371385 * y - 0.4985314 * z))) * 255);
  const g = Math.round(Math.max(0, Math.min(1, linearToSrgb(-0.9692660 * x + 1.8760108 * y + 0.0415560 * z))) * 255);
  const b = Math.round(Math.max(0, Math.min(1, linearToSrgb(+0.0556434 * x - 0.2040259 * y + 1.0572252 * z))) * 255);
  return `#${r.toString(16).padStart(2, "0")}${g.toString(16).padStart(2, "0")}${b.toString(16).padStart(2, "0")}`;
}

function parseOklch(s: string): [number, number, number] {
  const m = s.match(/oklch\(([0-9.]+)%\s+([0-9.]+)\s+([0-9.]+)\)/);
  if (!m) throw new Error(`Cannot parse: ${s}`);
  return [parseFloat(m[1]) / 100, parseFloat(m[2]), parseFloat(m[3])];
}

// --- Run ---

console.log("=== TEAL PALETTE (OKLCH) ===\n");
const tealScale = generateFullScale(designerTeal, "teal");
for (const [step, value] of Object.entries(tealScale)) {
  const hex = designerTeal[step] ?? "(interpolated)";
  console.log(`  --color-teal-${step}: ${value};  /* ${hex} */`);
}

console.log("\n=== STONE PALETTE (OKLCH) ===\n");
const stoneScale = generateFullScale(designerStone, "stone");
for (const [step, value] of Object.entries(stoneScale)) {
  const hex = designerStone[step] ?? "(interpolated)";
  console.log(`  --color-stone-${step}: ${value};  /* ${hex} */`);
}

console.log("\n=== CORAL PALETTE (OKLCH) ===\n");
const coralScale = generateCoralScale();
for (const [step, value] of Object.entries(coralScale)) {
  console.log(`  --color-coral-${step}: ${value};`);
}

console.log("\n=== SEMANTIC REFERENCE VALUES (OKLCH) ===\n");
for (const [name, hex] of Object.entries(semanticColors)) {
  console.log(`  ${name}: ${formatOklch(hexToOklch(hex))};  /* ${hex} */`);
}

// --- Hex output for identity-gen package ---
console.log("\n=== HEX VALUES FOR IDENTITY-GEN ===\n");

console.log("// Teal");
for (const [step, oklchStr] of Object.entries(tealScale)) {
  const hex = designerTeal[step] ?? oklchToHex(...parseOklch(oklchStr));
  console.log(`  ${step}: "${hex}",`);
}

console.log("\n// Stone");
for (const [step, oklchStr] of Object.entries(stoneScale)) {
  const hex = designerStone[step] ?? oklchToHex(...parseOklch(oklchStr));
  console.log(`  ${step}: "${hex}",`);
}

console.log("\n// Coral");
for (const [step, oklchStr] of Object.entries(coralScale)) {
  const [L, C, h] = parseOklch(oklchStr);
  console.log(`  ${step}: "${oklchToHex(L, C, h)}",`);
}
