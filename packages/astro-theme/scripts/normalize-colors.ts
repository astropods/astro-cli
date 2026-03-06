/**
 * Color scale normalization script.
 *
 * Analyzes OKLCH color scales for irregularities in lightness, chroma, and hue
 * progressions, then generates mathematically smooth replacements.
 *
 * Approach:
 *  - Lightness: must be monotonically decreasing; enforce via isotonic regression
 *  - Chroma: must follow a smooth curve; detect jumps in second derivative
 *  - Hue: must be smooth; detect non-monotonic reversals and sudden jumps
 *
 * Usage: bun scripts/normalize-colors.ts
 */

// ── Types ──────────────────────────────────────────────────────────────

type LCH = [L: number, C: number, H: number];

const STEPS = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;
type Step = (typeof STEPS)[number];

// Normalized t values for each step (0..1)
const T_VALUES = STEPS.map((s) => (s - 50) / 900);

// ── Parse existing colors ──────────────────────────────────────────────

function parseOklch(s: string): LCH {
  const m = s.match(/oklch\(([0-9.]+)%\s+([0-9.]+)\s+([0-9.]+)\)/);
  if (!m) throw new Error(`Cannot parse: ${s}`);
  return [parseFloat(m[1]) / 100, parseFloat(m[2]), parseFloat(m[3])];
}

function formatOklch([L, C, H]: LCH): string {
  const lStr = (L * 100).toFixed(2) + "%";
  const cStr = C.toFixed(4);
  const hStr = H.toFixed(3);
  if (C < 0.0005) return `oklch(${lStr} 0 0)`;
  return `oklch(${lStr} ${cStr} ${hStr})`;
}

import { palettes, type ColorScale } from "../src/colors";

function scaleToLCH(scale: ColorScale): LCH[] {
  return STEPS.map((step) => parseOklch(scale[step]));
}

// ── Math utilities ─────────────────────────────────────────────────────

/** Natural cubic spline interpolation. Returns evaluator function. */
function cubicSpline(xs: number[], ys: number[]): (x: number) => number {
  if (xs.length < 2) return () => ys[0] ?? 0;
  if (xs.length === 2) {
    const slope = (ys[1] - ys[0]) / (xs[1] - xs[0]);
    return (x: number) => ys[0] + slope * (x - xs[0]);
  }
  if (xs.length === 3) {
    // Quadratic through 3 points
    const [x0, x1, x2] = xs;
    const [y0, y1, y2] = ys;
    return (x: number) => {
      const l0 = ((x - x1) * (x - x2)) / ((x0 - x1) * (x0 - x2));
      const l1 = ((x - x0) * (x - x2)) / ((x1 - x0) * (x1 - x2));
      const l2 = ((x - x0) * (x - x1)) / ((x2 - x0) * (x2 - x1));
      return y0 * l0 + y1 * l1 + y2 * l2;
    };
  }

  const n = xs.length - 1;
  const h: number[] = [];
  for (let i = 0; i < n; i++) h.push(xs[i + 1] - xs[i]);

  const alpha: number[] = new Array(n + 1).fill(0);
  for (let i = 1; i < n; i++) {
    alpha[i] =
      (3 / h[i]) * (ys[i + 1] - ys[i]) - (3 / h[i - 1]) * (ys[i] - ys[i - 1]);
  }

  const l: number[] = new Array(n + 1).fill(1);
  const mu: number[] = new Array(n + 1).fill(0);
  const z: number[] = new Array(n + 1).fill(0);

  for (let i = 1; i < n; i++) {
    l[i] = 2 * (xs[i + 1] - xs[i - 1]) - h[i - 1] * mu[i - 1];
    mu[i] = h[i] / l[i];
    z[i] = (alpha[i] - h[i - 1] * z[i - 1]) / l[i];
  }

  const c: number[] = new Array(n + 1).fill(0);
  const b: number[] = new Array(n).fill(0);
  const d: number[] = new Array(n).fill(0);

  for (let j = n - 1; j >= 0; j--) {
    c[j] = z[j] - mu[j] * c[j + 1];
    b[j] = (ys[j + 1] - ys[j]) / h[j] - (h[j] * (c[j + 1] + 2 * c[j])) / 3;
    d[j] = (c[j + 1] - c[j]) / (3 * h[j]);
  }

  return (x: number) => {
    let i = 0;
    for (let j = 0; j < n; j++) {
      if (x >= xs[j]) i = j;
    }
    const dx = x - xs[i];
    return ys[i] + b[i] * dx + c[i] * dx * dx + d[i] * dx * dx * dx;
  };
}

/**
 * Detect outliers using leave-one-out cross-validation with cubic spline.
 * For each point, fit a spline through all OTHER points and check the residual.
 * Points where the residual exceeds the threshold are outliers.
 */
function detectOutliersLOO(
  xs: number[],
  ys: number[],
  threshold: number,
): { outlierIndices: number[]; predicted: number[] } {
  const n = xs.length;
  const predicted = new Array(n).fill(0);
  const outlierIndices: number[] = [];

  for (let i = 0; i < n; i++) {
    // Leave out point i, fit spline through the rest
    const xsLeft = [...xs.slice(0, i), ...xs.slice(i + 1)];
    const ysLeft = [...ys.slice(0, i), ...ys.slice(i + 1)];

    const spline = cubicSpline(xsLeft, ysLeft);
    predicted[i] = spline(xs[i]);

    if (Math.abs(ys[i] - predicted[i]) > threshold) {
      outlierIndices.push(i);
    }
  }

  return { outlierIndices, predicted };
}

/**
 * Smooth values by removing outliers and refitting.
 * Returns the smoothed values at all original x positions.
 */
function smoothWithOutlierRemoval(
  xs: number[],
  ys: number[],
  threshold: number,
): { smoothed: number[]; outlierIndices: number[] } {
  const { outlierIndices } = detectOutliersLOO(xs, ys, threshold);

  if (outlierIndices.length === 0) {
    // No outliers — return original values
    return { smoothed: [...ys], outlierIndices: [] };
  }

  // Remove outliers and refit
  const outlierSet = new Set(outlierIndices);
  const cleanXs: number[] = [];
  const cleanYs: number[] = [];
  for (let i = 0; i < xs.length; i++) {
    if (!outlierSet.has(i)) {
      cleanXs.push(xs[i]);
      cleanYs.push(ys[i]);
    }
  }

  const spline = cubicSpline(cleanXs, cleanYs);
  const smoothed = xs.map((x, i) => (outlierSet.has(i) ? spline(x) : ys[i]));

  return { smoothed, outlierIndices };
}

/**
 * Enforce monotonic decrease using pool-adjacent-violators.
 */
function enforceMonotonicDecrease(values: number[]): number[] {
  const neg = values.map((v) => -v);
  const blocks: { value: number; weight: number; startIdx: number }[] = neg.map(
    (v, i) => ({ value: v, weight: 1, startIdx: i }),
  );

  let changed = true;
  while (changed) {
    changed = false;
    for (let i = 0; i < blocks.length - 1; i++) {
      if (blocks[i].value > blocks[i + 1].value) {
        // Pool
        const w = blocks[i].weight + blocks[i + 1].weight;
        const pooled =
          (blocks[i].weight * blocks[i].value +
            blocks[i + 1].weight * blocks[i + 1].value) /
          w;
        blocks[i] = { value: pooled, weight: w, startIdx: blocks[i].startIdx };
        blocks.splice(i + 1, 1);
        changed = true;
        break;
      }
    }
  }

  const result = new Array(values.length);
  for (const block of blocks) {
    for (let k = 0; k < block.weight; k++) {
      result[block.startIdx + k] = -block.value;
    }
  }
  return result;
}

// ── Analysis ───────────────────────────────────────────────────────────

interface Issue {
  step: Step;
  channel: "L" | "C" | "H";
  original: number;
  normalized: number;
}

interface AnalysisResult {
  name: string;
  original: LCH[];
  normalized: LCH[];
  issues: Issue[];
}

function analyzeScale(name: string, scale: ColorScale): AnalysisResult {
  const original = scaleToLCH(scale);
  const issues: Issue[] = [];

  const Ls = original.map(([L]) => L);
  const Cs = original.map(([, C]) => C);
  const Hs = original.map(([, , H]) => H);

  // ── Lightness: enforce monotonic decrease ──
  const monoL = enforceMonotonicDecrease(Ls);
  for (let i = 0; i < STEPS.length; i++) {
    if (Math.abs(Ls[i] - monoL[i]) > 0.002) {
      issues.push({ step: STEPS[i], channel: "L", original: Ls[i], normalized: monoL[i] });
    }
  }

  // ── Chroma: smooth bell curve ──
  // Use LOO with threshold scaled to chroma range
  const chromaRange = Math.max(...Cs) - Math.min(...Cs);
  const chromaThreshold = chromaRange * 0.08; // 8% of range
  const { smoothed: smoothC, outlierIndices: chromaOutliers } = smoothWithOutlierRemoval(
    T_VALUES,
    Cs,
    chromaThreshold,
  );
  const clampedC = smoothC.map((c) => Math.max(0, c));

  for (const idx of chromaOutliers) {
    issues.push({
      step: STEPS[idx],
      channel: "C",
      original: Cs[idx],
      normalized: clampedC[idx],
    });
  }

  // ── Hue: smooth progression ──
  const maxChroma = Math.max(...Cs);
  let normalizedH = [...Hs];

  if (maxChroma > 0.001) {
    // Compute hue differences to detect reversals/jumps
    const hueDiffs: number[] = [];
    for (let i = 1; i < Hs.length; i++) {
      hueDiffs.push(Hs[i] - Hs[i - 1]);
    }

    // Use LOO with threshold based on median absolute hue step
    const absHueDiffs = hueDiffs.map(Math.abs);
    const medianHueDiff = [...absHueDiffs].sort((a, b) => a - b)[Math.floor(absHueDiffs.length / 2)];
    // Threshold: any LOO residual > 2× the median step size is an outlier
    const hueThreshold = Math.max(medianHueDiff * 2, 2.0); // minimum 2°

    const { smoothed: smoothH, outlierIndices: hueOutliers } = smoothWithOutlierRemoval(
      T_VALUES,
      Hs,
      hueThreshold,
    );
    normalizedH = smoothH;

    for (const idx of hueOutliers) {
      issues.push({
        step: STEPS[idx],
        channel: "H",
        original: Hs[idx],
        normalized: smoothH[idx],
      });
    }
  }

  const normalized: LCH[] = STEPS.map((_, i) => [monoL[i], clampedC[i], normalizedH[i]]);

  return { name, original, normalized, issues };
}

// ── Main ───────────────────────────────────────────────────────────────

console.log("╔══════════════════════════════════════════════════════════════╗");
console.log("║            OKLCH Color Scale Normalization Report           ║");
console.log("╚══════════════════════════════════════════════════════════════╝\n");

const results: AnalysisResult[] = [];

for (const [name, scale] of Object.entries(palettes)) {
  const result = analyzeScale(name, scale);
  results.push(result);
}

// ── Print raw channel data for debug ──

for (const r of results) {
  const Ls = r.original.map(([L]) => L);
  const Cs = r.original.map(([, C]) => C);
  const Hs = r.original.map(([, , H]) => H);

  // Check for hue direction reversals
  const hueDiffs: number[] = [];
  for (let i = 1; i < Hs.length; i++) hueDiffs.push(Hs[i] - Hs[i - 1]);

  // Check for lightness monotonicity violations
  const lViolations = [];
  for (let i = 1; i < Ls.length; i++) {
    if (Ls[i] >= Ls[i - 1]) lViolations.push(i);
  }

  // Check for hue reversals (sign changes in consecutive diffs)
  const hueReversals = [];
  for (let i = 1; i < hueDiffs.length; i++) {
    // A reversal is when the direction changes and the magnitude is significant
    if (Math.sign(hueDiffs[i]) !== Math.sign(hueDiffs[i - 1]) &&
        Math.abs(hueDiffs[i]) > 2 && Math.abs(hueDiffs[i - 1]) > 2) {
      hueReversals.push(i + 1); // +1 because hueDiffs is offset by 1
    }
  }

  // Only print if there are interesting findings
  if (lViolations.length > 0 || hueReversals.length > 0 || r.issues.length > 0) {
    console.log(`\n── ${r.name} ──`);
    console.log(`  Step   L%       C        H°       ΔH°`);
    for (let i = 0; i < STEPS.length; i++) {
      const diffStr = i > 0 ? (Hs[i] - Hs[i - 1]).toFixed(1).padStart(7) : "      —";
      const lFlag = lViolations.includes(i) ? " ⚠L" : "";
      const hFlag = hueReversals.includes(i) ? " ⚠H" : "";
      console.log(
        `  ${String(STEPS[i]).padStart(4)}   ${(Ls[i] * 100).toFixed(2).padStart(6)}  ${Cs[i].toFixed(4).padStart(7)}  ${Hs[i].toFixed(1).padStart(7)}  ${diffStr}${lFlag}${hFlag}`,
      );
    }
    if (lViolations.length > 0) console.log(`  ⚠L = lightness increased`);
    if (hueReversals.length > 0) console.log(`  ⚠H = hue direction reversal`);
  }
}

// ── Print summary ──

console.log(`\n${"─".repeat(62)}`);
let totalIssues = 0;
for (const r of results) {
  if (r.issues.length === 0) {
    console.log(`  ✓ ${r.name}`);
  } else {
    totalIssues += r.issues.length;
    const issueDescs = r.issues.map((i) => {
      if (i.channel === "L")
        return `L[${i.step}]: ${(i.original * 100).toFixed(2)}→${(i.normalized * 100).toFixed(2)}%`;
      if (i.channel === "C") return `C[${i.step}]: ${i.original.toFixed(4)}→${i.normalized.toFixed(4)}`;
      return `H[${i.step}]: ${i.original.toFixed(1)}→${i.normalized.toFixed(1)}°`;
    });
    console.log(`  ✗ ${r.name}: ${issueDescs.join(", ")}`);
  }
}
console.log(`\nTotal: ${totalIssues} corrections across ${results.filter((r) => r.issues.length > 0).length} scales`);

// ── Print corrected definitions for scales with issues ──

const scalesToFix = results.filter((r) => r.issues.length > 0);
if (scalesToFix.length > 0) {
  console.log("\n\n── Corrected Definitions ──────────────────────────────────\n");

  for (const r of scalesToFix) {
    console.log(`export const ${r.name}: ColorScale = {`);
    for (let i = 0; i < STEPS.length; i++) {
      const hasIssue = r.issues.some((iss) => iss.step === STEPS[i]);
      const marker = hasIssue ? " // ← corrected" : "";
      console.log(`  ${STEPS[i]}: "${formatOklch(r.normalized[i])}",${marker}`);
    }
    console.log("};\n");
  }
}
