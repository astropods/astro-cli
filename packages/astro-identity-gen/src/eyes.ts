export type EyeStyle = "dots" | "rings" | "slits" | "triangles" | "dashes" | "squares" | "semicircles" | "diamonds";

export const eyeStyles: EyeStyle[] = [
  "dots",
  "rings",
  "slits",
  "triangles",
  "dashes",
  "squares",
  "semicircles",
  "diamonds",
];

export interface EyeParams {
  /** Style for the left eye. */
  leftStyle: EyeStyle;
  /** Style for the right eye. */
  rightStyle: EyeStyle;
  /** Distance between eye centers as a fraction of size (0.15–0.35). */
  spacing: number;
  /** Eye size as a fraction of size (0.04–0.1). */
  eyeSize: number;
}

/** Render a single eye at (ex, ey) with the given style. */
function buildEye(
  style: EyeStyle,
  ex: number,
  ey: number,
  r: number,
  color: string,
): string {
  const sw = Math.max(1, r * 0.4);

  switch (style) {
    case "dots":
      return `<circle cx="${ex}" cy="${ey}" r="${r}" fill="${color}" />`;

    case "rings":
      return `<circle cx="${ex}" cy="${ey}" r="${r}" fill="none" stroke="${color}" stroke-width="${sw}" />`;

    case "slits":
      return `<line x1="${ex}" y1="${ey - r}" x2="${ex}" y2="${ey + r}" stroke="${color}" stroke-width="${sw}" stroke-linecap="round" />`;

    case "triangles": {
      const h = r * 1.2;
      return `<polygon points="${ex},${ey - h} ${ex - h},${ey + h * 0.6} ${ex + h},${ey + h * 0.6}" fill="${color}" />`;
    }

    case "dashes":
      return `<line x1="${ex - r}" y1="${ey}" x2="${ex + r}" y2="${ey}" stroke="${color}" stroke-width="${sw}" stroke-linecap="round" />`;

    case "squares":
      return `<rect x="${ex - r}" y="${ey - r}" width="${r * 2}" height="${r * 2}" fill="${color}" />`;

    case "semicircles":
      return `<path d="M ${ex - r} ${ey} A ${r} ${r} 0 0 1 ${ex + r} ${ey}" fill="${color}" />`;

    case "diamonds": {
      const d = r;
      return `<polygon points="${ex},${ey - d} ${ex + d},${ey} ${ex},${ey + d} ${ex - d},${ey}" fill="${color}" />`;
    }
  }
}

/** Render eyes as SVG elements. Returns an SVG fragment string. */
export function buildEyes(
  params: EyeParams,
  size: number,
  color: string,
): string {
  const cx = size / 2;
  const cy = size / 2;
  const gap = params.spacing * size;
  const lx = cx - gap / 2;
  const rx = cx + gap / 2;
  const r = params.eyeSize * size;

  return [
    buildEye(params.leftStyle, lx, cy, r, color),
    buildEye(params.rightStyle, rx, cy, r, color),
  ].join("\n  ");
}
