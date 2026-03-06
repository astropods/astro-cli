/**
 * Generates hue-smoothed versions of all color scales for visual comparison.
 *
 * Strategy: only smooth the HUE channel via weighted cubic polynomial regression.
 * Lightness and chroma are left exactly as the designer chose them.
 *
 * The polynomial is weighted by chroma² so high-saturation steps (where hue is
 * most visible) anchor the curve, and low-chroma endpoints (where hue is barely
 * perceptible) are allowed to shift toward the trend.
 *
 * Usage: bun scripts/normalize-colors.ts
 */

import { palettes, type ColorScale } from "../src/colors";

type LCH = [L: number, C: number, H: number];
const STEPS = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;
const T_VALUES = STEPS.map((s) => (s - 50) / 900); // 0..1

function parseOklch(s: string): LCH {
  const m = s.match(/oklch\(([0-9.]+)%\s+([0-9.]+)\s+([0-9.]+)\)/);
  if (!m) throw new Error(`Cannot parse: ${s}`);
  return [parseFloat(m[1]) / 100, parseFloat(m[2]), parseFloat(m[3])];
}

function formatOklch([L, C, H]: LCH): string {
  const lStr = (L * 100).toFixed(2) + "%";
  if (C < 0.0005) return `oklch(${lStr} 0 0)`;
  return `oklch(${lStr} ${C.toFixed(4)} ${H.toFixed(3)})`;
}

// ── Weighted polynomial regression ─────────────────────────────────────

function polyRegression(
  xs: number[],
  ys: number[],
  degree: number,
  weights?: number[],
): (x: number) => number {
  const n = xs.length;
  const w = weights ?? new Array(n).fill(1);
  const d = degree + 1;

  // Normal equations: (X^T W X) a = X^T W y
  const A = Array.from({ length: d }, () => new Array(d).fill(0));
  const b = new Array(d).fill(0);

  for (let i = 0; i < n; i++) {
    for (let j = 0; j < d; j++) {
      for (let k = 0; k < d; k++) {
        A[j][k] += w[i] * Math.pow(xs[i], j + k);
      }
      b[j] += w[i] * Math.pow(xs[i], j) * ys[i];
    }
  }

  // Gaussian elimination with partial pivoting
  for (let col = 0; col < d; col++) {
    let maxRow = col;
    for (let row = col + 1; row < d; row++) {
      if (Math.abs(A[row][col]) > Math.abs(A[maxRow][col])) maxRow = row;
    }
    [A[col], A[maxRow]] = [A[maxRow], A[col]];
    [b[col], b[maxRow]] = [b[maxRow], b[col]];

    for (let row = col + 1; row < d; row++) {
      const factor = A[row][col] / A[col][col];
      for (let k = col; k < d; k++) A[row][k] -= factor * A[col][k];
      b[row] -= factor * b[col];
    }
  }

  // Back substitution
  const coeffs = new Array(d).fill(0);
  for (let i = d - 1; i >= 0; i--) {
    let sum = b[i];
    for (let j = i + 1; j < d; j++) sum -= A[i][j] * coeffs[j];
    coeffs[i] = sum / A[i][i];
  }

  return (x: number) => {
    let result = 0;
    for (let i = 0; i < d; i++) result += coeffs[i] * Math.pow(x, i);
    return result;
  };
}

// ── Generate comparisons ───────────────────────────────────────────────

// We need the ORIGINAL amber (before our earlier fix) for comparison
const ORIGINAL_AMBER: Record<string, string> = {
  "700": "oklch(55.24% 0.1539 31.992)",
  "800": "oklch(47.26% 0.1244 41.392)",
};

console.log("── Hue Smoothing Report (cubic poly regression, weighted by C²) ──\n");

const output: { name: string; original: string[]; normalized: string[]; changed: boolean }[] = [];

for (const [name, scale] of Object.entries(palettes)) {
  const lch = STEPS.map((step) => parseOklch(scale[step]));
  const Ls = lch.map(([L]) => L);
  const Cs = lch.map(([, C]) => C);
  const Hs = lch.map(([, , H]) => H);

  const isAchromatic = Math.max(...Cs) < 0.001;

  // For amber, use original unfixed values for the "old" version
  const origStrings = STEPS.map((step, i) => {
    if (name === "amber" && ORIGINAL_AMBER[String(step)]) {
      return ORIGINAL_AMBER[String(step)];
    }
    return scale[step];
  });

  // For amber's hue regression, use the original (unfixed) values
  const origHs = [...Hs];
  if (name === "amber") {
    const orig700 = parseOklch(ORIGINAL_AMBER["700"]);
    const orig800 = parseOklch(ORIGINAL_AMBER["800"]);
    origHs[7] = orig700[2]; // 31.992
    origHs[8] = orig800[2]; // 41.392
  }

  let smoothedH = Hs;
  if (!isAchromatic) {
    // First detect hue outliers via neighbor-interpolation check
    const outlierSet = new Set<number>();
    for (let i = 1; i < origHs.length - 1; i++) {
      if (Cs[i] < 0.01) continue; // skip very low chroma
      const expected = (origHs[i - 1] + origHs[i + 1]) / 2;
      const deviation = Math.abs(origHs[i] - expected);
      const span = Math.abs(origHs[i + 1] - origHs[i - 1]);
      if (deviation > Math.max(span * 0.4, 3.0)) {
        outlierSet.add(i);
      }
    }

    // Fit cubic polynomial EXCLUDING outliers
    const fitTs: number[] = [];
    const fitHs: number[] = [];
    const fitWs: number[] = [];
    for (let i = 0; i < origHs.length; i++) {
      if (!outlierSet.has(i)) {
        fitTs.push(T_VALUES[i]);
        fitHs.push(origHs[i]);
        fitWs.push(Cs[i] * Cs[i]);
      }
    }

    const huePoly = polyRegression(fitTs, fitHs, 3, fitWs);
    smoothedH = T_VALUES.map((t) => huePoly(t));
  }

  const normalized: LCH[] = STEPS.map((_, i) => [Ls[i], Cs[i], smoothedH[i]]);
  const normStrings = STEPS.map((_, i) => formatOklch(normalized[i]));

  // Check for meaningful changes
  let maxHueDelta = 0;
  const changes: string[] = [];
  for (let i = 0; i < STEPS.length; i++) {
    // Compare against ORIGINAL values (not the already-fixed amber)
    const origH = origHs[i];
    const delta = Math.abs(origH - smoothedH[i]);
    if (delta > maxHueDelta) maxHueDelta = delta;
    if (delta > 0.5) {
      changes.push(
        `  ${String(STEPS[i]).padStart(4)}: ${origH.toFixed(1).padStart(6)}° → ${smoothedH[i].toFixed(1)}° (Δ${delta.toFixed(1)}°)${Cs[i] < 0.03 ? "  [low C]" : ""}`,
      );
    }
  }

  const changed = maxHueDelta > 0.5;
  output.push({ name, original: origStrings, normalized: normStrings, changed });

  if (changed) {
    console.log(`${name}: max ΔH = ${maxHueDelta.toFixed(1)}°`);
    for (const c of changes) console.log(c);
    console.log();
  } else {
    console.log(`${name}: no hue changes`);
  }
}

// ── Output TypeScript for colors.ts ────────────────────────────────────

console.log("\n── TypeScript Output ──\n");

for (const { name, original, normalized, changed } of output) {
  if (!changed) continue;

  // New (smoothed) version
  console.log(`export const ${name}: ColorScale = {`);
  for (let i = 0; i < STEPS.length; i++) {
    console.log(`  ${STEPS[i]}: "${normalized[i]}",`);
  }
  console.log("};\n");

  // Old version for comparison
  console.log(`export const ${name}Old: ColorScale = {`);
  for (let i = 0; i < STEPS.length; i++) {
    console.log(`  ${STEPS[i]}: "${original[i]}",`);
  }
  console.log("};\n");
}
