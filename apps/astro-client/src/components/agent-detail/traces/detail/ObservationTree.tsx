import { useMemo, useState } from "react";
import type { TraceObservation } from "@/lib/api";
import {
  buildObservationTree,
  collectIds,
  computeTraceBounds,
  flattenTree,
} from "./observation-utils";
import { ObservationTreeNode } from "./ObservationTreeNode";

export interface ObservationTreeProps {
  observations: TraceObservation[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  /** Show a per-row timing waterfall column (needs horizontal room). */
  showTimeline?: boolean;
}

export function ObservationTree({
  observations,
  selectedId,
  onSelect,
  showTimeline = false,
}: ObservationTreeProps) {
  const tree = useMemo(() => buildObservationTree(observations), [observations]);
  const bounds = useMemo(
    () => (showTimeline ? computeTraceBounds(observations) : null),
    [showTimeline, observations],
  );

  // Default to fully expanded so users see the structure on first open.
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set(collectIds(tree)),
  );

  const rows = useMemo(() => flattenTree(tree, expanded), [tree, expanded]);

  if (observations.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center px-4 py-8 text-body-sm text-muted-foreground">
        No observations recorded for this trace.
      </div>
    );
  }

  const toggle = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <div className="flex flex-col py-1">
      {rows.map((node) => (
        <ObservationTreeNode
          key={node.id}
          node={node}
          isSelected={node.id === selectedId}
          isExpanded={expanded.has(node.id)}
          onSelect={onSelect}
          onToggle={toggle}
          bounds={bounds}
        />
      ))}
    </div>
  );
}
