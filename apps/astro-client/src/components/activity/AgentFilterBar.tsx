import { useMemo } from "react";
import { MultiSelectFilterBar, type FilterEntry } from "./MultiSelectFilterBar";
import { ALL_AGENTS_KEY, ALL_AGENTS_COLOR } from "./use-insights-data";

interface AgentFilterBarProps {
  value: string[];
  onValueChange: (values: string[]) => void;
  allAgentNames: string[];
  colorMap: Record<string, string>;
}

export function AgentFilterBar({ value, onValueChange, allAgentNames, colorMap }: AgentFilterBarProps) {
  const entries = useMemo<FilterEntry[]>(
    () => allAgentNames.map((name) => ({
      key: name,
      label: name,
      color: colorMap[name] ?? "var(--color-muted-foreground)",
    })),
    [allAgentNames, colorMap],
  );
  return (
    <MultiSelectFilterBar
      value={value}
      onValueChange={onValueChange}
      entries={entries}
      allItem={{ key: ALL_AGENTS_KEY, label: "All agents", color: ALL_AGENTS_COLOR }}
      placeholder="Search agents..."
    />
  );
}
