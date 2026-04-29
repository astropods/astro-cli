/**
 * Image-loading utilities for the badge renderer.
 *
 * resvg cannot resolve external URLs — every image referenced in the SVG must
 * be a data URI. This module handles fetching/reading image bytes and
 * converting them to embeddable data URIs, with special handling for SVG
 * content stored with a raster extension (common for AI-generated avatars).
 */

import path from "path";
import { readFile } from "node:fs/promises";
import { Resvg } from "@resvg/resvg-js";

// ─── oklch → hex ───────────────────────────────────────────────────────────────
// resvg doesn't support oklch() — replace all occurrences in SVG source before
// rasterizing so avatar colors render correctly.

function oklchToHex(l: number, c: number, h: number): string {
  const hRad = (h * Math.PI) / 180;
  const a = c * Math.cos(hRad);
  const b = c * Math.sin(hRad);

  const l_ = l + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = l - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = l - 0.0894841775 * a - 1.2914855480 * b;

  const lc = l_ ** 3, mc = m_ ** 3, sc = s_ ** 3;

  const toSrgb = (x: number) =>
    x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055;

  const r = toSrgb(+4.0767416621 * lc - 3.3077115913 * mc + 0.2309699292 * sc);
  const g = toSrgb(-1.2684380046 * lc + 2.6097574011 * mc - 0.3413193965 * sc);
  const bv = toSrgb(-0.0041960863 * lc - 0.7034186147 * mc + 1.7076147010 * sc);

  const ch = (v: number) => Math.round(Math.max(0, Math.min(1, v)) * 255).toString(16).padStart(2, "0");
  return `#${ch(r)}${ch(g)}${ch(bv)}`;
}

function replaceOklch(svg: string): string {
  // Match oklch(L% C H) — L may be a percentage or a bare number (0..1)
  return svg.replace(/oklch\(\s*([\d.]+)(%?)\s+([\d.]+)\s+([\d.]+)\s*\)/g, (_, lRaw, pct, cRaw, hRaw) => {
    const l = pct === "%" ? parseFloat(lRaw) / 100 : parseFloat(lRaw);
    return oklchToHex(l, parseFloat(cRaw), parseFloat(hRaw));
  });
}

/**
 * Convert raw image bytes to an SVG-embeddable data URI.
 *
 * Sniffs the actual format from magic bytes, ignoring the file extension.
 * SVG content (detected by a leading `<`) is rasterized to PNG via resvg
 * because nested SVG images in data URIs are not supported by resvg.
 */
export function bufToDataUri(buf: Buffer): string | null {
  const head = buf.slice(0, 64).toString("utf8").trimStart();
  if (head.startsWith("<")) {
    // Convert oklch() colors to hex — resvg doesn't support oklch.
    const svgSrc = Buffer.from(replaceOklch(buf.toString("utf8")));
    // Try original size first, fall back to a fixed 512px wide render.
    // Never return an svg+xml data URI — resvg can't render nested SVG images.
    for (const fitTo of [{ mode: "original" as const }, { mode: "width" as const, value: 512 }]) {
      try {
        const png = Buffer.from(
          new Resvg(svgSrc, { fitTo, font: { loadSystemFonts: true } }).render().asPng(),
        );
        return `data:image/png;base64,${png.toString("base64")}`;
      } catch { /* try next */ }
    }
    return null;
  }
  const ct = buf[0] === 0x89 && buf[1] === 0x50 ? "image/png" : "image/jpeg";
  return `data:${ct};base64,${buf.toString("base64")}`;
}

/**
 * Read a file from staticDir and return its bytes.
 * Returns null if the file is missing or the path escapes the directory.
 */
async function readStaticFile(staticDir: string, assetPath: string): Promise<Buffer | null> {
  try {
    const base     = path.resolve(staticDir);
    const resolved = path.resolve(base, "." + assetPath);
    if (!resolved.startsWith(base + path.sep)) return null;
    return await readFile(resolved);
  } catch {
    return null;
  }
}

/**
 * Fetch a remote URL and return it as a data URI.
 * Handles absolute CDN URLs and relative paths resolved against apiUrl.
 */
async function fetchDataUri(url: string, apiUrl: string): Promise<string | null> {
  try {
    const abs  = url.startsWith("/") ? `${apiUrl}${url}` : url;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 2000);
    let res: Response;
    try { res = await fetch(abs, { signal: ctrl.signal }); }
    finally { clearTimeout(timer); }
    if (!res.ok) return null;
    return bufToDataUri(Buffer.from(await res.arrayBuffer()));
  } catch {
    return null;
  }
}

/**
 * Resolve an avatar path to a data URI.
 *
 * When staticDir is provided, /assets/ paths are read directly from disk —
 * no HTTP round-trip back to the same server process. Absolute CDN URLs and
 * other paths fall back to a plain HTTP fetch.
 */
export async function resolveAvatar(
  url: string,
  apiUrl: string,
  staticDir: string | undefined,
): Promise<string | null> {
  if (staticDir && url.startsWith("/assets/")) {
    const buf = await readStaticFile(staticDir, url);
    return buf ? bufToDataUri(buf) : null;
  }
  return fetchDataUri(url, apiUrl);
}
