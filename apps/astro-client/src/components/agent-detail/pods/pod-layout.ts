/**
 * Force-directed pod layout.
 *
 * 1. Seeds initial positions using a golden-angle spiral (deterministic,
 *    close to a good solution so the simulation converges fast).
 * 2. Runs a force simulation where tiles repel on overlap and a gentle
 *    centering force keeps the cluster compact.
 * 3. Returns final positions as pixel offsets from the container center.
 */

// ---------------------------------------------------------------------------
// Golden-angle spiral seed
// ---------------------------------------------------------------------------

const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));
const SEED_ROTATION = 0.7;

function seedPositions(count: number, sizes: TileSize[]): Position[] {
  if (count === 0) return [];
  if (count === 1) return [{ x: 0, y: 0 }];

  const avgSize =
    sizes.reduce((s, t) => s + Math.max(t.width, t.height), 0) / sizes.length;
  const spacing = avgSize * 0.75;

  return Array.from({ length: count }, (_, i) => {
    const angle = SEED_ROTATION + i * GOLDEN_ANGLE;
    const radius = spacing * Math.sqrt(i + 0.5);
    return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius };
  });
}

// ---------------------------------------------------------------------------
// Force simulation
// ---------------------------------------------------------------------------

export interface TileSize {
  width: number;
  height: number;
}

export interface Position {
  x: number;
  y: number;
}

interface SimConfig {
  /** Iterations to run. */
  iterations: number;
  /** Pixels of padding between tiles. */
  padding: number;
  /** Strength of the centering pull (0–1). */
  centeringStrength: number;
  /** Velocity damping per step (0–1, lower = more damping). */
  damping: number;
  /** Strength of repulsion force. */
  repulsionStrength: number;
}

const DEFAULT_CONFIG: SimConfig = {
  iterations: 80,
  padding: 32,
  repulsionStrength: 0.4,
  centeringStrength: 0.02,
  damping: 0.8,
};

/**
 * Check overlap between two axis-aligned rects centered at their positions.
 * Returns a repulsion vector along the center-to-center direction scaled by
 * the overlap magnitude, or null if no overlap.
 */
function rectOverlap(
  ax: number,
  ay: number,
  aw: number,
  ah: number,
  bx: number,
  by: number,
  bw: number,
  bh: number,
  padding: number,
): { dx: number; dy: number } | null {
  const halfW = (aw + bw) / 2 + padding;
  const halfH = (ah + bh) / 2 + padding;
  const dx = ax - bx;
  const dy = ay - by;
  const overlapX = halfW - Math.abs(dx);
  const overlapY = halfH - Math.abs(dy);

  if (overlapX <= 0 || overlapY <= 0) return null;

  // Push along the center-to-center vector to preserve organic angles.
  const dist = Math.sqrt(dx * dx + dy * dy);
  const overlap = Math.min(overlapX, overlapY);

  if (dist < 0.01) {
    // Near-coincident — pick a deterministic diagonal direction.
    return { dx: overlap * 0.7, dy: overlap * 0.7 };
  }

  return {
    dx: (dx / dist) * overlap,
    dy: (dy / dist) * overlap,
  };
}

function runSimulation(
  positions: Position[],
  sizes: TileSize[],
  config: SimConfig,
): Position[] {
  const n = positions.length;
  if (n <= 1) return positions;

  // Clone so we don't mutate input.
  const pos = positions.map((p) => ({ ...p }));
  const vel = Array.from({ length: n }, () => ({ x: 0, y: 0 }));

  for (let iter = 0; iter < config.iterations; iter++) {
    // Pairwise repulsion on overlap.
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const overlap = rectOverlap(
          pos[i].x, pos[i].y, sizes[i].width, sizes[i].height,
          pos[j].x, pos[j].y, sizes[j].width, sizes[j].height,
          config.padding,
        );
        if (!overlap) continue;

        const fx = overlap.dx * config.repulsionStrength;
        const fy = overlap.dy * config.repulsionStrength;
        vel[i].x += fx;
        vel[i].y += fy;
        vel[j].x -= fx;
        vel[j].y -= fy;
      }
    }

    // Centering force — pull toward origin.
    for (let i = 0; i < n; i++) {
      vel[i].x -= pos[i].x * config.centeringStrength;
      vel[i].y -= pos[i].y * config.centeringStrength;
    }

    // Integrate and damp.
    for (let i = 0; i < n; i++) {
      pos[i].x += vel[i].x;
      pos[i].y += vel[i].y;
      vel[i].x *= config.damping;
      vel[i].y *= config.damping;
    }
  }

  return pos;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Compute non-overlapping positions for tiles of known sizes.
 * Positions are pixel offsets from the center of the container.
 */
export function computePodPositions(
  sizes: TileSize[],
  config?: Partial<SimConfig>,
): Position[] {
  const count = sizes.length;
  if (count === 0) return [];

  const cfg = { ...DEFAULT_CONFIG, ...config };
  const initial = seedPositions(count, sizes);
  return runSimulation(initial, sizes, cfg);
}
