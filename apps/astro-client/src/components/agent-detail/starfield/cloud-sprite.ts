/**
 * Noise-based cloud sprites for the nebula/galaxy parallax effect.
 *
 * Each sprite samples multi-octave 2D simplex noise to produce organic,
 * structured cloud shapes. A radial feather eliminates hard square edges.
 * Multiple variants are generated with different seeds/frequencies so
 * scattered clouds look distinct from each other.
 */

export const CLOUD_SPRITE_SIZE = 512;
export const CLOUD_DRAW_SCALE = 1.0;

// ── 2D Simplex noise ─────────────────────────────────────────────────

const GRAD2: readonly [number, number][] = [
  [1, 1], [-1, 1], [1, -1], [-1, -1],
  [1, 0], [-1, 0], [0, 1], [0, -1],
];

function createNoise2D(seed: number): (x: number, y: number) => number {
  const perm = new Uint8Array(512);
  const p = new Uint8Array(256);
  for (let i = 0; i < 256; i++) p[i] = i;
  let s = (seed | 0) || 1;
  for (let i = 255; i > 0; i--) {
    s = Math.imul(s, 16807) % 2147483647;
    const j = s % (i + 1);
    const tmp = p[i];
    p[i] = p[j];
    p[j] = tmp;
  }
  for (let i = 0; i < 512; i++) perm[i] = p[i & 255];

  const F2 = 0.5 * (Math.sqrt(3) - 1);
  const G2 = (3 - Math.sqrt(3)) / 6;

  return (x: number, y: number): number => {
    const sk = (x + y) * F2;
    const i = Math.floor(x + sk);
    const j = Math.floor(y + sk);
    const t = (i + j) * G2;
    const x0 = x - (i - t);
    const y0 = y - (j - t);

    const i1 = x0 > y0 ? 1 : 0;
    const j1 = 1 - i1;

    const x1 = x0 - i1 + G2;
    const y1 = y0 - j1 + G2;
    const x2 = x0 - 1 + 2 * G2;
    const y2 = y0 - 1 + 2 * G2;

    const ii = i & 255;
    const jj = j & 255;

    let n0 = 0;
    let t0 = 0.5 - x0 * x0 - y0 * y0;
    if (t0 > 0) {
      const g = GRAD2[perm[ii + perm[jj]] & 7];
      t0 *= t0;
      n0 = t0 * t0 * (g[0] * x0 + g[1] * y0);
    }

    let n1 = 0;
    let t1 = 0.5 - x1 * x1 - y1 * y1;
    if (t1 > 0) {
      const g = GRAD2[perm[ii + i1 + perm[jj + j1]] & 7];
      t1 *= t1;
      n1 = t1 * t1 * (g[0] * x1 + g[1] * y1);
    }

    let n2 = 0;
    let t2 = 0.5 - x2 * x2 - y2 * y2;
    if (t2 > 0) {
      const g = GRAD2[perm[ii + 1 + perm[jj + 1]] & 7];
      t2 *= t2;
      n2 = t2 * t2 * (g[0] * x2 + g[1] * y2);
    }

    return 70 * (n0 + n1 + n2);
  };
}

// ── Sprite generation ─────────────────────────────────────────────────

const NUM_VARIANTS = 4;

/** Resolves any CSS color to [r, g, b] via a 1×1 canvas paint + readback. */
function resolveColor(color: string): [number, number, number] {
  const c = document.createElement("canvas");
  c.width = 1;
  c.height = 1;
  const ctx = c.getContext("2d")!;
  ctx.fillStyle = color;
  ctx.fillRect(0, 0, 1, 1);
  const d = ctx.getImageData(0, 0, 1, 1).data;
  return [d[0], d[1], d[2]];
}

/**
 * Generates unique cloud sprites with noise-driven alpha and radial feather.
 * Color is flat (the passed tint); only the alpha channel varies.
 */
export function makeCloudSprites(color: string): HTMLCanvasElement[] {
  const [r, g, b] = resolveColor(color);
  const sprites: HTMLCanvasElement[] = [];

  for (let v = 0; v < NUM_VARIANTS; v++) {
    const size = CLOUD_SPRITE_SIZE;
    const c = document.createElement("canvas");
    c.width = size;
    c.height = size;
    const ctx = c.getContext("2d")!;
    const half = size / 2;

    const noise = createNoise2D(v * 7 + 13);
    const imgData = ctx.createImageData(size, size);
    const data = imgData.data;

    const baseFreq = 0.0025 + v * 0.00075;
    const freq2 = baseFreq * 2.2;
    const freq3 = baseFreq * 4.5;

    for (let py = 0; py < size; py++) {
      for (let px = 0; px < size; px++) {
        // Multi-octave noise for organic structure
        let n =
          noise(px * baseFreq, py * baseFreq) * 0.55 +
          noise(px * freq2 + 50, py * freq2 + 50) * 0.3 +
          noise(px * freq3 + 100, py * freq3 + 100) * 0.15;

        // Normalize [-1, 1] → [0, 1]
        n = (n + 1) * 0.5;

        // Threshold + contrast for defined regions
        n = Math.max(0, n - 0.35) / 0.65;
        n = Math.min(1, n * n * 3);

        // Radial feather: full density within 30% radius, fades to 0 by edge
        const dx = (px - half) / half;
        const dy = (py - half) / half;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const feather = 1 - Math.max(0, Math.min(1, (dist - 0.3) / 0.7));
        const smooth = feather * feather * (3 - 2 * feather);

        const alpha = n * smooth;

        const idx = (py * size + px) * 4;
        data[idx] = r;
        data[idx + 1] = g;
        data[idx + 2] = b;
        data[idx + 3] = Math.round(alpha * 255);
      }
    }

    ctx.putImageData(imgData, 0, 0);
    sprites.push(c);
  }

  return sprites;
}
