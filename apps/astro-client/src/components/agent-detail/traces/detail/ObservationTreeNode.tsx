import {
  ArrowLeftRight,
  ChevronDown,
  ChevronRight,
  Sparkles,
  Zap,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { ObservationType } from "@/lib/api";
import { formatCost, formatLatency } from "../trace-utils";
import {
  levelStatus,
  nodeTimespan,
  type NodeTimespan,
  type ObservationNode,
  type TraceBounds,
} from "./observation-utils";

const RAIL_WIDTH_PX = 14;
const TIMELINE_WIDTH_PX = 160;
// Vertical offset from the row top to the center of the name line. Matches
// `py-1.5` (6px) + half the line-height of the icon row (~14/2). Used to
// pin the waterfall track to the name baseline instead of the row middle.
const TIMELINE_Y_PX = 13;

const TYPE_ICON: Record<ObservationType, typeof Sparkles> = {
  generation: Sparkles,
  span: ArrowLeftRight,
  event: Zap,
};

const TYPE_ICON_COLOR: Record<ObservationType, string> = {
  generation: "var(--color-pink-500, var(--color-coral-600))",
  span: "var(--color-indigo-400)",
  event: "var(--color-yellow-600)",
};

export interface ObservationTreeNodeProps {
  node: ObservationNode;
  isSelected: boolean;
  isExpanded: boolean;
  onSelect: (id: string) => void;
  onToggle: (id: string) => void;
  /** When provided, render a timing waterfall column at the right. */
  bounds?: TraceBounds | null;
}

export function ObservationTreeNode({
  node,
  isSelected,
  isExpanded,
  onSelect,
  onToggle,
  bounds,
}: ObservationTreeNodeProps) {
  const Icon = TYPE_ICON[node.type] ?? ArrowLeftRight;
  const status = levelStatus(node.level);
  const hasChildren = node.children.length > 0;
  const errorTone = status === "error";

  const span = bounds ? nodeTimespan(node, bounds) : null;

  // Avoid rendering the model name twice when the observation name already
  // contains it (e.g. "chat claude-sonnet-4-20250514" + model "claude-...").
  const showModel =
    node.model && !node.name.toLowerCase().includes(node.model.toLowerCase());

  return (
    <div
      className={cn(
        "group relative flex w-full items-stretch transition-colors",
        isSelected ? "bg-primary/10" : "hover:bg-muted/30",
      )}
    >
      <Rails ancestorsLast={node.ancestorsLast} isLast={node.isLast} hasParent={node.depth > 0} />

      <button
        type="button"
        onClick={() => onSelect(node.id)}
        className="flex min-w-0 flex-1 flex-col items-start gap-0.5 py-1.5 pl-1 pr-2 text-left"
      >
        <div className="flex w-full min-w-0 items-center gap-1.5">
          <Icon
            className="size-3.5 shrink-0"
            style={{ color: errorTone ? "var(--error)" : TYPE_ICON_COLOR[node.type] }}
          />
          <span className="min-w-0 flex-1 truncate text-body-sm text-foreground">
            <span className="font-medium">
              {node.name || <em className="text-muted-foreground">unnamed</em>}
            </span>
            {showModel && (
              <span className="ml-1.5 font-mono text-mono-sm text-muted-foreground">
                {node.model}
              </span>
            )}
          </span>
        </div>
        <NodeStats node={node} errorTone={errorTone} />
      </button>

      {span && (
        <Waterfall span={span} errorTone={errorTone} />
      )}

      {hasChildren ? (
        <button
          type="button"
          onClick={() => onToggle(node.id)}
          aria-label={isExpanded ? "Collapse" : "Expand"}
          aria-expanded={isExpanded}
          className="flex shrink-0 items-start justify-center px-1 pt-1 text-muted-foreground hover:text-foreground"
        >
          {isExpanded ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </button>
      ) : (
        // Reserve the same column width so leaf-row waterfalls align with
        // parent-row waterfalls (which yield a chevron button to the right).
        <span className="shrink-0 px-1" aria-hidden>
          <span className="block size-4" />
        </span>
      )}
    </div>
  );
}

function NodeStats({
  node,
  errorTone,
}: {
  node: ObservationNode;
  errorTone: boolean;
}) {
  const usage = node.usage;
  const cost = node.cost && node.cost > 0 ? node.cost : undefined;
  const latency = node.latency_ms;

  if (latency <= 0 && !usage && !cost) return null;

  const valueClass = "font-mono text-mono-sm";
  const valueColor = errorTone ? "var(--error)" : undefined;
  const mutedClass = "font-mono text-mono-sm text-muted-foreground";

  return (
    <div className="flex flex-wrap items-center gap-x-2 pl-[20px]">
      {latency > 0 && (
        <span className={valueClass} style={{ color: valueColor }}>
          {formatLatency(latency, true)}
        </span>
      )}

      {usage && (usage.input > 0 || usage.output > 0) && (
        <span className={mutedClass}>
          {usage.input.toLocaleString()}
          <span className="mx-1 text-muted-foreground/60">→</span>
          {usage.output.toLocaleString()}
          {usage.total > 0 && (
            <span className="ml-1 text-muted-foreground/60">
              (Σ {usage.total.toLocaleString()})
            </span>
          )}
        </span>
      )}

      {cost && (
        <span className={valueClass} style={{ color: valueColor }}>
          Σ {formatCost(cost)}
        </span>
      )}
    </div>
  );
}

/**
 * A track that draws the node's interval relative to the whole trace.
 * Bars use a single muted color (indigo) for everything; errored
 * observations swap to coral so they actually stand out.
 */
function Waterfall({
  span,
  errorTone,
}: {
  span: NodeTimespan;
  errorTone: boolean;
}) {
  const fill = errorTone
    ? "var(--error)"
    : "var(--color-indigo-400)";

  return (
    <div
      className="relative shrink-0 self-stretch"
      style={{ width: TIMELINE_WIDTH_PX }}
      aria-hidden
    >
      {/* Track — thin guideline pinned to the name-line height */}
      <div
        className="absolute left-0 right-0 h-px bg-border/40"
        style={{ top: TIMELINE_Y_PX }}
      />
      {/* Bar — same y-anchor so all rows share a horizontal axis */}
      <div
        className="absolute h-[4px] -translate-y-1/2 rounded-full"
        style={{
          top: TIMELINE_Y_PX,
          left: `${span.offsetPct}%`,
          width: `${span.widthPct}%`,
          background: fill,
        }}
      />
    </div>
  );
}

/**
 * Vertical/elbow connectors that visually link a node to its parent and to
 * its siblings. We render one column per ancestor depth (continuing line if
 * that ancestor was *not* a last child) plus a final elbow column for the
 * current node.
 */
function Rails({
  ancestorsLast,
  isLast,
  hasParent,
}: {
  ancestorsLast: boolean[];
  isLast: boolean;
  hasParent: boolean;
}) {
  return (
    <div className="flex shrink-0">
      {ancestorsLast.map((wasLast, i) => (
        <div
          key={i}
          className="relative"
          style={{ width: RAIL_WIDTH_PX }}
        >
          {!wasLast && (
            <span
              className="absolute inset-y-0"
              style={{
                left: RAIL_WIDTH_PX / 2,
                borderLeft: "1px solid var(--color-border)",
              }}
            />
          )}
        </div>
      ))}
      {hasParent && (
        <div className="relative" style={{ width: RAIL_WIDTH_PX }}>
          {/* Top half of the vertical line for this node */}
          <span
            className="absolute"
            style={{
              top: 0,
              bottom: isLast ? "50%" : 0,
              left: RAIL_WIDTH_PX / 2,
              borderLeft: "1px solid var(--color-border)",
            }}
          />
          {/* Horizontal stub from line to node body */}
          <span
            className="absolute"
            style={{
              top: "50%",
              left: RAIL_WIDTH_PX / 2,
              right: 0,
              borderTop: "1px solid var(--color-border)",
            }}
          />
        </div>
      )}
    </div>
  );
}
