/**
 * Canvas star field with a slow camera motion. Stars carry a 3D position
 * (x0, y0, z) and project to screen as (cx + x0/z, cy + y0/z) with size
 * baseR/z. The motion direction is pluggable via the `direction` prop:
 *
 * - "forward" / "reverse": camera moves through z. Stars warp outward
 *   (forward) or converge inward (reverse). z-based fade hides the spawn
 *   and the floor recycle.
 * - "left" / "right" / "up" / "down": camera drifts laterally. Stars'
 *   apparent motion is parallax — close stars (low z) sweep across fast,
 *   far stars (high z) drift slowly. Stars enter just off the trailing
 *   edge and exit at the leading edge; no fade needed.
 *
 * Common path: every star uses the same per-frame update (x0/y0/z += v*dt),
 * the same projection, the same out-of-bounds recycle, and the same sprite
 * blit. Only the velocity vector, the spawn-edge logic, and the alpha-vs-z
 * curve differ per direction; those live in DirectionStrategy.
 */

import { useEffect, useRef } from "react";
import { usePrefersReducedMotion } from "@/hooks/use-prefers-reduced-motion";
import { makeStarSprite, SPRITE_DRAW_SCALE } from "./star-sprite";
import { makeCloudSprites } from "./cloud-sprite";

function mulberry32(seed: number) {
  return () => {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const SEED = 42;
/** Stars per CSS pixel of viewport area. Calibrated so a 1920×1080 viewport
 *  gets ~1000 stars (the previously hand-tuned count). */
const STAR_DENSITY = 0.0007;
const MIN_STARS = 300;
const MAX_STARS = 6000;
const SIZE_RANGE: [number, number] = [0.7, 1.5];
const OPACITY_RANGE: [number, number] = [0.15, 0.8];
/** Inclusive z bounds for the warp lifetime. Out-of-range stars get recycled. */
const Z_NEAR = 0.3;
const Z_FAR = 1.0;
/** Base motion rate. For zoom: z-units per second. For lateral: scaled by
 *  cx/cy so the apparent traversal time is constant across viewport sizes. */
const SPEED = 0.003;
/** Recycle spawns inside this fraction of viewport, centered (zoom modes). */
const SPAWN_REGION = 0.95;
/** z-distance from spawn before fade-in alpha hits 1 (zoom modes). */
const FADE_IN_DEPTH = 0.25;
/** z-distance before recycle where fade-out alpha starts dropping (zoom modes). */
const FADE_OUT_DEPTH = 0.1;
/** Pixels past the viewport edge where lateral spawns originate, so stars
 *  drift in naturally rather than popping at the boundary. */
const SPAWN_BUFFER_PX = 4;

const DEFAULT_STAR_COLOR = "#ffffff";

const CLOUD_COUNT = 6;
/** Cloud size as fraction of viewport diagonal. */
const CLOUD_SIZE_RANGE: [number, number] = [0.25, 0.55];
const CLOUD_OPACITY_RANGE: [number, number] = [0.02, 0.06];

interface Star {
  x0: number;
  y0: number;
  z: number;
  baseR: number;
  baseOpacity: number;
}

interface Cloud {
  x0: number;
  y0: number;
  z: number;
  baseR: number;
  baseOpacity: number;
  spriteIdx: number;
}

function toCloud(star: Star, rand: () => number, numSprites: number): Cloud {
  return {
    x0: star.x0,
    y0: star.y0,
    z: star.z,
    baseR: CLOUD_SIZE_RANGE[0] + rand() * (CLOUD_SIZE_RANGE[1] - CLOUD_SIZE_RANGE[0]),
    baseOpacity: CLOUD_OPACITY_RANGE[0] + rand() * (CLOUD_OPACITY_RANGE[1] - CLOUD_OPACITY_RANGE[0]),
    spriteIdx: Math.floor(rand() * numSprites),
  };
}

/** Spawn a cloud at the recycle edge, offset far enough that it starts fully offscreen. */
function recycleCloud(
  spawnRecycled: (rand: () => number) => Star,
  rand: () => number,
  numSprites: number,
  diag: number,
  vx: number,
  vy: number,
): Cloud {
  const cloud = toCloud(spawnRecycled(rand), rand, numSprites);
  const halfDraw = cloud.baseR * diag / 2;
  // Push the cloud further behind the spawn edge so it's fully offscreen
  if (vx > 0) cloud.x0 -= halfDraw * cloud.z;
  else if (vx < 0) cloud.x0 += halfDraw * cloud.z;
  if (vy > 0) cloud.y0 -= halfDraw * cloud.z;
  else if (vy < 0) cloud.y0 += halfDraw * cloud.z;
  return cloud;
}

type Direction = "forward" | "reverse" | "left" | "right" | "up" | "down";
type LateralEdge = "left" | "right" | "top" | "bottom";

/** Per-direction configuration consumed by the hot loop. Built once per
 *  resize, captures viewport dimensions via closure. */
interface DirectionStrategy {
  vx: number;
  vy: number;
  vz: number;
  /** Produce a recycled-star at the appropriate entry point for this direction. */
  spawnRecycled: (rand: () => number) => Star;
  /** Alpha multiplier from a star's z. Zoom modes do z-based fade-in/out;
   *  lateral modes return 1 (entry/exit are masked by the viewport edge). */
  alphaForZ: (z: number) => number;
}

function smoothstep(edge0: number, edge1: number, x: number) {
  const t = Math.max(0, Math.min(1, (x - edge0) / (edge1 - edge0)));
  return t * t * (3 - 2 * t);
}

function rollBaseProps(rand: () => number) {
  return {
    baseR: SIZE_RANGE[0] + rand() * (SIZE_RANGE[1] - SIZE_RANGE[0]),
    baseOpacity: OPACITY_RANGE[0] + rand() * (OPACITY_RANGE[1] - OPACITY_RANGE[0]),
  };
}

function targetStarCount(w: number, h: number): number {
  return Math.max(MIN_STARS, Math.min(MAX_STARS, Math.round(STAR_DENSITY * w * h)));
}

/** Spawn within a centered fraction of the viewport at a given depth. Used
 *  for zoom-mode recycles (spread=SPAWN_REGION at the spawn z) and the initial
 *  fill (spread=1 at random z). */
function spawnInBox(
  rand: () => number,
  cx: number,
  cy: number,
  opts: { z: number; spread: number },
): Star {
  return {
    x0: cx * (2 * rand() - 1) * opts.spread * opts.z,
    y0: cy * (2 * rand() - 1) * opts.spread * opts.z,
    z: opts.z,
    ...rollBaseProps(rand),
  };
}

/** Spawn just off the given viewport edge with random depth and a random
 *  position along the perpendicular axis. Used for lateral-mode recycles. */
function spawnAtEdge(
  rand: () => number,
  cx: number,
  cy: number,
  edge: LateralEdge,
): Star {
  const z = Z_NEAR + rand() * (Z_FAR - Z_NEAR);
  let screenX = 0;
  let screenY = 0;
  switch (edge) {
    case "right":
      screenX = 2 * cx + SPAWN_BUFFER_PX;
      screenY = rand() * 2 * cy;
      break;
    case "left":
      screenX = -SPAWN_BUFFER_PX;
      screenY = rand() * 2 * cy;
      break;
    case "bottom":
      screenX = rand() * 2 * cx;
      screenY = 2 * cy + SPAWN_BUFFER_PX;
      break;
    case "top":
      screenX = rand() * 2 * cx;
      screenY = -SPAWN_BUFFER_PX;
      break;
  }
  return {
    x0: (screenX - cx) * z,
    y0: (screenY - cy) * z,
    z,
    ...rollBaseProps(rand),
  };
}

/** "This star has always been here" opts — random z, full-viewport spread.
 *  Used for the initial frame on every resize. */
function freshFillOpts(rand: () => number) {
  return { z: Z_NEAR + rand() * (Z_FAR - Z_NEAR), spread: 1 };
}

function buildStrategy(
  direction: Direction,
  cx: number,
  cy: number,
  speed: number,
): DirectionStrategy {
  switch (direction) {
    case "forward": {
      const fadeInEnd = Z_FAR - FADE_IN_DEPTH;
      const fadeOutStart = Z_NEAR + FADE_OUT_DEPTH;
      return {
        vx: 0,
        vy: 0,
        vz: -speed,
        spawnRecycled: (rand) =>
          spawnInBox(rand, cx, cy, { z: Z_FAR, spread: SPAWN_REGION }),
        alphaForZ: (z) =>
          smoothstep(Z_FAR, fadeInEnd, z) * smoothstep(Z_NEAR, fadeOutStart, z),
      };
    }
    case "reverse": {
      const fadeInEnd = Z_NEAR + FADE_IN_DEPTH;
      const fadeOutStart = Z_FAR - FADE_OUT_DEPTH;
      return {
        vx: 0,
        vy: 0,
        vz: speed,
        spawnRecycled: (rand) =>
          spawnInBox(rand, cx, cy, { z: Z_NEAR, spread: SPAWN_REGION }),
        alphaForZ: (z) =>
          smoothstep(Z_NEAR, fadeInEnd, z) * smoothstep(Z_FAR, fadeOutStart, z),
      };
    }
    case "left":
      return {
        vx: -speed * cx,
        vy: 0,
        vz: 0,
        spawnRecycled: (rand) => spawnAtEdge(rand, cx, cy, "right"),
        alphaForZ: () => 1,
      };
    case "right":
      return {
        vx: speed * cx,
        vy: 0,
        vz: 0,
        spawnRecycled: (rand) => spawnAtEdge(rand, cx, cy, "left"),
        alphaForZ: () => 1,
      };
    case "up":
      return {
        vx: 0,
        vy: -speed * cy,
        vz: 0,
        spawnRecycled: (rand) => spawnAtEdge(rand, cx, cy, "bottom"),
        alphaForZ: () => 1,
      };
    case "down":
      return {
        vx: 0,
        vy: speed * cy,
        vz: 0,
        spawnRecycled: (rand) => spawnAtEdge(rand, cx, cy, "top"),
        alphaForZ: () => 1,
      };
  }
}

interface StarFieldProps {
  /** Optional CSS background (color or gradient). Falls back to theme default. */
  backgroundColor?: string;
  /** Optional star color (any canvas-compatible CSS color). */
  starColor?: string;
  /** Optional cloud/nebula tint color. When set, subtle noise-based clouds
   *  drift with parallax behind the stars. */
  cloudColor?: string;
  /** Opacity multiplier for stars. Defaults to 1. */
  starOpacity?: number;
  /** Density multiplier for stars. Defaults to 1. */
  starDensity?: number;
  /** Opacity multiplier for clouds. Defaults to 1. */
  cloudOpacity?: number;
  /** Camera motion direction. "forward"/"reverse" zoom in/out; "left"/"right"/
   *  "up"/"down" produce a parallax drift. Defaults to "forward". */
  direction?: Direction;
  /** Motion speed multiplier. 1 = default tuned speed; 0.5 = half speed; etc. */
  speed?: number;
  /** RNG seed for star/cloud placement. Defaults to 42. */
  seed?: number;
}

export function StarField({
  backgroundColor,
  starColor,
  cloudColor,
  starOpacity = 1,
  starDensity = 1,
  cloudOpacity = 1,
  direction = "forward",
  speed = 1,
  seed: seedProp = SEED,
}: StarFieldProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const reduced = usePrefersReducedMotion();
  const effectiveColor = starColor ?? DEFAULT_STAR_COLOR;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const sprite = makeStarSprite(effectiveColor, starOpacity);
    const cloudSprites = cloudColor ? makeCloudSprites(cloudColor) : [];
    const recycleRand = mulberry32(seedProp + 1);
    const cloudRand = mulberry32(seedProp + 2);
    const dpr = window.devicePixelRatio || 1;
    const effectiveSpeed = SPEED * speed;
    let stars: Star[] = [];
    let clouds: Cloud[] = [];
    let strategy: DirectionStrategy = buildStrategy(direction, 1, 1, effectiveSpeed);
    let w = 0, h = 0, cx = 0, cy = 0;
    let rafId = 0;
    let lastT = performance.now();

    const ro = new ResizeObserver(([entry]) => {
      const rect = entry.contentRect;
      w = Math.max(1, Math.round(rect.width));
      h = Math.max(1, Math.round(rect.height));
      cx = w / 2;
      cy = h / 2;
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

      // Lateral velocity scales with viewport halves, so rebuild per resize.
      strategy = buildStrategy(direction, cx, cy, effectiveSpeed);

      // Full re-init: clean uniform distribution sized to the current viewport.
      const initRand = mulberry32(seedProp);
      const target = targetStarCount(w * starDensity, h);
      stars = Array.from({ length: target }, () =>
        spawnInBox(initRand, cx, cy, freshFillOpts(initRand)),
      );

      if (cloudSprites.length > 0) {
        const cloudInitRand = mulberry32(seedProp + 99);
        clouds = Array.from({ length: CLOUD_COUNT }, () => {
          // Spread clouds across the full viewport on initial render
          const star = spawnInBox(cloudInitRand, cx, cy, freshFillOpts(cloudInitRand));
          return toCloud(star, cloudInitRand, cloudSprites.length);
        });
      }

      if (reduced) drawFrame(0);
    });
    ro.observe(canvas);

    const drawFrame = (dtMs: number) => {
      if (!w || !h) return;
      ctx.clearRect(0, 0, w, h);
      const dt = dtMs / 1000;
      const { vx, vy, vz, spawnRecycled, alphaForZ } = strategy;

      // Draw clouds behind stars
      const diag = Math.sqrt(w * w + h * h);
      for (let i = 0; i < clouds.length; i++) {
        const c = clouds[i];
        c.x0 += vx * dt;
        c.y0 += vy * dt;
        c.z += vz * dt;

        if (c.z <= Z_NEAR || c.z >= Z_FAR) {
          clouds[i] = recycleCloud(spawnRecycled, cloudRand, cloudSprites.length, diag, vx, vy);
          continue;
        }

        const x = cx + c.x0 / c.z;
        const y = cy + c.y0 / c.z;
        const drawSize = c.baseR * diag;
        const halfDraw = drawSize / 2;

        if (
          x + halfDraw < 0 ||
          x - halfDraw > w ||
          y + halfDraw < 0 ||
          y - halfDraw > h
        ) {
          clouds[i] = recycleCloud(spawnRecycled, cloudRand, cloudSprites.length, diag, vx, vy);
          continue;
        }

        const alpha = c.baseOpacity * alphaForZ(c.z) * cloudOpacity;
        if (alpha <= 0) continue;

        ctx.globalAlpha = alpha;
        ctx.drawImage(cloudSprites[c.spriteIdx], x - halfDraw, y - halfDraw, drawSize, drawSize);
      }

      for (let i = 0; i < stars.length; i++) {
        const s = stars[i];
        s.x0 += vx * dt;
        s.y0 += vy * dt;
        s.z += vz * dt;

        // z out of range (zoom modes) — fade-out window above brings alpha
        // to 0 before this fires.
        if (s.z <= Z_NEAR || s.z >= Z_FAR) {
          stars[i] = spawnRecycled(recycleRand);
          continue;
        }

        const x = cx + s.x0 / s.z;
        const y = cy + s.y0 / s.z;
        const r = s.baseR / s.z;
        const drawSize = r / SPRITE_DRAW_SCALE;
        const halfDraw = drawSize / 2;

        // Screen exit — primary recycle path for lateral; rare for zoom.
        if (
          x + halfDraw < 0 ||
          x - halfDraw > w ||
          y + halfDraw < 0 ||
          y - halfDraw > h
        ) {
          stars[i] = spawnRecycled(recycleRand);
          continue;
        }

        const alpha = Math.min(1, s.baseOpacity * alphaForZ(s.z) * starOpacity);
        if (alpha <= 0) continue;

        ctx.globalAlpha = alpha;
        ctx.drawImage(sprite, x - halfDraw, y - halfDraw, drawSize, drawSize);
      }
      ctx.globalAlpha = 1;
    };

    const tick = (now: number) => {
      // Cap dt so a long pause doesn't warp the field forward in one jump.
      const dt = Math.min(now - lastT, 100);
      lastT = now;
      drawFrame(dt);
      rafId = requestAnimationFrame(tick);
    };

    const start = () => {
      lastT = performance.now();
      rafId = requestAnimationFrame(tick);
    };
    const stop = () => {
      cancelAnimationFrame(rafId);
      rafId = 0;
    };

    const onVisibility = () => {
      if (reduced) return;
      if (document.hidden) stop();
      else start();
    };
    document.addEventListener("visibilitychange", onVisibility);

    if (!reduced && !document.hidden) start();

    return () => {
      stop();
      ro.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [effectiveColor, starOpacity, starDensity, cloudColor, cloudOpacity, reduced, direction, speed, seedProp]);

  return (
    <div
      className="pointer-events-none absolute inset-0 bg-surface dark:bg-teal-950"
      style={backgroundColor ? { background: backgroundColor } : undefined}
    >
      <canvas ref={canvasRef} aria-hidden className="h-full w-full" />
    </div>
  );
}
