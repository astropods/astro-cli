import { useState } from "react";
import { ChevronRight, ChevronDown, Loader2 } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import {
  normalizeLevel,
  levelColorClass,
  formatLogTimestamp,
  type LogEntry,
} from "@/lib/log-utils";

// ── Types ─────────────────────────────────────────────────────────────────────

export interface BuildLogComponentData {
  name: string;
  status: string;
  logs: string;
}

export interface BuildLogViewerProps {
  components?: BuildLogComponentData[];
  isLoading?: boolean;
  isError?: boolean;
}

interface LogSection {
  name: string;
  content: string;
}

// ── Log line parsing ──────────────────────────────────────────────────────────

// Matches: "2024-01-15T10:30:00Z INFO message" or "2024-01-15 10:30:00 INFO message"
const TS_LEVEL_RE = /^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)\s+(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL)\s+(.*)/i;
// Matches: "ERROR: message", "WARN message"
const LEVEL_RE = /^(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL)[:\s]\s*(.*)/i;

function parseLogLine(line: string): LogEntry {
  const tsMatch = line.match(TS_LEVEL_RE);
  if (tsMatch) return { timestamp: tsMatch[1], level: tsMatch[2], message: tsMatch[3] };

  const lvlMatch = line.match(LEVEL_RE);
  if (lvlMatch) return { timestamp: null, level: lvlMatch[1], message: lvlMatch[2] };

  return { timestamp: null, level: null, message: line };
}

function parseLogLines(raw: string): LogEntry[] {
  return raw
    .split("\n")
    .filter((l) => l.trim())
    .map(parseLogLine);
}

// ── Section parsing ───────────────────────────────────────────────────────────

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

// ── Sub-components ────────────────────────────────────────────────────────────

function statusClass(status: string) {
  if (status === "succeeded") return "text-green-600 dark:text-green-400";
  if (status === "failed") return "text-destructive";
  if (status === "building") return "text-blue-500";
  return "text-muted-foreground";
}

function LogLines({ entries }: { entries: LogEntry[] }) {
  if (entries.length === 0) {
    return <p className="font-mono text-mono-sm text-faint-foreground py-2 pl-[18px]">(empty)</p>;
  }
  return (
    <>
      {entries.map((entry, i) => {
        const level = normalizeLevel(entry.level);
        const lvlClass = levelColorClass(entry.level);
        return (
          <div
            key={i}
            className="flex items-baseline gap-x-3 px-[18px] py-[1px] font-mono text-mono-sm tracking-normal leading-5"
          >
            {entry.timestamp ? (
              <span className="text-faint-foreground shrink-0 w-[24ch]">
                {formatLogTimestamp(entry.timestamp)}
              </span>
            ) : null}
            {entry.level ? (
              <span className={cn("font-medium w-[5ch] shrink-0", lvlClass)}>
                {level}
              </span>
            ) : null}
            <span className="text-foreground whitespace-pre-wrap break-all">
              {entry.message}
            </span>
          </div>
        );
      })}
    </>
  );
}

// ── BuildLogViewer ────────────────────────────────────────────────────────────

export function BuildLogViewer({ components = [], isLoading, isError }: BuildLogViewerProps) {
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
      <p className="font-mono text-mono-sm text-faint-foreground p-4">(no output)</p>
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
                  const entries = parseLogLines(section.content);

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
                        {entries.length > 0 && (
                          <span className="ml-auto font-mono text-[10px] text-muted-foreground">
                            {entries.length} lines
                          </span>
                        )}
                      </button>

                      {sectionOpen && (
                        <div className="border-t border-border bg-background py-2 max-h-72 overflow-y-auto">
                          <LogLines entries={entries} />
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
