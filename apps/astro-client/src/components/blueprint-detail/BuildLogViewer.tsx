import { useState } from "react";
import { ChevronRight, ChevronDown, Loader2 } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";

// ── Types ─────────────────────────────────────────────────────────────────────

export interface BuildLogComponentData {
  name: string;
  status: string;
  logs: string;
}

export interface BuildLogViewerProps {
  components: BuildLogComponentData[];
  isLoading?: boolean;
  isError?: boolean;
}

interface LogSection {
  name: string;
  content: string;
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function parseLogSections(raw: string): LogSection[] {
  const sections: LogSection[] = [];
  const parts = raw.split(/^=== .+? ===$\n?/m);
  const headers = [...raw.matchAll(/^=== (.+?) ===/gm)].map((m) => m[1]);

  if (parts[0]?.trim()) {
    sections.push({ name: "output", content: parts[0] });
  }
  headers.forEach((name, i) => {
    sections.push({ name, content: parts[i + 1] ?? "" });
  });

  if (sections.length === 0 && raw.trim()) {
    sections.push({ name: "output", content: raw });
  }
  return sections;
}

function statusClass(status: string) {
  if (status === "succeeded") return "text-green-600 dark:text-green-400";
  if (status === "failed") return "text-destructive";
  if (status === "building") return "text-blue-500";
  return "text-muted-foreground";
}

// ── Component ─────────────────────────────────────────────────────────────────

export function BuildLogViewer({ components, isLoading, isError }: BuildLogViewerProps) {
  const [expandedComponents, setExpandedComponents] = useState<Set<string>>(
    () => new Set(components.length > 0 ? [components[0].name] : [])
  );
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set());

  function toggleComponent(name: string) {
    setExpandedComponents((prev) => {
      const next = new Set(prev);
      next.has(name) ? next.delete(name) : next.add(name);
      return next;
    });
  }

  function toggleSection(key: string) {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12 gap-2 text-muted-foreground text-sm">
        <Spinner size={16} />
        Loading logs…
      </div>
    );
  }

  if (isError) {
    return (
      <p className="text-sm text-destructive p-4">
        Logs unavailable. The pod may have been cleaned up.
      </p>
    );
  }

  if (components.length === 0) {
    return (
      <p className="font-mono text-xs text-muted-foreground p-4">(no output)</p>
    );
  }

  return (
    <div className="divide-y divide-border">
      {components.map((comp) => {
        const compOpen = expandedComponents.has(comp.name);
        const sections = parseLogSections(comp.logs || "");

        return (
          <div key={comp.name}>
            {/* Component row */}
            <button
              onClick={() => toggleComponent(comp.name)}
              className="w-full flex items-center gap-2.5 px-4 py-3 text-left hover:bg-muted/40 transition-colors"
            >
              {compOpen
                ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
              <span className="font-mono font-semibold text-sm">{comp.name}</span>
              <span className={cn("font-mono text-xs", statusClass(comp.status))}>
                {comp.status === "building" && (
                  <Loader2 className="inline h-2.5 w-2.5 animate-spin mr-1 -mt-0.5" />
                )}
                {comp.status}
              </span>
            </button>

            {/* Sections */}
            {compOpen && (
              <div className="divide-y divide-border border-t border-border">
                {sections.map((section) => {
                  const sectionKey = `${comp.name}/${section.name}`;
                  const sectionOpen = expandedSections.has(sectionKey);
                  const lines = section.content.split("\n").filter((l) => l.trim()).length;

                  return (
                    <div key={sectionKey}>
                      <button
                        onClick={() => toggleSection(sectionKey)}
                        className="w-full flex items-center gap-2 pl-9 pr-4 py-2.5 text-left hover:bg-muted/40 transition-colors"
                      >
                        {sectionOpen
                          ? <ChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
                          : <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />}
                        <span className="font-mono text-sm">{section.name}</span>
                        {lines > 0 && (
                          <span className="ml-auto font-mono text-[10px] text-muted-foreground">
                            {lines} lines
                          </span>
                        )}
                      </button>

                      {sectionOpen && (
                        <div className="border-t border-border bg-muted/30">
                          <pre className="pl-14 pr-4 py-3 font-mono text-[11px] whitespace-pre-wrap break-all leading-[1.7] text-foreground max-h-72 overflow-y-auto">
                            {section.content.trim() || "(empty)"}
                          </pre>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
