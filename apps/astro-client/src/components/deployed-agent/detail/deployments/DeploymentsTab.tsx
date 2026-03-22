import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Search, Loader2, X, Eye, EyeOff, RefreshCw, Copy, Check, MoreVertical } from "lucide-react";
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
  amber: "var(--color-amber-700)",
  amberBg: "color-mix(in oklch, var(--color-amber-700) 12%, transparent)",
  amberBdr: "color-mix(in oklch, var(--color-amber-700) 28%, transparent)",
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

export interface ActiveContainerAccordionProps {
  podName: string;
  title: string;
  url?: string;
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
  if (/warn|warning|retry|attempt/.test(l)) return C.amber;
  return C.muted;
}

function splitLogLineTimestamp(line: string): { timestamp: string | null; message: string } {
  const iso = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$/);
  if (iso) return { timestamp: iso[1], message: iso[2] };
  const basic = line.match(/^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+(.*)$/);
  if (basic) return { timestamp: basic[1], message: basic[2] };
  return { timestamp: null, message: line };
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

function isSensitiveEnvVar(key: string, value: string): boolean {
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

function isReplicaSetHash(segment: string): boolean {
  return /^[a-z0-9]{8,10}$/.test(segment);
}

function isPodSuffix(segment: string): boolean {
  return /^[a-z0-9]{5}$/.test(segment);
}

function displayPodTitle(podName: string, deploymentName: string): string {
  const raw = podName.startsWith(`${deploymentName}-`) ? podName.slice(deploymentName.length + 1) : podName;
  const parts = raw.split("-");
  if (parts.length >= 3 && isReplicaSetHash(parts[parts.length - 2]) && isPodSuffix(parts[parts.length - 1])) {
    return parts.slice(0, -2).join("-");
  }
  if (parts.length >= 2 && isPodSuffix(parts[parts.length - 1])) {
    return parts.slice(0, -1).join("-");
  }
  return raw;
}

export function ActiveContainerAccordion({
  podName,
  title,
  url,
  readyText,
  uptime,
  containers,
  deploymentId,
  deploymentStatus,
  isOpen,
  onToggle,
}: ActiveContainerAccordionProps) {
  const [view, setView] = useState<"logs" | "vars">("logs");
  const [revealed, setRevealed] = useState<Set<string>>(new Set());
  const [logSearch, setLogSearch] = useState("");
  const [logTimeRange, setLogTimeRange] = useState<LogTimeRange>("24h");
  const [activeFilters, setActiveFilters] = useState<Set<"errors" | "warnings">>(new Set());
  const [copiedUrl, setCopiedUrl] = useState(false);
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
  const totalContainers = containers.length;
  const readyContainers = containers.filter((container) => container.ready).length;
  const allReady = totalContainers > 0 && readyContainers === totalContainers;

  useEffect(() => {
    if (!canShowVars && view === "vars") {
      setView("logs");
    }
  }, [canShowVars, view]);

  const { data: logsRaw, isLoading, isFetching, error, refetch } = useDeploymentLogs(
    deploymentId,
    podName,
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

  const toggleReveal = (key: string) =>
    setRevealed((prev) => {
      const n = new Set(prev);
      if (n.has(key)) n.delete(key);
      else n.add(key);
      return n;
    });

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

  const handleCopyUrl = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!url) return;
    const ok = await copyTextToClipboard(url);
    if (!ok) return;
    setCopiedUrl(true);
    setTimeout(() => setCopiedUrl(false), 900);
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
      <button
        className="dp-container-hdr"
        onClick={onToggle}
        style={{
          display: "flex",
          alignItems: "center",
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
        onMouseEnter={(e) => {
          if (!isOpen) e.currentTarget.style.background = C.panel;
        }}
        onMouseLeave={(e) => {
          if (!isOpen) e.currentTarget.style.background = C.bg;
        }}
      >
        <ChevronRight size={I.md} color={C.faint} style={{ flexShrink: 0, transform: isOpen ? "rotate(90deg)" : "none", transition: "transform 0.18s" }} />
        {deploymentStatus === "deploying" || deploymentStatus === "undeploying" ? (
          <Loader2 size={16} style={{ color: C.amber, animation: "dp-spin 1.2s linear infinite", flexShrink: 0 }} />
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
        <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 500, color: C.text }} title={podName}>
          {title}
        </span>
        <span style={{ flex: 1 }} />
        {url && (
          <button
            onClick={handleCopyUrl}
            style={{
              position: "relative",
              padding: "3px 10px",
              borderRadius: 5,
              border: `1px solid ${C.border}`,
              background: "transparent",
              cursor: "pointer",
              flexShrink: 0,
              fontFamily: S.mono,
              fontSize: T.label,
              color: C.stone,
              transition: "background 0.15s",
              maxWidth: 280,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap" as const,
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.background = C.bgDeep;
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = "transparent";
            }}
          >
            {url}
            {copiedUrl && (
              <span
                style={{
                  position: "absolute",
                  right: 6,
                  top: "50%",
                  transform: "translateY(-50%)",
                  color: C.tealMid,
                  display: "flex",
                  alignItems: "center",
                  gap: 4,
                  fontFamily: S.body,
                  fontSize: T.monoSm,
                  whiteSpace: "nowrap" as const,
                  background: "inherit",
                  paddingLeft: 4,
                }}
              >
                <Check size={I.xs} />
                Copied
              </span>
            )}
          </button>
        )}
        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, flexShrink: 0, marginLeft: 8 }}>
          {readyText} ready · {uptime}
        </span>
      </button>

      {isOpen && (
        <div style={{ border: `1px solid ${C.border}`, borderTop: "none", borderRadius: "0 0 8px 8px", overflow: "hidden" }}>
          <div style={{ display: "flex", background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
            {(["logs", "vars"] as const).map((v) => (
              v === "vars" && !canShowVars ? null : (
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
                  fontWeight: view === v ? 600 : 400,
                  color: view === v ? C.text : C.faint,
                  borderBottom: view === v ? `2px solid ${C.tealMid}` : "2px solid transparent",
                  transition: "color 0.12s",
                  textTransform: "capitalize" as const,
                }}
              >
                {v === "vars" ? "Variables" : "Logs"}
              </button>
              )
            ))}
          </div>

          {view === "vars" && (
            <div style={{ background: C.bg }}>
              {vars.length === 0 ? (
                <div style={{ padding: "16px", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>No variables</div>
              ) : (
                vars.map((v, vi) => {
                  const isRevealed = revealed.has(v.key);
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
                      <span style={{ fontFamily: S.mono, fontSize: T.monoMd, color: C.text, minWidth: 160, flexShrink: 0 }}>
                        {v.key}
                      </span>
                      <div style={{ flex: 1, display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoMd, color: C.faint, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }}>
                          {isSecret && !isRevealed ? "•••••••••" : v.value}
                        </span>
                        {isSecret && (
                          <button onClick={() => toggleReveal(v.key)} style={{ background: "none", border: "none", cursor: "pointer", color: C.stone, display: "flex", padding: 2, flexShrink: 0 }}>
                            {isRevealed ? <EyeOff size={I.md} /> : <Eye size={I.md} />}
                          </button>
                        )}
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

          {view === "logs" && (
            <div>
              <div style={{ display: "flex", alignItems: "center", gap: 6, padding: "8px 14px", background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
                {[
                  { key: "errors" as const, label: `Errors (${errCount})`, accent: "#dc2626", activeBg: "#fef2f2", activeBdr: "#fca5a5" },
                  { key: "warnings" as const, label: `Warnings (${warnCount})`, accent: "#d97706", activeBg: "#fffbeb", activeBdr: "#fcd34d" },
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
                        border: `1px solid ${active ? f.activeBdr : C.border}`,
                        cursor: "pointer",
                        fontFamily: S.body,
                        fontSize: T.bodySm,
                        transition: "all 0.12s",
                        background: active ? f.activeBg : "transparent",
                        color: active ? f.accent : C.muted,
                        fontWeight: active ? 500 : 400,
                        whiteSpace: "nowrap" as const,
                      }}
                    >
                      {f.label}
                      {active && <X size={I.xs} style={{ marginLeft: 2, flexShrink: 0 }} />}
                    </button>
                  );
                })}
                <div style={{ flex: 1 }} />
                {containers.length > 1 && (
                  <Select value={selectedContainer} onValueChange={setSelectedContainer}>
                    <SelectTrigger
                      className="h-8 w-auto min-w-[130px] px-3"
                      style={{ fontFamily: S.body, fontSize: T.bodySm, color: C.muted }}
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
                )}
                <Select value={logTimeRange} onValueChange={(value) => setLogTimeRange(value as LogTimeRange)}>
                  <SelectTrigger
                    className="h-8 w-auto min-w-[130px] px-3"
                    style={{ fontFamily: S.body, fontSize: T.bodySm, color: C.muted }}
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
                <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "4px 10px", borderRadius: 6, border: `1px solid ${C.border}`, background: C.bg }}>
                  <Search size={I.sm} color={C.faint} />
                  <input
                    type="text"
                    placeholder="Find in logs"
                    value={logSearch}
                    onChange={(e) => setLogSearch(e.target.value)}
                    style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: T.bodySm, color: C.muted, width: 120, caretColor: C.tealMid }}
                  />
                </div>
                <button
                  type="button"
                  title="Refresh logs"
                  onClick={() => {
                    void refetch({ cancelRefetch: false });
                  }}
                  style={{
                    background: "none",
                    border: `1px solid ${C.border}`,
                    cursor: "pointer",
                    padding: "4px 6px",
                    borderRadius: 5,
                    color: C.faint,
                    display: "flex",
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
                  style={{ background: "none", border: `1px solid ${C.border}`, cursor: "pointer", padding: "4px 6px", borderRadius: 5, color: C.faint, display: "flex" }}
                >
                  {copiedLogs ? <Check size={I.sm} color={C.tealMid} /> : <Copy size={I.sm} />}
                </button>
              </div>
              <div style={{ background: C.panel, padding: "10px 0 14px" }}>
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
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, minWidth: 190, paddingRight: 12, flexShrink: 0 }}>
                          {parsed.timestamp ?? "—"}
                        </span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoMd, color: logLineColor(line), lineHeight: 1.75 }}>
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
  // Historical (non-current) rows should appear as READY unless explicitly undeployed.
  return "ready";
}

function statusColor(status: DeployHistoryStatus): string {
  if (status === "failed") return C.coral;
  if (status === "undeployed") return C.stone;
  if (status === "deploying") return C.amber;
  if (status === "undeploying") return C.faint;
  return C.success;
}

function statusLabel(status: DeployHistoryStatus): string {
  if (status === "active") return "Live";
  if (status === "ready") return "Ready";
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

  const podRows = useMemo(() => {
    const externalUrls = deployment.external_urls ?? [];
    const primaryUrl = externalUrls[0]?.url;
    return (deployment.pods ?? []).map((pod) => {
      const mappedContainers = (pod.containers ?? []).map((c) => ({
        name: c.name,
        ready: c.ready,
        vars: (c.env ?? [])
          .filter((e) => {
            const key = (e.name ?? "").trim();
            return key !== "*" && !key.endsWith("*");
          })
          .map((e) => {
            const val = e.value ?? "";
            return {
              key: e.name,
              value: val,
              secret: isSensitiveEnvVar(e.name, val),
              source: e.from ?? "static",
            };
          }),
      }));
      const readyCount = mappedContainers.filter((c) => c.ready).length;
      const title = displayPodTitle(pod.name, deployment.name);
      const url = primaryUrl && pod.name.includes("-agent-") ? primaryUrl : undefined;
      return {
        id: pod.name,
        podName: pod.name,
        title,
        readyText: `${readyCount}/${mappedContainers.length || 0}`,
        uptime: pod.age ?? "—",
        containers: mappedContainers,
        url,
      };
    });
  }, [deployment]);

  const totalPodCount = useMemo(() => podRows.length, [podRows]);

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
    if (podRows.length === 0) return;
    if (hasAutoOpenedOverview.current) return;
    hasAutoOpenedOverview.current = true;
    setOpenContainers(new Set([podRows[0].id]));
  }, [podRows]);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ fontFamily: S.body, fontSize: T.heading1, fontWeight: 600, color: C.text, flex: 1 }}>Deployments</span>
        </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
              {[
                { label: "CURRENT BUILD", value: deployment.build_id?.slice(0, 8) || "—" },
                { label: "DEPLOYMENT STATUS", value: String(deployment.status || "unknown").toUpperCase() },
                { label: "DEPLOYED", value: deployment.created_at ? new Date(deployment.created_at).toLocaleString() : "—" },
                { label: "PODS", value: String(totalPodCount) },
              ].map((item) => (
                <div key={item.label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px" }}>
                  <span style={{ display: "block", fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, marginBottom: 8 }}>
                    {item.label}
                  </span>
                  <span style={{ display: "block", fontFamily: S.body, fontSize: T.heading4, fontWeight: 600, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {item.value}
                  </span>
                </div>
              ))}
            </div>

            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "visible" }}>
              <div style={{ display: "grid", gridTemplateColumns: "minmax(200px, 1fr) 88px 84px 116px 116px 28px", gap: 12, padding: "8px 14px", borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
                {["Deployment", "Status", "Duration", "Build No.", "Deployed on", ""].map((h, i) => (
                  <span key={h} style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, textAlign: i >= 2 ? "right" : "left", whiteSpace: "nowrap" }}>
                    {h.toUpperCase()}
                  </span>
                ))}
              </div>
              {currentRow ? (
                <>
                  <div
                    style={{
                      display: "grid",
                      gridTemplateColumns: "minmax(200px, 1fr) 88px 84px 116px 116px 28px",
                      gap: 12,
                      padding: "12px 14px",
                      alignItems: "center",
                      borderLeft: `3px solid ${C.tealMid}`,
                      background: "rgba(21,130,125,0.02)",
                    }}
                  >
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={currentRow.rowLabel}>
                        {currentRow.rowLabel}
                      </div>
                    </div>
                    <span
                      style={{
                        fontFamily: S.mono,
                        fontSize: T.label,
                        letterSpacing: "0.06em",
                        color: statusColor(currentRow.status),
                        fontWeight: 500,
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 6,
                      }}
                    >
                      {currentRow.status === "deploying" || currentRow.status === "undeploying" ? <Loader2 size={I.sm} style={{ animation: "dp-spin 1.2s linear infinite" }} /> : null}
                      {statusLabel(currentRow.status).toUpperCase()}
                    </span>
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, textAlign: "right" as const }}>{currentRow.duration}</span>
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, fontWeight: 600, color: C.text, textAlign: "right" as const }}>{currentRow.build}</span>
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, whiteSpace: "nowrap" as const, textAlign: "right" as const }}>
                      {currentRow.time}
                    </span>
                    <span />
                  </div>

                  <div style={{ padding: "8px 16px 16px", borderTop: `1px solid ${C.border}`, background: C.bg }}>
                    <div style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, margin: "6px 0 10px" }}>
                      Pods
                    </div>
                    {podRows.length === 0 ? (
                      <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, margin: 0, display: "flex", alignItems: "center", gap: 8 }}>
                        {currentRow.status === "deploying" || currentRow.status === "undeploying" ? <Loader2 size={I.md} style={{ animation: "dp-spin 1.2s linear infinite" }} /> : null}
                        {currentRow.status === "deploying"
                          ? "Waiting for pods to start and logs to stream…"
                          : currentRow.status === "undeploying"
                            ? "Tearing down pods and streaming final logs…"
                            : "No pod data available"}
                      </p>
                    ) : (
                      podRows.map((pod) => (
                        <ActiveContainerAccordion
                          key={pod.id}
                          podName={pod.podName}
                          title={pod.title}
                          url={pod.url}
                          readyText={pod.readyText}
                          uptime={pod.uptime}
                          deploymentId={deployment.id}
                          containers={pod.containers}
                          deploymentStatus={currentRow.status}
                          isOpen={openContainers.has(pod.id)}
                          onToggle={() =>
                            setOpenContainers((prev: Set<string>) => {
                              const n = new Set(prev);
                              if (n.has(pod.id)) n.delete(pod.id);
                              else n.add(pod.id);
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
              <div style={{ borderTop: `1px solid ${C.border}`, background: C.bgAlt }}>
                {historyError && (
                  <div style={{ padding: "14px", fontFamily: S.mono, fontSize: T.monoSm, color: C.coral }}>
                    Could not load deployment history from the server.
                  </div>
                )}
                {historyLoading ? (
                  <div style={{ padding: "14px", display: "flex", alignItems: "center", gap: 8, fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
                    <Loader2 size={I.md} className="dp-spin" />
                    Loading deployment history…
                  </div>
                ) : pastRows.length === 0 ? null : (
                  <>
                    {pastRows.map((row, idx) => (
                      <div
                        key={row.id}
                        style={{
                          display: "grid",
                          gridTemplateColumns: "minmax(200px, 1fr) 88px 84px 116px 116px 28px",
                          gap: 12,
                          padding: "11px 14px",
                          alignItems: "center",
                          borderBottom: idx < pastRows.length - 1 ? `1px solid ${C.border}` : "none",
                        }}
                      >
                        <div style={{ minWidth: 0 }}>
                          <div style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={row.rowLabel}>
                            {row.rowLabel}
                          </div>
                        </div>
                        <span
                          style={{
                            fontFamily: S.mono,
                            fontSize: T.label,
                            letterSpacing: "0.06em",
                            color: statusColor(row.status),
                            fontWeight: 500,
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 6,
                          }}
                        >
                          {row.status === "deploying" || row.status === "undeploying" ? <Loader2 size={I.sm} style={{ animation: "dp-spin 1.2s linear infinite" }} /> : null}
                          {statusLabel(row.status).toUpperCase()}
                        </span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, textAlign: "right" as const }}>{row.duration}</span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, fontWeight: 600, color: C.text, textAlign: "right" as const }}>{row.build}</span>
                        <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.text, whiteSpace: "nowrap" as const, textAlign: "right" as const }}>
                          {row.time}
                        </span>
                        <div style={{ position: "relative" }}>
                          <button
                            type="button"
                            onClick={() => setOpenPastDeployMenu((prev) => (prev === row.id ? null : row.id))}
                            style={{ background: "none", border: "none", cursor: "pointer", color: C.faint, display: "flex", padding: 4, borderRadius: 4 }}
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
                                  Redeploy
                                </button>
                              </div>
                            </>
                          )}
                        </div>
                      </div>
                    ))}
                  </>
                )}
              </div>
            </div>
          </div>
    </div>
  );
}
