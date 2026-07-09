/**
 * Relationship edges for the pod graph, from the workloads' wiring: the agent
 * is the hub, connected to every knowledge store, model, tool, and the
 * collector; each ingestion tile connects to every knowledge store it
 * populates. Returned as index pairs into `roles`.
 */

import type { Role } from "./classify";

export interface Edge {
  from: number;
  to: number;
}

export function computeRelationshipEdges(roles: Role[]): Edge[] {
  const edges: Edge[] = [];
  const agent = roles.indexOf("agent");
  const knowledge = roles.flatMap((r, i) => (r === "knowledge" ? [i] : []));
  // With no agent, spokes fall back to the first tile so nothing floats free.
  const hub = agent >= 0 ? agent : 0;

  roles.forEach((role, i) => {
    if (i === hub) return;
    if (role === "ingestion") {
      if (knowledge.length > 0) {
        for (const k of knowledge) edges.push({ from: i, to: k });
      } else {
        edges.push({ from: hub, to: i });
      }
      return;
    }
    edges.push({ from: hub, to: i });
  });

  return edges;
}
