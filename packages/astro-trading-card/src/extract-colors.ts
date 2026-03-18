/**
 * Browser-only: extract card colors from an image source.
 *
 * Encapsulates the canvas → pixel data → palette → CardColors pipeline
 * so consumers don't need to orchestrate it manually.
 */

import type { CardColors } from "./types";
import { extractPalette } from "./mmcq";
import { pickCardColors } from "./colors";

/**
 * Extract a CardColors scheme from an image source (URL or data URI).
 *
 * Draws the image to an off-screen 64×64 canvas, extracts a palette via MMCQ,
 * and derives a full color scheme. Returns null if extraction fails.
 */
export function extractColorsFromImage(source: string): Promise<CardColors | null> {
  return new Promise((resolve) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = 64;
      canvas.height = 64;
      const ctx = canvas.getContext("2d")!;
      ctx.drawImage(img, 0, 0, 64, 64);
      const { data } = ctx.getImageData(0, 0, 64, 64);
      const palette = extractPalette(data, 8);
      resolve(pickCardColors(palette));
    };
    img.onerror = () => resolve(null);
    img.src = source;
  });
}

/**
 * Build a data URI from inline SVG content (no outer `<svg>` wrapper).
 * Useful for extracting colors from a generated identity SVG.
 */
export function svgToImageSource(innerSvg: string, size = 128): string {
  const full = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">${innerSvg}</svg>`;
  return "data:image/svg+xml;charset=utf-8," + encodeURIComponent(full);
}
