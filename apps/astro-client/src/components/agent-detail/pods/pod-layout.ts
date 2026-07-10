/**
 * Deterministic pod-graph layout. Tiles are grouped into role columns
 * (ingestion | knowledge | agent | others), each stacked vertically, with the
 * whole row centered on the origin. Positions are a pure function of role and
 * measured size — same input, same layout — so a resize only shifts its own
 * column. Offsets are pixels from the container center.
 */

import { roleRank, type Role } from "./classify";

export interface TileSize {
  width: number;
  height: number;
}

export interface Position {
  x: number;
  y: number;
}

export interface LayoutTile {
  role: Role;
  size: TileSize;
}

const COLUMN_GAP = 28;
const ROW_GAP = 16;

type ColumnKey = "ingestion" | "knowledge" | "agent" | "others";

const COLUMN_ORDER: ColumnKey[] = ["ingestion", "knowledge", "agent", "others"];

function columnOf(role: Role): ColumnKey {
  if (role === "ingestion") return "ingestion";
  if (role === "knowledge") return "knowledge";
  if (role === "agent") return "agent";
  return "others";
}

function byRole(tiles: LayoutTile[]) {
  return (a: number, b: number) => roleRank(tiles[a].role) - roleRank(tiles[b].role) || a - b;
}

/** Stack `indices` into a vertical run at `cx`, centered on y = 0. */
function stackColumn(indices: number[], tiles: LayoutTile[], cx: number, out: Position[]): void {
  const height = indices.reduce((s, i) => s + tiles[i].size.height, 0) + (indices.length - 1) * ROW_GAP;
  let y = -height / 2;
  for (const i of indices) {
    out[i] = { x: cx, y: y + tiles[i].size.height / 2 };
    y += tiles[i].size.height + ROW_GAP;
  }
}

/** Role columns laid left to right, the whole row centered on the origin. */
export function computeColumnLayout(tiles: LayoutTile[]): Position[] {
  const columns = new Map<ColumnKey, number[]>();
  tiles.forEach((tile, i) => {
    const key = columnOf(tile.role);
    const col = columns.get(key);
    if (col) col.push(i);
    else columns.set(key, [i]);
  });
  // Only the mixed "others" column needs sorting; single-role columns keep input order.
  columns.get("others")?.sort(byRole(tiles));

  const present = COLUMN_ORDER.filter((k) => columns.has(k));
  const widths = present.map((k) => Math.max(...columns.get(k)!.map((i) => tiles[i].size.width)));
  const totalWidth = widths.reduce((s, w) => s + w, 0) + (present.length - 1) * COLUMN_GAP;

  const positions: Position[] = new Array(tiles.length);
  let left = -totalWidth / 2;
  present.forEach((key, k) => {
    stackColumn(columns.get(key)!, tiles, left + widths[k] / 2, positions);
    left += widths[k] + COLUMN_GAP;
  });
  return positions;
}
