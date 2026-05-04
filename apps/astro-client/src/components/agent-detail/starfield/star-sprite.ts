/**
 * Pre-renders a glowing star sprite: a small bright core surrounded by a wide
 * soft halo. The warp loop blits this via drawImage instead of computing a
 * Gaussian blur per frame.
 *
 * Two constants control the look independently:
 * - SPRITE_DRAW_SCALE sizes the whole sprite (drawSize = r / SPRITE_DRAW_SCALE),
 *   so it controls the glow's footprint.
 * - SPRITE_BRIGHT_STOP is where the gradient's solid alpha ends, controlling
 *   the visible bright-core size within that footprint.
 */

/** Sprite size in CSS px; drawImage scales from this. */
export const SPRITE_SIZE = 32;
/** Scale relating logical star radius `r` to drawn sprite size:
 *  drawSize = r / SPRITE_DRAW_SCALE. Smaller = bigger glow footprint per r. */
export const SPRITE_DRAW_SCALE = 0.11;
/** Gradient stop (fraction of sprite radius) where the solid bright core
 *  ends. Smaller than SPRITE_DRAW_SCALE = visible core narrower than the
 *  drawSize would imply. */
const SPRITE_BRIGHT_STOP = 0.07;

export function makeStarSprite(color: string, opacityMultiplier = 1): HTMLCanvasElement {
  const c = document.createElement("canvas");
  c.width = SPRITE_SIZE;
  c.height = SPRITE_SIZE;
  const ctx = c.getContext("2d")!;
  const half = SPRITE_SIZE / 2;

  // 1. Fill with the star color (full alpha).
  ctx.fillStyle = color;
  ctx.fillRect(0, 0, SPRITE_SIZE, SPRITE_SIZE);

  // 2. Apply a multi-stop alpha falloff via destination-in. The mask's RGB
  //    is ignored — only its alpha is used — so this works for any color
  //    format the caller passes (hex, rgb, oklch).
  const coreAlpha = Math.min(1, 0.55 * opacityMultiplier);
  const mask = ctx.createRadialGradient(half, half, 0, half, half, half);
  mask.addColorStop(0.0, `rgba(0,0,0,${coreAlpha})`);
  mask.addColorStop(SPRITE_BRIGHT_STOP, `rgba(0,0,0,${coreAlpha})`);
  mask.addColorStop(SPRITE_BRIGHT_STOP + 0.01, "rgba(0,0,0,0)");
  mask.addColorStop(1.0, "rgba(0,0,0,0)");

  ctx.globalCompositeOperation = "destination-in";
  ctx.fillStyle = mask;
  ctx.fillRect(0, 0, SPRITE_SIZE, SPRITE_SIZE);

  return c;
}
