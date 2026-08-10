import { useCallback, useEffect, useMemo, useState } from "react";
import type { AgentDeploymentSummary } from "@/lib/api";
import { getPersistentStorageSnapshot, setPersistentStorageSnapshot } from "@/lib/persistent-storage";

export type SortOption = "recent" | "name" | "requests";

export type DeploymentStatusFilter = "active" | "stopped" | "error";

// Maps a coarse filter onto the loose UI status string a summary carries
// (dbStatusToUIStatus on the server): active -> Running, stopped -> Stopped,
// error -> error (failed/suspended/undeployed all surface as error).
const STATUS_FILTER_UI: Record<DeploymentStatusFilter, string> = {
  active: "Running",
  stopped: "Stopped",
  error: "error",
};

const STATUS_FILTER_STORAGE_KEY = "astro:page-filters:agents-status";

function readStoredStatusFilter(): DeploymentStatusFilter | null {
  const v = getPersistentStorageSnapshot(STATUS_FILTER_STORAGE_KEY);
  return v === "active" || v === "stopped" || v === "error" ? v : null;
}

export function useAgentFilters<T extends AgentDeploymentSummary>(
  deployments: T[],
  requestCounts: Map<string, number> = new Map(),
  controlledFilter?: {
    filter: string;
    onFilterChange: (value: string) => void;
  },
) {
  const [localFilter, setLocalFilter] = useState("");
  const [sortBy, setSortBy] = useState<SortOption>("recent");
  const filter = controlledFilter?.filter ?? localFilter;
  const setFilter = controlledFilter?.onFilterChange ?? setLocalFilter;
  const filterLocally = controlledFilter == null;
  const [statusFilter, setStatusFilterState] = useState<DeploymentStatusFilter | null>(null);

  // Restore the last-used status filter on mount (client only, so SSR markup
  // stays at the default and hydration matches), then persist every change.
  useEffect(() => {
    const stored = readStoredStatusFilter();
    if (stored) setStatusFilterState(stored);
  }, []);
  const setStatusFilter = useCallback((next: DeploymentStatusFilter | null) => {
    setStatusFilterState(next);
    setPersistentStorageSnapshot(STATUS_FILTER_STORAGE_KEY, next ?? "");
  }, []);

  const filtered = useMemo(() => {
    let list = deployments;

    // The dashboard supplies a controlled filter because its query searches the
    // full server-side scope. Local filtering remains for standalone consumers.
    if (filter && filterLocally) {
      const lower = filter.toLowerCase();
      list = list.filter(
        (d) =>
          d.name.toLowerCase().includes(lower) ||
          d.display_name?.toLowerCase().includes(lower),
      );
    }

    if (statusFilter) {
      list = list.filter((d) => d.status === STATUS_FILTER_UI[statusFilter]);
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
  }, [deployments, filter, filterLocally, sortBy, statusFilter, requestCounts]);

  return {
    filtered,
    toolbarProps: {
      filter,
      onFilterChange: setFilter,
      sortBy,
      onSortChange: setSortBy,
      statusFilter,
      onStatusChange: setStatusFilter,
    },
  };
}
