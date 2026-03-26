import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Search, Loader2, X, RefreshCw, Copy, Check, MoreVertical, Globe } from "lucide-react";
import { useDeploymentLogs, useDeploymentHistory } from "@/api/queries/deployments";
import {
  formatDate,
  isDeployingState,
  mapDeploymentStatus,
} from "@/lib/deployment-utils";
import type { AgentDeployment, ApiError, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { deploymentKeys } from "@/api/queries/keys";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { DeploymentHistoryTableRow, DeployHistoryStatus } from "./history/types";
import { ErrorPanel } from "@/components/ui/status-panel";
import { InlineBadge } from "@/components/InlineBadge";

const C = {
  bg: "var(--muted)",
  bgAlt: "var(--surface)",
  bgDeep: "var(--muted)",
  panel: "var(--surface)",
  border: "var(--border)",
  teal: "var(--primary)",
  tealMid: "var(--color-teal-600)",
  tealLt: "var(--color-teal-400)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  faint: "var(--faint-foreground)",
  stone: "var(--color-stone-500)",
  amber: "var(--color-amber-800)",
  amberBg: "color-mix(in oklch, var(--color-amber-700) 12%, transparent)",
  amberBdr: "color-mix(in oklch, var(--color-amber-700) 28%, transparent)",
  warning: "var(--color-yellow-500)",
  coral: "var(--color-coral-600)",
  coralBg: "color-mix(in oklch, var(--color-coral-600) 12%, transparent)",
  coralBdr: "color-mix(in oklch, var(--color-coral-600) 28%, transparent)",
  success: "var(--color-green-700)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  heading1: "var(--text-heading-1)",
  heading2: "var(--text-heading-2)",
  heading4: "var(--text-heading-4)",
  body: "var(--text-body)",
  bodySm: "var(--text-body-sm)",
  label: "var(--text-label)",
  monoSm: "var(--text-mono-sm)",
  monoMd: "var(--text-mono-md)",
} as const;

const I = {
  xs: 10,
  sm: 12,
  md: 14,
} as const;

type LogTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

const LOG_TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: "15m", label: "Last 15 min" },
  { value: "1h", label: "Last 1 hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];
const DEPLOYMENT_GRID_COLUMNS = "minmax(220px, 1fr) 88px 84px 116px 116px 28px";

export interface ActiveContainerAccordionProps {
  workloadName: string;
  title: string;
  isCompact?: boolean;
  isAgentService?: boolean;
  url?: string;
  urls?: { name: string; url: string; type?: string }[];
  readyText: string;
  uptime: string;
  containers: {
    name: string;
    ready: boolean;
    vars: { key: string; value: string; secret: boolean; source: string }[];
  }[];
  deploymentId: string;
  deploymentStatus: DeployHistoryStatus;
  isOpen: boolean;
  onToggle: () => void;
}

function logLineColor(line: string): string {
  const l = line.toLowerCase();
  if (/✓|connected|ready|healthy|initialized|registered|success|loaded|complete/.test(l)) return C.success;
  if (/error|failed|exception|fatal/.test(l)) return C.coral;
  if (/warn|warning|retry|attempt/.test(l)) return C.warning;
  return C.text;
}

function splitLogLineTimestamp(line: string): { timestamp: string | null; message: string } {
  const iso = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$/);
  if (iso) return { timestamp: iso[1], message: iso[2] };
  const basic = line.match(/^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+(.*)$/);
  if (basic) return { timestamp: basic[1], message: basic[2] };
  return { timestamp: null, message: line };
}

function formatLogTimestamp(timestamp: string | null): string {
  if (!timestamp) return "—";
  const m = timestamp.match(
    /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})?$/,
  );
  if (!m) return timestamp;
  const date = m[1];
  const time = m[2];
  const millis = ((m[3] ?? "") + "000").slice(0, 3);
  return `${date} ${time}.${millis}`;
}

async function copyTextToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to legacy copy below
  }

  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "true");
    textarea.style.position = "absolute";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}

function isSensitiveEnvVar(key: string, value: string, source: string): boolean {
  if (source.startsWith("secret:")) return true;

  const upperKey = key.toUpperCase();
  const keyLooksSensitive =
    upperKey.includes("KEY") ||
    upperKey.includes("TOKEN") ||
    upperKey.includes("SECRET") ||
    upperKey.includes("PASSWORD") ||
    upperKey.includes("PASSWD") ||
    upperKey.includes("PRIVATE") ||
    upperKey.includes("CREDENTIAL") ||
    upperKey.includes("AUTH") ||
    upperKey.includes("DSN") ||
    upperKey.includes("WEBHOOK");

  const valueLooksSensitive =
    value.startsWith("sk-") ||
    value.startsWith("secret:") ||
    value.includes("••");

  return keyLooksSensitive || valueLooksSensitive;
}


export function ActiveContainerAccordion({
  workloadName,
  title,
  isCompact = false,
  isAgentService = false,
  url,
  urls,
  readyText,
  uptime,
  containers,
  deploymentId,
  deploymentStatus,
  isOpen,
  onToggle,
}: ActiveContainerAccordionProps) {
  const [view, setView] = useState<"logs" | "vars" | "domains">("logs");
  const [logSearch, setLogSearch] = useState("");
  const [logTimeRange, setLogTimeRange] = useState<LogTimeRange>("24h");
  const [activeFilters, setActiveFilters] = useState<Set<"errors" | "warnings">>(new Set());
  const [copiedPlaygroundCommand, setCopiedPlaygroundCommand] = useState(false);
  const [copiedLogs, setCopiedLogs] = useState(false);
  const [selectedContainer, setSelectedContainer] = useState<string>(containers[0]?.name ?? "");

  useEffect(() => {
    setSelectedContainer((prev) => {
      if (containers.length === 0) return "";
      if (containers.some((c) => c.name === prev)) return prev;
      return containers[0].name;
    });
  }, [containers]);

  const activeContainer = useMemo(
    () => containers.find((c) => c.name === selectedContainer) ?? containers[0],
    [containers, selectedContainer],
  );
  const vars = activeContainer?.vars ?? [];
  const canShowVars = selectedContainer !== "collector";
  const canShowDomains = (urls ?? []).length > 0;
  const totalContainers = containers.length;
  const readyContainers = containers.filter((container) => container.ready).length;
  const allReady = totalContainers > 0 && readyContainers === totalContainers;

  useEffect(() => {
    if (!canShowVars && view === "vars") {
      setView("logs");
    }
    if (!canShowDomains && view === "domains") {
      setView("logs");
    }
  }, [canShowVars, canShowDomains, view]);

  const { data: logsRaw, isLoading, isFetching, error, refetch } = useDeploymentLogs(
    deploymentId,
    workloadName,
    selectedContainer,
    logTimeRange,
    { enabled: isOpen && !!selectedContainer, refetchInterval: isOpen && deploymentStatus === "deploying" ? 3000 : false },
  );

  const logs = useMemo(() => (logsRaw ?? "").split("\n"), [logsRaw]);

  const logErrorMessage = error
    ? (error as unknown as ApiError & { details?: string }).details ??
      (error as unknown as ApiError).error_description ??
      (error as Error).message ??
      "Failed to fetch logs"
    : null;

  const toggleFilter = (f: "errors" | "warnings") =>
    setActiveFilters((prev) => {
      const n = new Set(prev);
      if (n.has(f)) n.delete(f);
      else n.add(f);
      return n;
    });

  const errCount = logs.filter((l) => /error|failed|fatal/i.test(l)).length;
  const warnCount = logs.filter((l) => /warn|warning|retry|attempt/i.test(l)).length;
  const filtered = logs.filter((l) => {
    if (activeFilters.size > 0) {
      const isErr = /error|failed|fatal/i.test(l);
      const isWarn = /warn|warning|retry|attempt/i.test(l);
      if (activeFilters.has("errors") && activeFilters.has("warnings") && !isErr && !isWarn) return false;
      if (activeFilters.has("errors") && !activeFilters.has("warnings") && !isErr) return false;
      if (activeFilters.has("warnings") && !activeFilters.has("errors") && !isWarn) return false;
    }
    if (logSearch && !l.toLowerCase().includes(logSearch.toLowerCase())) return false;
    return true;
  });

  const hasPublicUrl = !!url;
  const playgroundCommand = hasPublicUrl ? `ast playground ${url}` : "ast playground <deployment-url>";

  const handleCopyPlaygroundCommand = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!hasPublicUrl) return;
    const ok = await copyTextToClipboard(playgroundCommand);
    if (!ok) return;
    setCopiedPlaygroundCommand(true);
    setTimeout(() => setCopiedPlaygroundCommand(false), 1200);
  };

  const handleCopyLogs = async () => {
    const payload = logs.join("\n");
    if (!payload.trim()) return;
    const ok = await copyTextToClipboard(payload);
    if (!ok) return;
    setCopiedLogs(true);
    setTimeout(() => setCopiedLogs(false), 900);
  };

  return (
    <div style={{ marginBottom: 6 }}>
      <div
        className="dp-container-hdr"
        style={{
          display: "flex",
          alignItems: isCompact ? "flex-start" : "center",
          flexWrap: isCompact ? "wrap" : "nowrap",
          gap: 8,
          width: "100%",
          padding: "10px 14px",
          borderRadius: isOpen ? "8px 8px 0 0" : 8,
          border: `1px solid ${C.border}`,
          borderBottom: isOpen ? `1px solid ${C.bgDeep}` : `1px solid ${C.border}`,
          background: isOpen ? C.bgAlt : C.bg,
          cursor: "pointer",
          textAlign: "left" as const,
          transition: "background 0.15s",
        }}
        onClick={onToggle}
        onMouseEnter={(e) => {
          if (!isOpen) e.currentTarget.style.background = "var(--color-stone-200)";
        }}
        onMouseLeave={(e) => {
          if (!isOpen) e.currentTarget.style.background = C.bg;
        }}
      >
        <ChevronRight size={I.md} color={C.faint} style={{ flexShrink: 0, transform: isOpen ? "rotate(90deg)" : "none", transition: "transform 0.18s" }} />
        {deploymentStatus === "deploying" || deploymentStatus === "undeploying" ? (
          <Loader2
            size={16}
            style={{
              color: deploymentStatus === "deploying" ? C.warning : C.faint,
              animation: "dp-spin 1.2s linear infinite",
              flexShrink: 0,
            }}
          />
        ) : allReady ? (
          <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
            <circle cx="12" cy="12" r="10" fill="rgba(21,130,125,0.12)" />
            <path d="M7.5 12l3 3 6-6" stroke={C.tealMid} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
          </svg>
        ) : (
          <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
            <circle cx="12" cy="12" r="10" fill={C.amberBg} />
            <circle cx="12" cy="12" r="4" fill={C.amber} />
          </svg>
        )}
        <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 500, color: C.text }} title={workloadName}>
          {title}
        </span>
        <span style={{ flex: 1 }} />
        {isAgentService && (
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexShrink: 0, minWidth: 0 }} onClick={(e) => e.stopPropagation()}>
            <div style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0, color: C.text }}>
              <span style={{ fontFamily: S.body, fontSize: T.bodySm, whiteSpace: "nowrap" as const }}>
                To chat, run:
              </span>
              <button
                type="button"
                onClick={handleCopyPlaygroundCommand}
                title={hasPublicUrl ? playgroundCommand : "Public URL not available yet"}
                disabled={!hasPublicUrl}
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 5,
                  maxWidth: isCompact ? "min(430px, 50vw)" : "min(430px, 55vw)",
                  border: `1px solid ${C.border}`,
                  borderRadius: 5,
                  padding: "2px 8px",
                  background: "var(--color-stone-200)",
                  cursor: hasPublicUrl ? "pointer" : "not-allowed",
                  color: !hasPublicUrl ? C.faint : copiedPlaygroundCommand ? C.tealMid : C.muted,
                  opacity: hasPublicUrl ? 1 : 0.7,
                }}
              >
                <span
                  style={{
                    fontFamily: S.mono,
                    fontSize: T.monoSm,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap" as const,
                  }}
                >
                  {playgroundCommand}
                </span>
                {hasPublicUrl ? (copiedPlaygroundCommand ? <Check size={I.sm} /> : <Copy size={I.sm} />) : null}
              </button>
            </div>
          </div>
        )}
        <span
          style={{
            fontFamily: S.mono,
            fontSize: T.monoSm,
            color: C.text,
            flexShrink: 0,
            marginLeft: isCompact ? 0 : 8,
            width: isCompact ? "100%" : "auto",
          }}
        >
          <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
            {readyText}
            <span>ready</span>
            {allReady ? <Check size={I.xs} /> : null}
          </span>
          {" • "}
          {uptime}
        </span>
      </div>

      {isOpen && (
        <div style={{ border: `1px solid ${C.border}`, borderTop: "none", borderRadius: "0 0 8px 8px", overflow: "hidden" }}>
          <div style={{ display: "flex", alignItems: "center", flexWrap: isCompact ? "wrap" : "nowrap", background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
            {(["logs", "vars", "domains"] as const).map((v) => (
              (v === "vars" && !canShowVars) || (v === "domains" && !canShowDomains) ? null : (
              <button
                key={v}
                onClick={() => setView(v)}
                style={{
                  padding: "7px 14px",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  fontFamily: S.body,
                  fontSize: T.body,
                  fontWeight: view === v ? 500 : 400,
                  color: view === v ? C.text : C.faint,
                  borderBottom: view === v ? `2px solid ${C.tealMid}` : "2px solid transparent",
                  transition: "color 0.12s",
                  textTransform: "capitalize" as const,
                }}
              >
                {v === "vars" ? "Variables" : v === "domains" ? "Domains" : "Logs"}
              </button>
              )
            ))}
            {containers.length > 1 && (
              <div style={{ marginLeft: "auto", paddingRight: 8, paddingBottom: isCompact ? 8 : 0 }}>
                <Select value={selectedContainer} onValueChange={setSelectedContainer}>
                  <SelectTrigger
                    className="h-7 w-auto min-w-[130px] px-3"
                    style={{ fontFamily: S.body, fontSize: T.bodySm, background: "var(--popover)" }}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {containers.map((container) => (
                      <SelectItem key={container.name} value={container.name}>
                        {container.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>

          {view === "vars" && (
            <div style={{ background: "var(--color-stone-50)" }}>
              {vars.length === 0 ? (
                <div style={{ padding: "16px", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>No variables</div>
              ) : (
                vars.map((v, vi) => {
                  const isSecret =
                    v.secret || v.value.startsWith("sk-") || v.value.startsWith("secret:") || v.value.includes("••");
                  const srcStyle =
                    v.source === "input"
                      ? { bg: "rgba(21,130,125,0.1)", color: C.tealMid, label: "input" }
                      : v.source === "injected"
                        ? { bg: "rgba(212,143,30,0.1)", color: C.amber, label: "injected" }
                        : { bg: C.bgDeep, color: C.stone, label: "static" };
                  return (
                    <div
                      key={v.key}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 10,
                        padding: "9px 16px",
                        borderBottom: vi < vars.length - 1 ? `1px solid ${C.border}` : "none",
                      }}
                    >
                      <span style={{ fontFamily: S.mono, fontSize: T.label, color: C.stone, flexShrink: 0, userSelect: "none" as const }}>
                        {"{}"}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: !v.value ? C.stone : C.text, minWidth: 160, flexShrink: 0, textDecoration: !v.value ? "line-through" : undefined }}>
                        {v.key}
                      </span>
                      <div style={{ flex: 1, display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: !v.value ? C.stone : C.muted, fontStyle: !v.value ? "italic" : undefined, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }}>
                          {!v.value ? "empty" : isSecret ? "•••••••••" : v.value}
                        </span>
                      </div>
                      <span style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.08em", padding: "2px 6px", borderRadius: 4, background: srcStyle.bg, color: srcStyle.color, flexShrink: 0 }}>
                        {srcStyle.label}
                      </span>
                    </div>
                  );
                })
              )}
            </div>
          )}

          {view === "domains" && (
            <div style={{ background: "var(--color-stone-50)" }}>
              {(urls ?? []).map((u, i) => (
                <div
                  key={u.url}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "9px 16px",
                    borderBottom: i < (urls ?? []).length - 1 ? `1px solid ${C.border}` : "none",
                  }}
                >
                  <Globe size={14} style={{ flexShrink: 0, color: C.faint }} />
                  <span style={{ fontFamily: S.body, fontSize: T.body, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const, flex: 1 }}>
                    {u.url}
                  </span>
                  {u.type && (
                    <span style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.08em", padding: "2px 6px", borderRadius: 4, background: C.bgDeep, color: C.stone, flexShrink: 0 }}>
                      {u.type}
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}

          {view === "logs" && (
            <div>
              <div style={{ display: "flex", alignItems: "center", flexWrap: isCompact ? "wrap" : "nowrap", gap: 6, padding: "8px 14px", background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
                {[
                  {
                    key: "errors" as const,
                    label: "Errors",
                    count: errCount,
                    accent: "var(--color-red-700)",
                    tagBg: "color-mix(in oklch, var(--color-red-700) 12%, transparent)",
                    chipBg: "color-mix(in oklch, var(--color-red-700) 5%, transparent)",
                    chipBdr: "color-mix(in oklch, var(--color-red-700) 24%, transparent)",
                    activeBg: "color-mix(in oklch, var(--color-red-700) 10%, transparent)",
                    activeBdr: "color-mix(in oklch, var(--color-red-700) 28%, transparent)",
                  },
                  {
                    key: "warnings" as const,
                    label: "Warnings",
                    count: warnCount,
                    accent: "var(--color-yellow-700)",
                    tagBg: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)",
                    chipBg: "color-mix(in oklch, var(--color-yellow-700) 5%, transparent)",
                    chipBdr: "color-mix(in oklch, var(--color-yellow-700) 24%, transparent)",
                    activeBg: "color-mix(in oklch, var(--color-yellow-700) 10%, transparent)",
                    activeBdr: "color-mix(in oklch, var(--color-yellow-700) 28%, transparent)",
                  },
                ].map((f) => {
                  const active = activeFilters.has(f.key);
                  return (
                    <button
                      key={f.key}
                      onClick={() => toggleFilter(f.key)}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 5,
                        padding: "4px 8px",
                        borderRadius: 6,
                        border: `1px solid ${C.border}`,
                        cursor: "pointer",
                        fontFamily: S.body,
                        fontSize: T.bodySm,
                        transition: "all 0.12s",
                        background: active ? "var(--color-stone-200)" : "transparent",
                        color: f.accent,
                        fontWeight: active ? 500 : 400,
                        whiteSpace: "nowrap" as const,
                      }}
                    >
                      <span>{f.label}</span>
                      <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: f.accent }}>
                        {f.count}
                      </span>
                      {active && <X size={I.xs} style={{ marginLeft: 1, flexShrink: 0 }} />}
                    </button>
                  );
                })}
                <div style={{ flex: 1 }} />
                <Select value={logTimeRange} onValueChange={(value) => setLogTimeRange(value as LogTimeRange)}>
                  <SelectTrigger
                    className="h-8 w-auto min-w-[130px] px-3"
                    style={{ fontFamily: S.body, fontSize: T.bodySm, background: "var(--popover)" }}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LOG_TIME_RANGE_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div style={{ display: "flex", alignItems: "center", gap: 5, height: 32, padding: "0 10px", borderRadius: 6, border: `1px solid ${C.border}`, background: "var(--popover)" }}>
                  <Search size={I.sm} color={C.faint} />
                  <input
                    type="text"
                    placeholder="Search logs"
                    value={logSearch}
                    onChange={(e) => setLogSearch(e.target.value)}
                    style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: T.bodySm, color: C.muted, width: isCompact ? 92 : 160, caretColor: C.tealMid }}
                  />
                </div>
                <button
                  type="button"
                  title="Refresh logs"
                  onClick={() => {
                    void refetch({ cancelRefetch: true });
                  }}
                  style={{
                    background: "none",
                    border: `1px solid ${C.border}`,
                    cursor: "pointer",
                    width: 32,
                    height: 32,
                    padding: 0,
                    borderRadius: 5,
                    color: C.text,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    opacity: 1,
                  }}
                >
                  <RefreshCw size={I.sm} className={isFetching ? "dp-spin" : undefined} />
                </button>
                <button
                  type="button"
                  title="Copy logs"
                  onClick={() => {
                    void handleCopyLogs();
                  }}
                  style={{
                    background: "none",
                    border: `1px solid ${C.border}`,
                    cursor: "pointer",
                    width: 32,
                    height: 32,
                    padding: 0,
                    borderRadius: 5,
                    color: C.text,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  {copiedLogs ? <Check size={I.sm} color={C.tealMid} /> : <Copy size={I.sm} />}
                </button>
              </div>
              <div style={{ background: "var(--color-stone-50)", padding: "10px 0 14px" }}>
                {isLoading ? (
                  <div style={{ padding: "12px 18px", display: "flex", alignItems: "center", gap: 8, fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
                    <Loader2 size={I.md} className="dp-spin" />
                    Loading logs…
                  </div>
                ) : logErrorMessage ? (
                  <div style={{ padding: "12px 18px", fontFamily: S.mono, fontSize: T.monoSm, color: C.coral, lineHeight: 1.5 }}>
                    {logErrorMessage}
                  </div>
                ) : filtered.length === 0 ? (
                  <div style={{ padding: "12px 18px", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
                    {logs.length === 0 ? "No log lines in this time window" : "No matching lines"}
                  </div>
                ) : (
                  filtered.map((line, li) => {
                    const parsed = splitLogLineTimestamp(line);
                    return (
                      <div key={li} className="dp-log" style={{ display: "flex", alignItems: "baseline", padding: "1px 0" }}>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.stone, minWidth: 44, textAlign: "right" as const, paddingRight: 12, flexShrink: 0, userSelect: "none" as const }}>
                          {li + 1}
                        </span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, minWidth: isCompact ? 128 : 190, paddingRight: 12, flexShrink: 0 }}>
                          {formatLogTimestamp(parsed.timestamp)}
                        </span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoMd, color: logLineColor(line), lineHeight: 1.75, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                          {parsed.message}
                        </span>
                      </div>
                    );
                  })
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "—";
  const sec = Math.max(0, Math.round(ms / 1000));
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function resolveDeployedAtMs(h: ApiDeploymentHistoryRecord, live: AgentDeployment): number {
  const fromHist = new Date(h.deployed_at).getTime();
  if (h.id === live.id) {
    const fromLive = new Date(live.created_at).getTime();
    if (!Number.isFinite(fromHist) || Number.isNaN(fromHist)) return fromLive;
    return fromHist;
  }
  return fromHist;
}

function deploymentHistoryDurationMs(
  h: ApiDeploymentHistoryRecord,
  idx: number,
  merged: ApiDeploymentHistoryRecord[],
  live: AgentDeployment,
  isCurrent: boolean,
): number | null {
  const start = resolveDeployedAtMs(h, live);
  if (!Number.isFinite(start) || Number.isNaN(start)) return null;
  if (isCurrent) return Date.now() - start;
  if (h.undeployed_at) {
    const end = new Date(h.undeployed_at).getTime();
    if (!Number.isFinite(end) || Number.isNaN(end)) return null;
    return end - start;
  }
  if (idx > 0) {
    const end = resolveDeployedAtMs(merged[idx - 1], live);
    if (!Number.isFinite(end) || Number.isNaN(end)) return null;
    return end - start;
  }
  return null;
}

function deploymentHistoryUiStatus(h: ApiDeploymentHistoryRecord, live: AgentDeployment): DeployHistoryStatus {
  if (h.undeployed_at) return "undeployed";
  if (h.id === live.id) {
    const ds = mapDeploymentStatus(live);
    if (ds === "error") return "failed";
    if (ds === "undeploying") return "undeploying";
    if (ds === "pending") return "deploying";
    return "active";
  }
  // Historical (non-current) rows should appear as INACTIVE unless explicitly undeployed.
  return "ready";
}

function statusColor(status: DeployHistoryStatus): string {
  if (status === "failed") return C.coral;
  if (status === "undeployed") return C.stone;
  if (status === "deploying") return C.warning;
  if (status === "undeploying") return C.faint;
  if (status === "active") return C.tealMid;
  return C.faint;
}

function statusLabel(status: DeployHistoryStatus): string {
  if (status === "active") return "Live";
  if (status === "ready") return "Inactive";
  if (status === "deploying") return "Deploying";
  if (status === "undeploying") return "Undeploying";
  if (status === "failed") return "Failed";
  return "Undeployed";
}

export function DeploymentsTab({
  deployment,
  account,
  onOpenConfigure,
}: {
  deployment: AgentDeployment;
  account: string;
  onOpenConfigure?: () => void;
}) {
  const queryClient = useQueryClient();
  const [isCompact, setIsCompact] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return window.innerWidth < 1180;
  });
  const [showAllHistory, setShowAllHistory] = useState(false);
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set());
  const [openPastDeployMenu, setOpenPastDeployMenu] = useState<string | null>(null);
  const hasAutoOpenedOverview = useRef(false);

  const { data: historyData, isLoading: historyLoading, isError: historyError } = useDeploymentHistory(account, deployment.name);

  useEffect(() => {
    if (!isDeployingState(deployment)) return;
    const interval = setInterval(() => {
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    }, 4000);
    return () => clearInterval(interval);
  }, [account, deployment, queryClient]);

  const serviceRows = useMemo(() => {
    const externalUrls = deployment.external_urls ?? [];
    const primaryUrl = externalUrls[0]?.url;
    return (deployment.workloads ?? []).map((wl) => {
      const mappedContainers = (wl.containers ?? []).map((c) => ({
        name: c.name,
        ready: c.ready,
        vars: (c.env ?? [])
          .map((e) => {
            const val = e.value ?? "";
            return {
              key: e.name,
              value: val,
              source: e.from ?? "static",
              secret: isSensitiveEnvVar(e.name, val, e.from ?? "static"),
            };
          }),
      }));
      const readyCount = mappedContainers.filter((c) => c.ready).length;
      const url = primaryUrl && wl.component === "agent" ? primaryUrl : undefined;
      return {
        id: wl.name,
        workloadName: wl.name,
        title: wl.component || wl.name,
        isAgentService: wl.component === "agent",
        readyText: `${readyCount}/${mappedContainers.length || 0}`,
        uptime: wl.age ?? "—",
        containers: mappedContainers,
        url,
        urls: wl.urls,
      };
    });
  }, [deployment]);

  const totalServiceCount = useMemo(() => serviceRows.length, [serviceRows]);

  const allRows = useMemo((): DeploymentHistoryTableRow[] => {
    const fromApi = historyData?.deployments ?? [];
    const seen = new Set(fromApi.map((h) => h.id));
    const merged: ApiDeploymentHistoryRecord[] = [...fromApi];
    if (!seen.has(deployment.id)) {
      merged.unshift({
        id: deployment.id,
        agent_name: deployment.name,
        build_id: deployment.build_id,
        namespace: deployment.namespace,
        status: deployment.status,
        deployed_at: deployment.created_at,
        spec: {},
      });
    }
    merged.sort((a, b) => resolveDeployedAtMs(b, deployment) - resolveDeployedAtMs(a, deployment));

    const rows: DeploymentHistoryTableRow[] = merged.map((h, idx) => {
      const isCurrent = h.id === deployment.id;
      const status = deploymentHistoryUiStatus(h, deployment);
      const build = h.build_id?.slice(0, 8) || "—";
      const rowLabel = isCurrent ? deployment.display_name || deployment.name : `${deployment.name} · ${build}`;
      const durMs = deploymentHistoryDurationMs(h, idx, merged, deployment, isCurrent);
      const deployedAtIso = new Date(resolveDeployedAtMs(h, deployment)).toISOString();
      return {
        id: h.id,
        status,
        build,
        duration: durMs !== null ? formatDurationMs(durMs) : "—",
        time: formatDate(deployedAtIso),
        isCurrent,
        rowLabel,
        source: h,
      };
    });

    return rows;
  }, [historyData, deployment]);
  const pastRows = useMemo(() => allRows.filter((row) => !row.isCurrent), [allRows]);
  const currentRow = useMemo(() => allRows.find((row) => row.isCurrent) ?? null, [allRows]);

  useEffect(() => {
    hasAutoOpenedOverview.current = false;
    setOpenContainers(new Set());
  }, [deployment.id]);

  useEffect(() => {
    if (serviceRows.length === 0) return;
    if (hasAutoOpenedOverview.current) return;
    hasAutoOpenedOverview.current = true;
    setOpenContainers(new Set([serviceRows[0].id]));
  }, [serviceRows]);

  useEffect(() => {
    const onResize = () => setIsCompact(window.innerWidth < 1180);
    onResize();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  const deploymentGridColumns = isCompact
    ? "minmax(0, 1.35fr) minmax(0, 0.78fr) minmax(0, 0.65fr) minmax(0, 0.78fr)"
    : DEPLOYMENT_GRID_COLUMNS;
  const deploymentGridGap = isCompact ? 8 : 12;
  const deploymentHeaderPadding = isCompact ? "8px 10px" : "8px 14px";
  const deploymentRowPadding = isCompact ? "10px 10px" : "11px 14px";
  const deploymentGridHeaders = isCompact
    ? ["Deployment", "Status", "Duration", "Build No."]
    : ["Deployment", "Status", "Duration", "Build No.", "Deployed on", ""];
  const hasCollapsedHistory = pastRows.length > 4;
  const visiblePastRows = showAllHistory ? pastRows : pastRows.slice(0, 4);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ fontFamily: S.body, fontSize: T.heading1, fontWeight: 600, color: C.text, flex: 1 }}>Deployments</span>
        </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, minmax(0, 1fr))", gap: 10 }}>
              {[
                { label: "CURRENT BUILD", value: deployment.build_id?.slice(0, 8) || "—", wrap: false, valueColor: C.text },
                {
                  label: "DEPLOYMENT STATUS",
                  value: String(deployment.status || "unknown").charAt(0).toUpperCase() + String(deployment.status || "unknown").slice(1).toLowerCase(),
                  wrap: false,
                  valueColor: C.text,
                },
                {
                  label: "DEPLOYED ON",
                  value: deployment.created_at
                    ? `${formatDate(deployment.created_at)},${isCompact ? "\n" : " "}${new Date(deployment.created_at).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`
                    : "—",
                  wrap: true,
                  valueColor: C.text,
                },
                { label: "SERVICES", value: String(totalServiceCount), wrap: false, valueColor: C.text },
              ].map((item) => (
                <div key={item.label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px", minWidth: 0 }}>
                  <span style={{ display: "block", fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, marginBottom: 8 }}>
                    {item.label}
                  </span>
                  <span
                    style={{
                      display: "block",
                      fontFamily: S.body,
                      fontSize: T.heading4,
                      fontWeight: 600,
                      color: item.valueColor,
                      overflow: "hidden",
                      textOverflow: item.wrap ? "clip" : "ellipsis",
                      whiteSpace: item.wrap ? (isCompact ? ("pre-line" as const) : ("nowrap" as const)) : ("nowrap" as const),
                      lineHeight: item.wrap ? 1.25 : undefined,
                    }}
                  >
                    {item.value}
                  </span>
                </div>
              ))}
            </div>

            <div>
              <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden", maxWidth: "100%" }}>
              <div style={{ display: "grid", gridTemplateColumns: deploymentGridColumns, gap: deploymentGridGap, padding: deploymentHeaderPadding, borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
                {deploymentGridHeaders.map((h, i) => (
                  <span key={h} style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, textAlign: i === 0 ? "left" : "right", whiteSpace: "nowrap", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>
                    {h.toUpperCase()}
                  </span>
                ))}
              </div>
              {currentRow ? (
                <>
                  <div
                    style={{
                      display: "grid",
                      gridTemplateColumns: deploymentGridColumns,
                      gap: deploymentGridGap,
                      padding: isCompact ? "11px 10px" : "12px 14px",
                      alignItems: "center",
                      borderLeft: `3px solid ${C.tealMid}`,
                    }}
                  >
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={currentRow.rowLabel}>
                        {currentRow.rowLabel}
                      </div>
                    </div>
                    <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 5 }}>
                      {currentRow.status === "deploying" || currentRow.status === "undeploying"
                        ? <Loader2 size={I.sm} style={{ color: statusColor(currentRow.status), animation: "dp-spin 1.2s linear infinite" }} />
                        : <span style={{ width: 5, height: 5, borderRadius: "50%", background: statusColor(currentRow.status), flexShrink: 0, display: "inline-block" }} />}
                      <span style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.06em", color: statusColor(currentRow.status), fontWeight: 500 }}>
                        {statusLabel(currentRow.status).toUpperCase()}
                      </span>
                    </div>
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, textAlign: "right" as const }}>{currentRow.duration}</span>
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, fontWeight: 400, color: C.text, textAlign: "right" as const }}>{currentRow.build}</span>
                    {!isCompact ? (
                      <>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, whiteSpace: "nowrap" as const, textAlign: "right" as const }}>
                          {currentRow.time}
                        </span>
                        <span />
                      </>
                    ) : null}
                  </div>

                  <div style={{ padding: "8px 16px 16px", borderTop: `1px solid ${C.border}`, background: C.bg }}>
                    <div style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, textTransform: "uppercase" as const, margin: "6px 0 10px", display: "flex", alignItems: "center", gap: 6 }}>
                      Services
                      {serviceRows.length > 0 && (
                        <InlineBadge variant="fill" shape="square" className="normal-case size-[18px] p-0 justify-center text-muted-foreground text-[11px]">
                          {serviceRows.length}
                        </InlineBadge>
                      )}
                    </div>
                    {serviceRows.length === 0 ? (
                      <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
                        {currentRow.status === "deploying" || currentRow.status === "undeploying" ? (
                          <Loader2
                            size={I.md}
                            style={{
                              animation: "dp-spin 1.2s linear infinite",
                              color: currentRow.status === "deploying" ? C.warning : C.faint,
                            }}
                          />
                        ) : null}
                        {currentRow.status === "deploying"
                          ? "Waiting for services to start and logs to stream…"
                          : currentRow.status === "undeploying"
                            ? "Tearing down services and streaming final logs…"
                            : "No service data available"}
                      </p>
                    ) : (
                      serviceRows.map((svc) => (
                        <ActiveContainerAccordion
                          key={svc.id}
                          workloadName={svc.workloadName}
                          title={svc.title}
                          isCompact={isCompact}
                          isAgentService={svc.isAgentService}
                          url={svc.url}
                          urls={svc.urls}
                          readyText={svc.readyText}
                          uptime={svc.uptime}
                          deploymentId={deployment.id}
                          containers={svc.containers}
                          deploymentStatus={currentRow.status}
                          isOpen={openContainers.has(svc.id)}
                          onToggle={() =>
                            setOpenContainers((prev: Set<string>) => {
                              const n = new Set(prev);
                              if (n.has(svc.id)) n.delete(svc.id);
                              else n.add(svc.id);
                              return n;
                            })
                          }
                        />
                      ))
                    )}
                  </div>
                </>
              ) : (
                <div style={{ padding: "20px 16px", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>No active deployment found.</div>
              )}
              {(historyLoading || historyError || pastRows.length > 0) && <div style={{ borderTop: `1px solid ${C.border}`, background: C.bgAlt }}>
                {historyError && (
                  <div style={{ padding: "12px 14px" }}>
                    <ErrorPanel title="Unable to load deployment history" dismissible>
                      Could not load deployment history from the server.
                    </ErrorPanel>
                  </div>
                )}
                {historyLoading ? (
                  <div style={{ padding: "14px", display: "flex", alignItems: "center", gap: 8, fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
                    <Loader2 size={I.md} className="dp-spin" />
                    Loading deployment history…
                  </div>
                ) : pastRows.length === 0 ? null : (
                  <>
                    {visiblePastRows.map((row, idx) => (
                      <div
                        key={row.id}
                        style={{
                          display: "grid",
                          gridTemplateColumns: deploymentGridColumns,
                          gap: deploymentGridGap,
                          padding: deploymentRowPadding,
                          alignItems: "center",
                          borderBottom: idx < visiblePastRows.length - 1 ? `1px solid ${C.border}` : "none",
                        }}
                      >
                        <div style={{ minWidth: 0 }}>
                          <div style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={row.rowLabel}>
                            {row.rowLabel}
                          </div>
                        </div>
                        <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 5 }}>
                          {row.status === "deploying" || row.status === "undeploying"
                            ? <Loader2 size={I.sm} style={{ color: statusColor(row.status), animation: "dp-spin 1.2s linear infinite" }} />
                            : <span style={{ width: 5, height: 5, borderRadius: "50%", background: statusColor(row.status), flexShrink: 0, display: "inline-block" }} />}
                          <span style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.06em", color: statusColor(row.status), fontWeight: 500 }}>
                            {statusLabel(row.status).toUpperCase()}
                          </span>
                        </div>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, textAlign: "right" as const }}>{row.duration}</span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, fontWeight: 400, color: C.text, textAlign: "right" as const }}>{row.build}</span>
                        {!isCompact ? (
                          <>
                            <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, whiteSpace: "nowrap" as const, textAlign: "right" as const }}>
                              {row.time}
                            </span>
                            <div style={{ position: "relative" }}>
                              <button
                                type="button"
                                onClick={() => setOpenPastDeployMenu((prev) => (prev === row.id ? null : row.id))}
                                style={{ background: "none", border: "none", cursor: "pointer", color: C.text, display: "flex", padding: 4, borderRadius: 4 }}
                                onMouseEnter={(e) => {
                                  e.currentTarget.style.background = C.bgDeep;
                                }}
                                onMouseLeave={(e) => {
                                  e.currentTarget.style.background = "none";
                                }}
                                aria-label={`Actions for deployment ${row.id}`}
                              >
                                <MoreVertical size={I.md} />
                              </button>
                              {openPastDeployMenu === row.id && (
                                <>
                                  <div onClick={() => setOpenPastDeployMenu(null)} style={{ position: "fixed", inset: 0, zIndex: 10 }} />
                                  <div
                                    style={{
                                      position: "absolute",
                                      right: 0,
                                      top: "calc(100% + 4px)",
                                      zIndex: 20,
                                      minWidth: 150,
                                      background: C.bgAlt,
                                      border: `1px solid ${C.border}`,
                                      borderRadius: 8,
                                      overflow: "hidden",
                                      boxShadow: "0 6px 20px rgba(0,0,0,0.1)",
                                    }}
                                  >
                                    <button
                                      type="button"
                                      onClick={() => {
                                        setOpenPastDeployMenu(null);
                                        onOpenConfigure?.();
                                      }}
                                      style={{
                                        width: "100%",
                                        display: "flex",
                                        alignItems: "center",
                                        gap: 8,
                                        padding: "9px 14px",
                                        background: "none",
                                        border: "none",
                                        cursor: "pointer",
                                        fontFamily: S.body,
                                        fontSize: T.body,
                                        color: C.text,
                                        textAlign: "left" as const,
                                      }}
                                      onMouseEnter={(e) => {
                                        e.currentTarget.style.background = C.bgDeep;
                                      }}
                                      onMouseLeave={(e) => {
                                        e.currentTarget.style.background = "none";
                                      }}
                                    >
                                      Rollback
                                    </button>
                                  </div>
                                </>
                              )}
                            </div>
                          </>
                        ) : null}
                      </div>
                    ))}
                    {hasCollapsedHistory && (
                      <div style={{ display: "flex", justifyContent: "center", padding: "8px 12px 10px", borderTop: `1px solid ${C.border}` }}>
                        <button
                          type="button"
                          onClick={() => setShowAllHistory((prev) => !prev)}
                          style={{
                            background: "none",
                            border: "none",
                            cursor: "pointer",
                            fontFamily: S.mono,
                            fontSize: T.monoSm,
                            letterSpacing: "0.04em",
                            color: C.faint,
                            textDecoration: "underline",
                          }}
                        >
                          {showAllHistory ? "See less" : `See more (${pastRows.length - 4} more)`}
                        </button>
                      </div>
                    )}
                  </>
                )}
              </div>}
              </div>
            </div>
          </div>
    </div>
  );
}
