import { cn } from "@/lib/utils";

export type TraceTab = "trace" | "tree";

export interface TraceTabsProps {
  active: TraceTab;
  onChange: (tab: TraceTab) => void;
  observationCount?: number;
}

const TABS: { id: TraceTab; label: string }[] = [
  { id: "trace", label: "Overview" },
  { id: "tree", label: "Tree" },
];

export function TraceTabs({ active, onChange, observationCount }: TraceTabsProps) {
  return (
    <div className="flex items-center gap-1 border-b border-border px-2">
      {TABS.map((tab) => {
        const isActive = tab.id === active;
        return (
          <button
            key={tab.id}
            type="button"
            onClick={() => onChange(tab.id)}
            aria-pressed={isActive}
            className={cn(
              "relative px-3 py-2 text-body-sm transition-colors",
              isActive
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <span>{tab.label}</span>
            {tab.id === "tree" && observationCount != null && observationCount > 0 && (
              <span className="ml-1 rounded bg-muted/60 px-1.5 py-0.5 text-mono-sm text-muted-foreground">
                {observationCount}
              </span>
            )}
            {isActive && (
              <span className="absolute inset-x-2 -bottom-px h-px bg-foreground" />
            )}
          </button>
        );
      })}
    </div>
  );
}
