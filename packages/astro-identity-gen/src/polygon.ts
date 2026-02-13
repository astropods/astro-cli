export type EdgeStyle = "flat" | "spikey" | "scalloped" | "inverse-scalloped";

export const edgeStyles: EdgeStyle[] = [
  "flat",
  "spikey",
  "scalloped",
  "inverse-scalloped",
];

export interface PolygonParams {
  /** Number of sides (3–8). */
  sides: number;
  /** Edge treatment style. */
  edgeStyle: EdgeStyle;
  /** Rotation in radians. */
  rotation: number;
  /** Radius as a fraction of half the viewbox (0.5–0.95). */
  radius: number;
  /** For spikey: inner radius ratio (0.3–0.7 of radius). */
  spikeDepth: number;
  /** For scalloped/inverse-scalloped: how far control points bow (0.2–0.5). */
  curveAmount: number;
}

interface Point {
  x: number;
  y: number;
}

/** Get the vertices of a regular polygon centered at (cx, cy). */
function regularVertices(
  sides: number,
  cx: number,
  cy: number,
  r: number,
  rotation: number,
): Point[] {
  const pts: Point[] = [];
  for (let i = 0; i < sides; i++) {
    const angle = rotation + (2 * Math.PI * i) / sides;
    pts.push({ x: cx + r * Math.cos(angle), y: cy + r * Math.sin(angle) });
  }
  return pts;
}

/** Build an SVG path string for a flat (straight-edged) polygon. */
function flatPath(vertices: Point[]): string {
  const [first, ...rest] = vertices;
  return (
    `M ${first.x} ${first.y} ` +
    rest.map((p) => `L ${p.x} ${p.y}`).join(" ") +
    " Z"
  );
}

/** Build an SVG path for a star / spikey polygon. */
function spikeyPath(
  sides: number,
  cx: number,
  cy: number,
  outerR: number,
  innerR: number,
  rotation: number,
): string {
  const pts: Point[] = [];
  for (let i = 0; i < sides; i++) {
    const outerAngle = rotation + (2 * Math.PI * i) / sides;
    pts.push({
      x: cx + outerR * Math.cos(outerAngle),
      y: cy + outerR * Math.sin(outerAngle),
    });
    const innerAngle = outerAngle + Math.PI / sides;
    pts.push({
      x: cx + innerR * Math.cos(innerAngle),
      y: cy + innerR * Math.sin(innerAngle),
    });
  }
  const [first, ...rest] = pts;
  return (
    `M ${first.x} ${first.y} ` +
    rest.map((p) => `L ${p.x} ${p.y}`).join(" ") +
    " Z"
  );
}

/** Build an SVG path with scalloped (outward-curved) or inverse-scalloped (inward-curved) edges. */
function scallopedPath(
  vertices: Point[],
  cx: number,
  cy: number,
  curveAmount: number,
  inward: boolean,
): string {
  const n = vertices.length;
  const parts: string[] = [`M ${vertices[0].x} ${vertices[0].y}`];

  for (let i = 0; i < n; i++) {
    const a = vertices[i];
    const b = vertices[(i + 1) % n];
    // Midpoint of the edge
    const mx = (a.x + b.x) / 2;
    const my = (a.y + b.y) / 2;
    // Direction from center to midpoint
    const dx = mx - cx;
    const dy = my - cy;
    const dist = Math.sqrt(dx * dx + dy * dy);
    const nx = dx / dist;
    const ny = dy / dist;
    // Push control point outward or inward
    const sign = inward ? -1 : 1;
    const offset = curveAmount * dist;
    const cpx = mx + sign * nx * offset;
    const cpy = my + sign * ny * offset;
    parts.push(`Q ${cpx} ${cpy} ${b.x} ${b.y}`);
  }

  parts.push("Z");
  return parts.join(" ");
}

/** Generate a full SVG path `d` attribute for the given polygon parameters. */
export function buildPolygonPath(params: PolygonParams, size: number): string {
  const cx = size / 2;
  const cy = size / 2;
  const r = params.radius * (size / 2);

  switch (params.edgeStyle) {
    case "flat": {
      const verts = regularVertices(params.sides, cx, cy, r, params.rotation);
      return flatPath(verts);
    }
    case "spikey": {
      const innerR = r * params.spikeDepth;
      return spikeyPath(params.sides, cx, cy, r, innerR, params.rotation);
    }
    case "scalloped": {
      const verts = regularVertices(params.sides, cx, cy, r, params.rotation);
      return scallopedPath(verts, cx, cy, params.curveAmount, false);
    }
    case "inverse-scalloped": {
      const verts = regularVertices(params.sides, cx, cy, r, params.rotation);
      return scallopedPath(verts, cx, cy, params.curveAmount, true);
    }
  }
}
