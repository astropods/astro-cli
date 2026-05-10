import type {
  ObservationLevel,
  ObservationType,
  TraceObservation,
} from "@/lib/api";

export interface ObservationNode extends TraceObservation {
  depth: number;
  /** True when this node is the last child of its parent. */
  isLast: boolean;
  /**
   * For each ancestor depth (root → parent), whether that ancestor was a
   * last child. Used to draw the vertical connector rails: a non-last
   * ancestor needs a continuing line, a last ancestor doesn't.
   */
  ancestorsLast: boolean[];
  children: ObservationNode[];
}

/**
 * Build a parent/child tree from a flat observation list. Roots are
 * observations whose parent_id is empty or references something outside the
 * provided list (orphans get promoted so we never silently drop nodes).
 *
 * Children are sorted by start_time so the tree reads chronologically.
 */
export function buildObservationTree(
  observations: TraceObservation[],
): ObservationNode[] {
  if (observations.length === 0) return [];

  const byId = new Map<string, ObservationNode>();
  for (const o of observations) {
    byId.set(o.id, {
      ...o,
      depth: 0,
      isLast: false,
      ancestorsLast: [],
      children: [],
    });
  }

  const roots: ObservationNode[] = [];
  for (const node of byId.values()) {
    const parent = node.parent_id ? byId.get(node.parent_id) : undefined;
    if (parent) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }

  const decorate = (
    nodes: ObservationNode[],
    depth: number,
    ancestorsLast: boolean[],
  ) => {
    nodes.sort(byStart);
    nodes.forEach((node, i) => {
      node.depth = depth;
      node.isLast = i === nodes.length - 1;
      node.ancestorsLast = ancestorsLast;
      decorate(node.children, depth + 1, [...ancestorsLast, node.isLast]);
    });
  };
  decorate(roots, 0, []);

  return roots;
}

function byStart(a: ObservationNode, b: ObservationNode): number {
  return new Date(a.start_time).getTime() - new Date(b.start_time).getTime();
}

/**
 * Flatten a tree into the visible row order, honoring per-node expansion.
 * Collapsed nodes hide their descendants.
 */
export function flattenTree(
  nodes: ObservationNode[],
  expanded: Set<string>,
): ObservationNode[] {
  const out: ObservationNode[] = [];
  const walk = (list: ObservationNode[]) => {
    for (const n of list) {
      out.push(n);
      if (n.children.length > 0 && expanded.has(n.id)) walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

export function collectIds(nodes: ObservationNode[]): string[] {
  const out: string[] = [];
  const walk = (list: ObservationNode[]) => {
    for (const n of list) {
      out.push(n.id);
      walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

const TYPE_LABEL: Record<ObservationType, string> = {
  span: "Span",
  generation: "Generation",
  event: "Event",
};

export function observationTypeLabel(type: ObservationType): string {
  return TYPE_LABEL[type] ?? type;
}

const LEVEL_TO_STATUS: Record<ObservationLevel, "ok" | "warn" | "error"> = {
  debug: "ok",
  default: "ok",
  warning: "warn",
  error: "error",
};

export function levelStatus(level?: ObservationLevel): "ok" | "warn" | "error" {
  return level ? LEVEL_TO_STATUS[level] ?? "ok" : "ok";
}

// ---------------------------------------------------------------------------
// Timing bounds
// ---------------------------------------------------------------------------

export interface TraceBounds {
  startMs: number;
  endMs: number;
  durationMs: number;
}

/**
 * Trace bounds are the earliest start and the latest end across all
 * observations. Falls back to start + latency_ms when end_time is missing.
 * Returns null if there isn't enough timing data to draw a waterfall.
 */
export function computeTraceBounds(
  observations: TraceObservation[],
): TraceBounds | null {
  let start = Number.POSITIVE_INFINITY;
  let end = Number.NEGATIVE_INFINITY;
  for (const o of observations) {
    const s = Date.parse(o.start_time);
    if (!Number.isFinite(s)) continue;
    if (s < start) start = s;
    const e = o.end_time
      ? Date.parse(o.end_time)
      : s + (o.latency_ms || 0);
    if (Number.isFinite(e) && e > end) end = e;
  }
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null;
  const durationMs = end - start;
  if (durationMs <= 0) return null;
  return { startMs: start, endMs: end, durationMs };
}

export interface NodeTimespan {
  /** % offset from the trace start, clamped to [0, 100]. */
  offsetPct: number;
  /** % width of the bar, clamped so offset+width ≤ 100. */
  widthPct: number;
  /** Share of the trace's total duration this node took, in [0, 1]. */
  share: number;
}

/**
 * Compute waterfall geometry for a node. Returns null when timing is
 * unusable (missing start, before-bounds, etc).
 */
export function nodeTimespan(
  node: TraceObservation,
  bounds: TraceBounds,
): NodeTimespan | null {
  const s = Date.parse(node.start_time);
  if (!Number.isFinite(s)) return null;
  const e = node.end_time
    ? Date.parse(node.end_time)
    : s + (node.latency_ms || 0);
  if (!Number.isFinite(e)) return null;

  const offsetPct = ((s - bounds.startMs) / bounds.durationMs) * 100;
  const rawWidthPct = ((e - s) / bounds.durationMs) * 100;

  const offset = Math.max(0, Math.min(100, offsetPct));
  // Floor at 0.5% so a millisecond-scale span is still visible.
  const width = Math.max(0.5, Math.min(100 - offset, rawWidthPct));
  const share = bounds.durationMs > 0 ? node.latency_ms / bounds.durationMs : 0;
  return { offsetPct: offset, widthPct: width, share: Math.max(0, Math.min(1, share)) };
}
