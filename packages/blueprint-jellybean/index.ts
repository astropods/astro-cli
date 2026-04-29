import path from "path";
import { fileURLToPath } from "node:url";
import { Resvg } from "@resvg/resvg-js";
import { CANVAS_W } from "./card";

export { buildBlueprintBadgeSvg, CANVAS_W, type CardColors } from "./card";
export { bufToDataUri, resolveAvatar } from "./assets";

// ─── Font paths ────────────────────────────────────────────────────────────────

const FONTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), "fonts");
const FONT_FILES = [
  path.join(FONTS_DIR, "Geist-Bold.ttf"),
  path.join(FONTS_DIR, "Geist-Regular.ttf"),
  path.join(FONTS_DIR, "Inter-Regular.ttf"),
  path.join(FONTS_DIR, "InterDisplay-SemiBold.ttf"),
  path.join(FONTS_DIR, "GeistMono-Regular.ttf"),
];

// ─── Rendering ────────────────────────────────────────────────────────────────

export function renderSvgToPng(svg: string): Buffer {
  const resvg = new Resvg(svg, {
    fitTo: { mode: "width", value: CANVAS_W },
    font: { fontFiles: FONT_FILES, loadSystemFonts: false },
    dpi: 300,
    imageRendering: 0,
    shapeRendering: 2,
    textRendering: 2,
  });
  return Buffer.from(resvg.render().asPng());
}
