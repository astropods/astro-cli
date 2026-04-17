import { useMemo, useState } from "react";
import { normalizeLevel, type LogEntry } from "@/lib/log-utils";

export type LogFilter = "errors" | "warnings";

export function useLogFiltering(logs: LogEntry[]) {
  const [activeFilters, setActiveFilters] = useState<Set<LogFilter>>(new Set());

  const toggleFilter = (f: LogFilter) =>
    setActiveFilters((prev) => {
      const n = new Set(prev);
      if (n.has(f)) n.delete(f);
      else n.add(f);
      return n;
    });

  const { errCount, warnCount, filtered } = useMemo(() => {
    let errs = 0;
    let warns = 0;
    const result: LogEntry[] = [];

    for (const entry of logs) {
      const level = normalizeLevel(entry.level);
      const isErr = level === "ERROR" || level === "FATAL";
      const isWarn = level === "WARN";
      if (isErr) errs++;
      if (isWarn) warns++;

      if (activeFilters.size > 0) {
        const wantErr = activeFilters.has("errors");
        const wantWarn = activeFilters.has("warnings");
        if (wantErr && wantWarn && !isErr && !isWarn) continue;
        if (wantErr && !wantWarn && !isErr) continue;
        if (wantWarn && !wantErr && !isWarn) continue;
      }
      result.push(entry);
    }

    return { errCount: errs, warnCount: warns, filtered: result };
  }, [logs, activeFilters]);

  return { activeFilters, toggleFilter, errCount, warnCount, filtered };
}
