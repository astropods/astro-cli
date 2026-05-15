import { useState } from "react";
import { ChevronRight, ChevronDown, Loader2, Github, ArrowRight, CheckCircle2, XCircle, Clock } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { AstroIcon } from "@/components/ui/astro-icon";
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
  duration?: string;
}

export interface BuildLogViewerProps {
  commitSha?: string;
  buildId?: string;
  totalDuration?: string;
  components?: BuildLogComponentData[];
  isLoading?: boolean;
  error?: string;
}

interface LogSection {
  name: string;
  content: string;
}

// Sections to hide entirely.
const HIDDEN_SECTIONS = new Set(["events", "ecr-login"]);

// Renames applied to section names.
const SECTION_RENAMES: Record<string, string> = {
  buildkit: "build",
};

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

  return sections
    .filter((s) => !HIDDEN_SECTIONS.has(s.name))
    .map((s) => ({ ...s, name: SECTION_RENAMES[s.name] ?? s.name }));
}

// ── Sub-components ────────────────────────────────────────────────────────────

function StatusIcon({ status, size = "md" }: { status: string; size?: "sm" | "md" }) {
  const sm = size === "sm";
  const cls = sm ? "h-3 w-3 shrink-0" : "h-3.5 w-3.5 shrink-0";
  if (status === "succeeded") return <CheckCircle2 className={cn(cls, "text-green-600 dark:text-green-400")} />;
  if (status === "failed") return <XCircle className={cn(cls, "text-destructive")} />;
  if (status === "building") return <Loader2 className={cn(cls, "text-blue-500 animate-spin")} />;
  return <Clock className={cn(cls, "text-muted-foreground")} />;
}

function LogLines({ entries }: { entries: LogEntry[] }) {
  if (entries.length === 0) {
    return <p className="font-mono text-mono-sm text-faint-foreground py-2 pl-9">(empty)</p>;
  }
  return (
    <>
      {entries.map((entry, i) => {
        const level = normalizeLevel(entry.level);
        const lvlClass = levelColorClass(entry.level);
        return (
          <div
            key={i}
            className="flex items-baseline gap-x-3 pl-9 pr-4 py-[1px] font-mono text-mono-sm tracking-normal leading-5"
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

export function BuildLogViewer({ commitSha, buildId, totalDuration, components = [], isLoading, error }: BuildLogViewerProps) {
  const [activeTab, setActiveTab] = useState(() => components[0]?.name ?? "");
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set());

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

  const activeComp = components.find((c) => c.name === activeTab) ?? components[0];
  const sections = parseLogSections(activeComp?.logs || "");

  return (
    <div>
      {/* Header */}
      <div className="px-4 pt-4 pb-3 border-b border-border">
        <p className="text-sm font-semibold">Build Logs</p>
        {commitSha && (
          <div className="flex items-center gap-2 mt-1">
            <span className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground">
              <Github className="h-3.5 w-3.5 shrink-0" />
              {commitSha}
            </span>
            <ArrowRight className="h-3 w-3 text-muted-foreground shrink-0" />
            {buildId ? (
              <span className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground">
                <AstroIcon className="h-3.5 w-3.5" />
                {buildId}
              </span>
            ) : (
              <span className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground/50">
                <AstroIcon className="h-3.5 w-3.5" />
                <Loader2 className="h-3 w-3 animate-spin" />
                pending
              </span>
            )}
            {totalDuration && (
              <span className="ml-auto font-mono text-xs text-muted-foreground">
                total duration: {totalDuration}
              </span>
            )}
          </div>
        )}
      </div>

      {error ? (
        <p className="font-mono text-mono-sm text-destructive p-4">{error}</p>
      ) : components.length === 0 ? (
        <p className="font-mono text-mono-sm text-faint-foreground p-4">(no output)</p>
      ) : (
        <>
          {/* Single component label */}
          {components.length === 1 && (
            <div className="flex items-center gap-2 px-4 py-2.5 border-b border-border">
              <StatusIcon status={activeComp.status} size="sm" />
              <span className="text-sm font-medium">{activeComp.name}</span>
              {activeComp.duration && (
                <span className="font-mono text-[10px] text-muted-foreground ml-auto">{activeComp.duration}</span>
              )}
            </div>
          )}

          {/* Tab bar — only shown when there are multiple components */}
          {components.length > 1 && (
            <div className="flex border-b border-border overflow-x-auto">
              {components.map((comp) => (
                <button
                  key={comp.name}
                  onClick={() => setActiveTab(comp.name)}
                  className={cn(
                    "flex items-center gap-2 px-4 py-2.5 text-sm whitespace-nowrap transition-colors border-b-2 -mb-px",
                    activeTab === comp.name
                      ? "border-foreground text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground",
                  )}
                >
                  <StatusIcon status={comp.status} size="sm" />
                  <span>{comp.name}</span>
                  {comp.duration && (
                    <span className="font-mono text-[10px] text-muted-foreground">{comp.duration}</span>
                  )}
                </button>
              ))}
            </div>
          )}

          {/* Sections accordion */}
          <div>
            {sections.map((section) => {
              const sectionKey = `${activeComp.name}/${section.name}`;
              const sectionOpen = expandedSections.has(sectionKey);
              const entries = parseLogLines(section.content);

              return (
                <div key={sectionKey}>
                  <button
                    onClick={() => toggleSection(sectionKey)}
                    className="w-full flex items-center gap-2 px-4 py-2.5 text-left hover:bg-muted/40 transition-colors"
                  >
                    {sectionOpen
                      ? <ChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
                      : <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />}
                    <span className="font-mono text-sm">{section.name}</span>
                    {entries.length > 0 && (
                      <span className="ml-auto font-mono text-[10px] text-muted-foreground/60">
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
        </>
      )}
    </div>
  );
}
