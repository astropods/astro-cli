import { describe, it, expect } from "vitest";
import { computeColumnLayout, type LayoutTile, type Position } from "./pod-layout";
import type { Role } from "./classify";

const tile = (role: Role, width = 220, height = 120): LayoutTile => ({ role, size: { width, height } });

function overlaps(a: Position, as: LayoutTile, b: Position, bs: LayoutTile): boolean {
  const dx = Math.abs(a.x - b.x);
  const dy = Math.abs(a.y - b.y);
  return dx < (as.size.width + bs.size.width) / 2 && dy < (as.size.height + bs.size.height) / 2;
}

function assertNoOverlaps(positions: Position[], tiles: LayoutTile[]) {
  for (let i = 0; i < positions.length; i++) {
    for (let j = i + 1; j < positions.length; j++) {
      expect(overlaps(positions[i], tiles[i], positions[j], tiles[j])).toBe(false);
    }
  }
}

// A representative deployment: ingestion + two stores + agent + a model + collector.
function fullDeployment(): LayoutTile[] {
  return [
    tile("ingestion"),
    tile("knowledge"),
    tile("knowledge"),
    tile("agent"),
    tile("model"),
    tile("collector"),
  ];
}
const [ING, KN1, KN2, AGENT, MODEL, COLL] = [0, 1, 2, 3, 4, 5];

describe("computeColumnLayout", () => {
  it("returns no positions for zero tiles", () => {
    expect(computeColumnLayout([])).toEqual([]);
  });

  it("centers a single tile at the origin", () => {
    expect(computeColumnLayout([tile("agent")])).toEqual([{ x: 0, y: 0 }]);
  });

  it("centers the overall graph horizontally", () => {
    const tiles = fullDeployment();
    const p = computeColumnLayout(tiles);
    const minX = Math.min(...p.map((pos, i) => pos.x - tiles[i].size.width / 2));
    const maxX = Math.max(...p.map((pos, i) => pos.x + tiles[i].size.width / 2));
    expect((minX + maxX) / 2).toBeCloseTo(0, 5);
  });

  it("orders columns ingestion → knowledge → agent → others left to right", () => {
    const tiles = fullDeployment();
    const p = computeColumnLayout(tiles);
    expect(p[ING].x).toBeLessThan(p[KN1].x); // ingestion left of knowledge
    expect(p[KN1].x).toBeLessThan(p[AGENT].x); // knowledge left of agent
    expect(p[AGENT].x).toBeLessThan(p[MODEL].x); // model right of agent
    expect(p[AGENT].x).toBeLessThan(p[COLL].x); // collector right of agent
  });

  it("stacks a column on a shared x, centered vertically", () => {
    const tiles = fullDeployment();
    const p = computeColumnLayout(tiles);
    expect(p[KN1].x).toBe(p[KN2].x); // same column → same x
    // Two-tile knowledge column is centered on the vertical axis.
    expect((p[KN1].y + p[KN2].y) / 2).toBeCloseTo(0, 5);
    expect(p[KN1].y).not.toBe(p[KN2].y); // but separated vertically
  });

  it("packs without overlaps across shapes and configurations", () => {
    const configs: LayoutTile[][] = [
      [tile("agent"), tile("knowledge")],
      fullDeployment(),
      // Varied sizes so wide/tall tiles still clear each other.
      Array.from({ length: 8 }, (_, i) =>
        tile((["agent", "knowledge", "ingestion", "model", "integration", "collector"] as Role[])[i % 6],
          160 + ((i * 37) % 160), 90 + ((i * 23) % 120)),
      ),
      // A tall single column of stores.
      [tile("agent"), ...Array.from({ length: 6 }, () => tile("knowledge"))],
    ];
    for (const tiles of configs) {
      assertNoOverlaps(computeColumnLayout(tiles), tiles);
    }
  });

  it("is deterministic — identical input yields identical output", () => {
    const tiles = fullDeployment();
    expect(computeColumnLayout(tiles)).toEqual(computeColumnLayout(tiles));
  });

  it("returns one finite position per tile", () => {
    const tiles = fullDeployment();
    const positions = computeColumnLayout(tiles);
    expect(positions).toHaveLength(tiles.length);
    for (const p of positions) {
      expect(Number.isFinite(p.x)).toBe(true);
      expect(Number.isFinite(p.y)).toBe(true);
    }
  });
});
