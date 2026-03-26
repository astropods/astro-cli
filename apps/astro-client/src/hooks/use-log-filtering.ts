import { useMemo, useState } from "react";

type LogFilter = "errors" | "warnings";

export function useLogFiltering(logs: string[], search: string) {
  const [activeFilters, setActiveFilters] = useState<Set<LogFilter>>(new Set());

  const toggleFilter = (f: LogFilter) =>
    setActiveFilters((prev) => {
      const n = new Set(prev);
      if (n.has(f)) n.delete(f);
      else n.add(f);
      return n;
    });

  const errCount = useMemo(() => logs.filter((l) => /error|failed|fatal/i.test(l)).length, [logs]);
  const warnCount = useMemo(() => logs.filter((l) => /warn|warning|retry|attempt/i.test(l)).length, [logs]);

  const filtered = useMemo(() => {
    return logs.filter((l) => {
      if (activeFilters.size > 0) {
        const isErr = /error|failed|fatal/i.test(l);
        const isWarn = /warn|warning|retry|attempt/i.test(l);
        if (activeFilters.has("errors") && activeFilters.has("warnings") && !isErr && !isWarn) return false;
        if (activeFilters.has("errors") && !activeFilters.has("warnings") && !isErr) return false;
        if (activeFilters.has("warnings") && !activeFilters.has("errors") && !isWarn) return false;
      }
      if (search && !l.toLowerCase().includes(search.toLowerCase())) return false;
      return true;
    });
  }, [logs, activeFilters, search]);

  return { activeFilters, toggleFilter, errCount, warnCount, filtered };
}
