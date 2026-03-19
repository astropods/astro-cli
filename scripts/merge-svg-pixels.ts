#!/usr/bin/env bun
/**
 * Merges adjacent same-colored rectangles in pixel-art SVGs to eliminate
 * the "screen door" rendering artifact caused by sub-pixel gaps between
 * adjacent <path> rectangles.
 *
 * For each color, connected components are flood-filled, then boundary edges
 * are traced into closed loops. All loops (outer boundary + interior holes)
 * are combined into a single <path> with fill-rule="evenodd" so that holes
 * are correctly cut out.
 *
 * Usage: bun scripts/merge-svg-pixels.ts [--dry-run] [glob...]
 *        Defaults to assets/placeholders/accounts/avatar_*.svg
 */

import { readFileSync, writeFileSync } from "fs";
import { globSync } from "fs";
import path from "path";

// ---------------------------------------------------------------------------
// 1. Parse rectangles from SVG path data
// ---------------------------------------------------------------------------

interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  fill: string;
}

function parsePathToRect(d: string): { left: number; top: number; right: number; bottom: number } | null {
  // Format: M x1 y1 H x2 V y2 H x3 V y3 Z
  const re = /[MHVZ]/gi;
  const parts = d.split(re).filter(Boolean);
  if (parts.length < 4) return null;

  const mParts = parts[0].trim().split(/[\s,]+/).map(Number);
  if (mParts.length < 2) return null;
  const [mx, my] = mParts;
  const hx1 = Number(parts[1].trim());
  const vy1 = Number(parts[2].trim());

  return {
    left: Math.min(mx, hx1),
    right: Math.max(mx, hx1),
    top: Math.min(my, vy1),
    bottom: Math.max(my, vy1),
  };
}

function parseSvg(content: string): { width: string; height: string; viewBox: string; rects: Rect[] } {
  const svgMatch = content.match(/<svg\s+([^>]+)>/);
  if (!svgMatch) throw new Error("No <svg> tag found");

  const widthMatch = svgMatch[1].match(/width="([^"]+)"/);
  const heightMatch = svgMatch[1].match(/height="([^"]+)"/);
  const viewBoxMatch = svgMatch[1].match(/viewBox="([^"]+)"/);

  const width = widthMatch?.[1] ?? "149";
  const height = heightMatch?.[1] ?? "157";
  const viewBox = viewBoxMatch?.[1] ?? `0 0 ${width} ${height}`;

  const pathRe = /<path\s+d="([^"]+)"\s+fill="([^"]+)"\s*\/>/g;
  const rects: Rect[] = [];
  let match;
  while ((match = pathRe.exec(content)) !== null) {
    const bounds = parsePathToRect(match[1]);
    if (bounds) rects.push({ ...bounds, fill: match[2] });
  }

  return { width, height, viewBox, rects };
}

// ---------------------------------------------------------------------------
// 2. Snap to grid
// ---------------------------------------------------------------------------

function detectGridSize(rects: Rect[]): number {
  const sizes: number[] = [];
  for (const r of rects) {
    sizes.push(r.right - r.left);
    sizes.push(r.bottom - r.top);
  }
  const rounded = sizes.map((s) => Math.round(s * 100) / 100);
  const counts = new Map<number, number>();
  for (const s of rounded) counts.set(s, (counts.get(s) ?? 0) + 1);
  let best = 0;
  let bestCount = 0;
  for (const [size, count] of counts) {
    if (count > bestCount) { best = size; bestCount = count; }
  }
  return best;
}

function buildGrid(rects: Rect[], gridSize: number): { grid: Map<string, string>; cols: number; rows: number } {
  const grid = new Map<string, string>();
  let maxCol = 0;
  let maxRow = 0;
  for (const r of rects) {
    const col = Math.round(r.left / gridSize);
    const row = Math.round(r.top / gridSize);
    maxCol = Math.max(maxCol, col);
    maxRow = Math.max(maxRow, row);
    grid.set(`${col},${row}`, r.fill);
  }
  return { grid, cols: maxCol + 1, rows: maxRow + 1 };
}

// ---------------------------------------------------------------------------
// 3. Connected components via flood fill
// ---------------------------------------------------------------------------

function findConnectedComponents(grid: Map<string, string>): Map<string, Set<string>[]> {
  const byColor = new Map<string, Set<string>>();
  for (const [key, fill] of grid) {
    if (!byColor.has(fill)) byColor.set(fill, new Set());
    byColor.get(fill)!.add(key);
  }

  const result = new Map<string, Set<string>[]>();
  for (const [fill, cells] of byColor) {
    const remaining = new Set(cells);
    const components: Set<string>[] = [];
    while (remaining.size > 0) {
      const start = remaining.values().next().value!;
      const component = new Set<string>();
      const queue = [start];
      remaining.delete(start);
      while (queue.length > 0) {
        const current = queue.pop()!;
        component.add(current);
        const [col, row] = current.split(",").map(Number);
        for (const [dc, dr] of [[0, -1], [0, 1], [-1, 0], [1, 0]]) {
          const nk = `${col + dc},${row + dr}`;
          if (remaining.has(nk)) { remaining.delete(nk); queue.push(nk); }
        }
      }
      components.push(component);
    }
    result.set(fill, components);
  }
  return result;
}

// ---------------------------------------------------------------------------
// 4. Trace contour edges → closed loops → single compound path
// ---------------------------------------------------------------------------

function roundNum(n: number): number {
  return Math.round(n * 100) / 100;
}

/**
 * Traces ALL boundary loops for a connected component and returns a single
 * compound SVG path string. Multiple loops (outer + holes) are concatenated
 * into one `d` attribute; the caller adds `fill-rule="evenodd"`.
 */
function traceCompoundPath(component: Set<string>, gridSize: number): string {
  // Collect boundary edges with consistent winding:
  //   outer boundaries → CCW, hole boundaries → CW  (or vice-versa)
  // With evenodd fill-rule the winding doesn't matter, just need closed loops.
  const edges: { x1: number; y1: number; x2: number; y2: number; used: boolean }[] = [];

  for (const key of component) {
    const [col, row] = key.split(",").map(Number);
    const x = roundNum(col * gridSize);
    const y = roundNum(row * gridSize);
    const x2 = roundNum(x + gridSize);
    const y2 = roundNum(y + gridSize);

    // Top edge
    if (!component.has(`${col},${row - 1}`))
      edges.push({ x1: x, y1: y, x2, y2: y, used: false });
    // Bottom edge
    if (!component.has(`${col},${row + 1}`))
      edges.push({ x1: x2, y1: y2, x2: x, y2: y2, used: false });
    // Left edge
    if (!component.has(`${col - 1},${row}`))
      edges.push({ x1: x, y1: y2, x2: x, y2: y, used: false });
    // Right edge
    if (!component.has(`${col + 1},${row}`))
      edges.push({ x1: x2, y1: y, x2: x2, y2: y2, used: false });
  }

  // Index edges by start point for fast lookup
  const startIndex = new Map<string, number[]>();
  for (let i = 0; i < edges.length; i++) {
    const key = `${edges[i].x1},${edges[i].y1}`;
    if (!startIndex.has(key)) startIndex.set(key, []);
    startIndex.get(key)!.push(i);
  }

  // Chain edges into closed loops, then combine into one compound path
  const loops: [number, number][][] = [];

  for (let i = 0; i < edges.length; i++) {
    if (edges[i].used) continue;

    const polygon: [number, number][] = [];
    let idx = i;

    while (!edges[idx].used) {
      edges[idx].used = true;
      polygon.push([edges[idx].x1, edges[idx].y1]);

      const nextKey = `${edges[idx].x2},${edges[idx].y2}`;
      const candidates = startIndex.get(nextKey);
      if (!candidates) break;

      let found = false;
      for (const c of candidates) {
        if (!edges[c].used) { idx = c; found = true; break; }
      }
      if (!found) break;
    }

    if (polygon.length >= 3) {
      loops.push(removeCollinearPoints(polygon));
    }
  }

  // Build a single compound path from all loops
  return loops.map(polygonToPath).join("");
}

function removeCollinearPoints(points: [number, number][]): [number, number][] {
  if (points.length < 3) return points;
  const result: [number, number][] = [];
  const n = points.length;
  for (let i = 0; i < n; i++) {
    const prev = points[(i - 1 + n) % n];
    const curr = points[i];
    const next = points[(i + 1) % n];
    const dx1 = curr[0] - prev[0];
    const dy1 = curr[1] - prev[1];
    const dx2 = next[0] - curr[0];
    const dy2 = next[1] - curr[1];
    if (Math.abs(dx1 * dy2 - dy1 * dx2) > 0.001) result.push(curr);
  }
  return result;
}

function polygonToPath(points: [number, number][]): string {
  if (points.length === 0) return "";
  let d = `M${roundNum(points[0][0])} ${roundNum(points[0][1])}`;
  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1];
    const curr = points[i];
    if (roundNum(curr[1]) === roundNum(prev[1])) {
      d += `H${roundNum(curr[0])}`;
    } else if (roundNum(curr[0]) === roundNum(prev[0])) {
      d += `V${roundNum(curr[1])}`;
    } else {
      d += `L${roundNum(curr[0])} ${roundNum(curr[1])}`;
    }
  }
  d += "Z";
  return d;
}

// ---------------------------------------------------------------------------
// 5. Generate output SVG
// ---------------------------------------------------------------------------

function generateSvg(
  width: string,
  height: string,
  viewBox: string,
  components: Map<string, Set<string>[]>,
  gridSize: number,
): string {
  let paths = "";
  const sortedColors = [...components.keys()].sort();

  for (const fill of sortedColors) {
    const comps = components.get(fill)!;
    // Merge ALL connected components of the same color into one <path>
    // so we get a single element per color with proper hole handling.
    const allLoops: string[] = [];
    for (const comp of comps) {
      allLoops.push(traceCompoundPath(comp, gridSize));
    }
    const combinedD = allLoops.join("");
    if (combinedD) {
      paths += `<path d="${combinedD}" fill="${fill}" fill-rule="evenodd"/>\n`;
    }
  }

  return `<svg width="${width}" height="${height}" viewBox="${viewBox}" fill="none" xmlns="http://www.w3.org/2000/svg">\n${paths}</svg>\n`;
}

// ---------------------------------------------------------------------------
// 6. Main
// ---------------------------------------------------------------------------

function processFile(filePath: string, dryRun: boolean): { before: number; after: number } {
  const content = readFileSync(filePath, "utf-8");
  const { width, height, viewBox, rects } = parseSvg(content);

  if (rects.length === 0) {
    console.log(`  Skipping ${filePath}: no rectangles found`);
    return { before: 0, after: 0 };
  }

  const gridSize = detectGridSize(rects);
  const { grid } = buildGrid(rects, gridSize);
  const components = findConnectedComponents(grid);

  const output = generateSvg(width, height, viewBox, components, gridSize);
  const outputPathCount = (output.match(/<path/g) ?? []).length;

  if (!dryRun) {
    writeFileSync(filePath, output);
  }

  return { before: rects.length, after: outputPathCount };
}

// CLI
const args = process.argv.slice(2);
const dryRun = args.includes("--dry-run");
const fileArgs = args.filter((a) => a !== "--dry-run");

const defaultGlob = "assets/placeholders/accounts/avatar_*.svg";
const patterns = fileArgs.length > 0 ? fileArgs : [defaultGlob];

let files: string[] = [];
for (const pattern of patterns) {
  const matched = globSync(pattern, { cwd: process.cwd() });
  files.push(...matched.map((f) => path.resolve(process.cwd(), f)));
}

if (files.length === 0) {
  console.error("No files matched. Provide glob patterns or run from repo root.");
  process.exit(1);
}

console.log(`Processing ${files.length} SVG files${dryRun ? " (dry run)" : ""}...\n`);

let totalBefore = 0;
let totalAfter = 0;

for (const file of files.sort()) {
  const { before, after } = processFile(file, dryRun);
  totalBefore += before;
  totalAfter += after;
  console.log(`  ${path.basename(file)}: ${before} paths → ${after} paths`);
}

console.log(`\nTotal: ${totalBefore} → ${totalAfter} paths (${Math.round((1 - totalAfter / totalBefore) * 100)}% reduction)`);
