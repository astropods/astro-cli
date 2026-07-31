import { useMemo, useState } from "react";
import type { AgentDeploymentSummary } from "@/lib/api";

export type SortOption = "recent" | "name" | "requests";

export function useAgentFilters(
  deployments: AgentDeploymentSummary[],
  requestCounts: Map<string, number> = new Map(),
) {
  const [filter, setFilter] = useState("");
  const [sortBy, setSortBy] = useState<SortOption>("recent");

  const filtered = useMemo(() => {
    let list = deployments;

    if (filter) {
      const lower = filter.toLowerCase();
      list = list.filter(
        (d) =>
          d.name.toLowerCase().includes(lower) ||
          d.display_name?.toLowerCase().includes(lower),
      );
    }

    if (sortBy === "name") {
      list = [...list].sort((a, b) =>
        (a.display_name || a.name).localeCompare(b.display_name || b.name),
      );
    } else if (sortBy === "recent") {
      list = [...list].sort(
        (a, b) =>
          new Date(b.updated_at || b.created_at).getTime() -
          new Date(a.updated_at || a.created_at).getTime(),
      );
    } else if (sortBy === "requests") {
      list = [...list].sort(
        (a, b) => (requestCounts.get(b.id) ?? 0) - (requestCounts.get(a.id) ?? 0),
      );
    }

    return list;
  }, [deployments, filter, sortBy, requestCounts]);

  return {
    filtered,
    toolbarProps: {
      filter,
      onFilterChange: setFilter,
      sortBy,
      onSortChange: setSortBy,
    },
  };
}
