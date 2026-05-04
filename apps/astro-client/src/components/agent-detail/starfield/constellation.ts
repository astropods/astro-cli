/**
 * Minimum spanning tree (Prim's algorithm) for constellation lines.
 *
 * Given a set of 2D positions, returns the edges of the MST as index pairs.
 * This produces a connected graph with N-1 edges that looks like a
 * constellation chart — every node connected, no redundant lines.
 */

import type { Position } from "../pods/pod-layout";

export interface Edge {
  from: number;
  to: number;
}

function distance(a: Position, b: Position): number {
  const dx = a.x - b.x;
  const dy = a.y - b.y;
  return Math.sqrt(dx * dx + dy * dy);
}

export function computeMST(positions: Position[]): Edge[] {
  const n = positions.length;
  if (n < 2) return [];

  const inTree = new Uint8Array(n);
  const minCost = new Float64Array(n).fill(Infinity);
  const minEdge = new Int32Array(n).fill(-1);
  const edges: Edge[] = [];

  // Start from node 0.
  minCost[0] = 0;

  for (let iter = 0; iter < n; iter++) {
    // Pick the cheapest node not yet in the tree.
    let u = -1;
    for (let i = 0; i < n; i++) {
      if (!inTree[i] && (u === -1 || minCost[i] < minCost[u])) {
        u = i;
      }
    }

    inTree[u] = 1;
    if (minEdge[u] !== -1) {
      edges.push({ from: minEdge[u], to: u });
    }

    // Update costs for neighbors.
    for (let v = 0; v < n; v++) {
      if (inTree[v]) continue;
      const d = distance(positions[u], positions[v]);
      if (d < minCost[v]) {
        minCost[v] = d;
        minEdge[v] = u;
      }
    }
  }

  return edges;
}
