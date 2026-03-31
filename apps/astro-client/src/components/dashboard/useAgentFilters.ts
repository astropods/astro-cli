import { useState, useMemo } from "react";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";

export type SortOption = "recent" | "name" | "requests";

export function useAgentFilters(
  deployments: AgentDeployment[],
  requestCounts: Map<string, number> = new Map(),
) {
  const [filter, setFilter] = useState("");
  const [sortBy, setSortBy] = useState<SortOption>("recent");
  const [statusFilter, setStatusFilter] = useState<string[]>([]);

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

    if (statusFilter.length > 0) {
      list = list.filter((d) => statusFilter.includes(mapDeploymentStatus(d)));
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
  }, [deployments, filter, statusFilter, sortBy, requestCounts]);

  return {
    filtered,
    toolbarProps: {
      filter,
      onFilterChange: setFilter,
      statusFilter,
      onStatusFilterChange: setStatusFilter,
      sortBy,
      onSortChange: setSortBy,
    },
  };
}
