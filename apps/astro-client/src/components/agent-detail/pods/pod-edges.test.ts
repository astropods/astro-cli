import { describe, it, expect } from "vitest";
import { computeRelationshipEdges } from "./pod-edges";
import type { Role } from "./classify";

// Normalize to unordered "from-to" keys for order-independent assertions.
function edgeSet(roles: Role[]): Set<string> {
  return new Set(
    computeRelationshipEdges(roles).map((e) => {
      const [a, b] = [e.from, e.to].sort((x, y) => x - y);
      return `${a}-${b}`;
    }),
  );
}

describe("computeRelationshipEdges", () => {
  it("connects every direct dependency to the agent hub", () => {
    // 0 agent, 1 knowledge, 2 model, 3 integration, 4 collector
    const roles: Role[] = ["agent", "knowledge", "model", "integration", "collector"];
    expect(edgeSet(roles)).toEqual(new Set(["0-1", "0-2", "0-3", "0-4"]));
  });

  it("fans each ingestion tile to every knowledge tile", () => {
    // 0 agent, 1 knowledge, 2 knowledge, 3 ingestion
    const roles: Role[] = ["agent", "knowledge", "knowledge", "ingestion"];
    const edges = edgeSet(roles);
    expect(edges.has("1-3")).toBe(true); // ingestion → knowledge 1
    expect(edges.has("2-3")).toBe(true); // ingestion → knowledge 2
    expect(edges.has("0-1")).toBe(true); // agent → knowledge 1
    expect(edges.has("0-2")).toBe(true); // agent → knowledge 2
    // Ingestion is NOT wired straight to the agent when knowledge exists.
    expect(edges.has("0-3")).toBe(false);
  });

  it("falls back to the agent when ingestion has no knowledge to feed", () => {
    const roles: Role[] = ["agent", "ingestion"];
    expect(edgeSet(roles)).toEqual(new Set(["0-1"]));
  });

  it("never emits a self-edge", () => {
    const roles: Role[] = ["agent", "knowledge", "ingestion", "model"];
    for (const e of computeRelationshipEdges(roles)) {
      expect(e.from).not.toBe(e.to);
    }
  });
});
