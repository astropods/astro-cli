import { useMemo, useState } from "react";
import { ChevronRight, MoreVertical, Search, Calendar, Loader2, X, Eye, EyeOff, RefreshCw, Copy, Check } from "lucide-react";
import { useDeploymentLogs, useDeploymentHistory } from "@/api/queries/deployments";
import { formatDate, mapDeploymentStatus } from "@/lib/deployment-utils";
import type { AgentDeployment, ApiError, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { MultiSelect } from "../shared/MultiSelect";
import { C, S } from "../theme";

type LogTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

const LOG_TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: "15m", label: "Last 15 min" },
  { value: "1h", label: "Last 1 hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];

interface ActiveContainerAccordionProps {
  name: string;
  url?: string;
  ready: string;
  uptime: string;
  liveLogs: { deploymentId: string; podName: string; containerName: string };
  vars: { key: string; value: string; secret: boolean; source: string }[];
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

function ActiveContainerAccordion({ name, url, ready, uptime, liveLogs, vars, isOpen, onToggle }: ActiveContainerAccordionProps) {
  const [view, setView] = useState<"logs" | "vars">("logs");
  const [revealed, setRevealed] = useState<Set<string>>(new Set());
  const [logSearch, setLogSearch] = useState("");
  const [logTimeRange, setLogTimeRange] = useState<LogTimeRange>("24h");
  const [activeFilters, setActiveFilters] = useState<Set<"errors" | "warnings">>(new Set());
  const [copiedUrl, setCopiedUrl] = useState(false);

  const { data: logsRaw, isLoading, isFetching, error, refetch } = useDeploymentLogs(
    liveLogs.deploymentId,
    liveLogs.podName,
    liveLogs.containerName,
    logTimeRange,
    { enabled: isOpen },
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

  const handleCopyUrl = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!url) return;
    navigator.clipboard.writeText(url);
    setCopiedUrl(true);
    setTimeout(() => setCopiedUrl(false), 900);
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
        <ChevronRight size={13} color={C.faint} style={{ flexShrink: 0, transform: isOpen ? "rotate(90deg)" : "none", transition: "transform 0.18s" }} />
        <svg width="16" height="16" viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
          <circle cx="12" cy="12" r="10" fill="rgba(21,130,125,0.12)" />
          <path d="M7.5 12l3 3 6-6" stroke={C.tealMid} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
        </svg>
        <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 500, color: C.text }}>{name}</span>
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
              fontSize: 10,
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
                  fontSize: 11,
                  whiteSpace: "nowrap" as const,
                  background: "inherit",
                  paddingLeft: 4,
                }}
              >
                <Check size={10} />
                Copied
              </span>
            )}
          </button>
        )}
        <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, flexShrink: 0, marginLeft: 8 }}>
          {ready} ready · {uptime}
        </span>
      </button>

      {isOpen && (
        <div style={{ border: `1px solid ${C.border}`, borderTop: "none", borderRadius: "0 0 8px 8px", overflow: "hidden" }}>
          <div style={{ display: "flex", background: C.bgAlt, borderBottom: `1px solid ${C.border}` }}>
            {(["logs", "vars"] as const).map((v) => (
              <button
                key={v}
                onClick={() => setView(v)}
                style={{
                  padding: "7px 14px",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  fontFamily: S.body,
                  fontSize: 12,
                  fontWeight: view === v ? 600 : 400,
                  color: view === v ? C.text : C.faint,
                  borderBottom: view === v ? `2px solid ${C.tealMid}` : "2px solid transparent",
                  transition: "color 0.12s",
                  textTransform: "capitalize" as const,
                }}
              >
                {v === "vars" ? "Variables" : "Logs"}
              </button>
            ))}
          </div>

          {view === "vars" && (
            <div style={{ background: C.bg }}>
              {vars.length === 0 ? (
                <div style={{ padding: "16px", fontFamily: S.mono, fontSize: 11, color: C.faint }}>No variables</div>
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
                      <span style={{ fontFamily: S.mono, fontSize: 10, color: C.stone, flexShrink: 0, userSelect: "none" as const }}>
                        {"{}"}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: C.text, minWidth: 160, flexShrink: 0 }}>
                        {v.key}
                      </span>
                      <div style={{ flex: 1, display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                        <span style={{ fontFamily: S.mono, fontSize: 12, color: C.faint, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }}>
                          {isSecret && !isRevealed ? "•••••••••" : v.value}
                        </span>
                        {isSecret && (
                          <button onClick={() => toggleReveal(v.key)} style={{ background: "none", border: "none", cursor: "pointer", color: C.stone, display: "flex", padding: 2, flexShrink: 0 }}>
                            {isRevealed ? <EyeOff size={13} /> : <Eye size={13} />}
                          </button>
                        )}
                      </div>
                      <span style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: "0.08em", padding: "2px 6px", borderRadius: 4, background: srcStyle.bg, color: srcStyle.color, flexShrink: 0 }}>
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
                        fontSize: 11,
                        transition: "all 0.12s",
                        background: active ? f.activeBg : "transparent",
                        color: active ? f.accent : C.muted,
                        fontWeight: active ? 500 : 400,
                        whiteSpace: "nowrap" as const,
                      }}
                    >
                      {f.label}
                      {active && <X size={9} style={{ marginLeft: 2, flexShrink: 0 }} />}
                    </button>
                  );
                })}
                <div style={{ flex: 1 }} />
                <select
                  value={logTimeRange}
                  onChange={(e) => setLogTimeRange(e.target.value as LogTimeRange)}
                  style={{
                    padding: "4px 24px 4px 10px",
                    borderRadius: 6,
                    border: `1px solid ${C.border}`,
                    background: C.bg,
                    fontFamily: S.body,
                    fontSize: 11,
                    color: C.muted,
                    cursor: "pointer",
                    outline: "none",
                    appearance: "none" as const,
                    backgroundImage:
                      "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E\")",
                    backgroundRepeat: "no-repeat",
                    backgroundPosition: "right 8px center",
                  }}
                >
                  {LOG_TIME_RANGE_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
                <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "4px 8px", borderRadius: 5, border: `1px solid ${C.border}`, background: C.bg }}>
                  <Search size={11} color={C.faint} />
                  <input
                    type="text"
                    placeholder="Find in logs"
                    value={logSearch}
                    onChange={(e) => setLogSearch(e.target.value)}
                    style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: 12, color: C.muted, width: 120, caretColor: C.tealMid }}
                  />
                </div>
                <button
                  type="button"
                  title="Refresh logs"
                  onClick={() => void refetch()}
                  disabled={isFetching}
                  style={{
                    background: "none",
                    border: `1px solid ${C.border}`,
                    cursor: isFetching ? "wait" : "pointer",
                    padding: "4px 6px",
                    borderRadius: 5,
                    color: C.faint,
                    display: "flex",
                    opacity: isFetching ? 0.7 : 1,
                  }}
                >
                  <RefreshCw size={11} className={isFetching ? "dp-spin" : undefined} />
                </button>
                <button
                  type="button"
                  onClick={() => navigator.clipboard.writeText(logs.join("\n"))}
                  style={{ background: "none", border: `1px solid ${C.border}`, cursor: "pointer", padding: "4px 6px", borderRadius: 5, color: C.faint, display: "flex" }}
                >
                  <Copy size={11} />
                </button>
              </div>
              <div style={{ background: C.panel, padding: "10px 0 14px" }}>
                {isLoading ? (
                  <div style={{ padding: "12px 18px", display: "flex", alignItems: "center", gap: 8, fontFamily: S.mono, fontSize: 11, color: C.faint }}>
                    <Loader2 size={14} className="dp-spin" />
                    Loading logs…
                  </div>
                ) : logErrorMessage ? (
                  <div style={{ padding: "12px 18px", fontFamily: S.mono, fontSize: 11, color: C.coral, lineHeight: 1.5 }}>
                    {logErrorMessage}
                  </div>
                ) : filtered.length === 0 ? (
                  <div style={{ padding: "12px 18px", fontFamily: S.mono, fontSize: 11, color: C.faint }}>
                    {logs.length === 0 ? "No log lines in this time window" : "No matching lines"}
                  </div>
                ) : (
                  filtered.map((line, li) => (
                    <div key={li} className="dp-log" style={{ display: "flex", alignItems: "baseline", padding: "1px 0" }}>
                      <span style={{ fontFamily: S.mono, fontSize: 11, color: C.stone, minWidth: 56, textAlign: "right" as const, paddingRight: 18, flexShrink: 0, userSelect: "none" as const }}>
                        {li + 1}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: logLineColor(line), lineHeight: 1.75 }}>{line}</span>
                    </div>
                  ))
                )}
                {!isLoading && !logErrorMessage && filtered.length > 0 && (
                  <div style={{ display: "flex", alignItems: "baseline", padding: "1px 0", marginTop: 2 }}>
                    <span style={{ minWidth: 56, paddingRight: 18, flexShrink: 0 }} />
                    <span className="dp-blink" style={{ fontFamily: S.mono, fontSize: 12, color: C.tealMid }}>
                      ▊
                    </span>
                  </div>
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

type DeployHistoryStatus = "active" | "ready" | "failed" | "undeployed";

function deploymentHistoryUiStatus(h: ApiDeploymentHistoryRecord, live: AgentDeployment): DeployHistoryStatus {
  if (h.undeployed_at) return "undeployed";
  if (h.id === live.id) {
    const ds = mapDeploymentStatus(live);
    if (ds === "error") return "failed";
    if (ds === "pending") return "ready";
    return "active";
  }
  const st = (h.status ?? "").toLowerCase();
  if (st === "error" || st === "failed") return "failed";
  return "ready";
}

interface DeploymentHistoryTableRow {
  id: string;
  status: DeployHistoryStatus;
  build: string;
  duration: string;
  time: string;
  isCurrent: boolean;
  rowLabel: string;
  source: ApiDeploymentHistoryRecord;
}

const DEPLOY_STATUS_STYLE: Record<DeployHistoryStatus, { color: string; label: string }> = {
  active: { color: C.success, label: "Active" },
  ready: { color: C.success, label: "Ready" },
  failed: { color: C.coral, label: "Failed" },
  undeployed: { color: C.stone, label: "Undeployed" },
};

export function DeploymentsTab({
  deployment,
  account,
  onOpenConfigure,
}: {
  deployment: AgentDeployment;
  account: string;
  onOpenConfigure?: () => void;
}) {
  const [deploySearch, setDeploySearch] = useState("");
  const [deployStatus, setDeployStatus] = useState<string[]>([]);
  const [historyPreset, setHistoryPreset] = useState<"all" | "7d" | "30d">("all");
  const [expandedDeploy, setExpandedDeploy] = useState<string | null>(deployment.id);
  const [openDeployMenu, setOpenDeployMenu] = useState<string | null>(null);
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set());

  const { data: historyData, isLoading: historyLoading, isError: historyError } = useDeploymentHistory(account, deployment.name);

  const toggleContainer = (id: string) =>
    setOpenContainers((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  const containers = (deployment.pods ?? []).flatMap((pod) =>
    (pod.containers ?? []).map((c) => ({
      id: `${pod.name}:${c.name}`,
      podName: pod.name,
      name: c.name,
      ready: c.ready ? "1/1" : "0/1",
      uptime: pod.age ?? "—",
      vars: (c.env ?? []).map((e) => {
        const val = e.value ?? "";
        return {
          key: e.name,
          value: val,
          secret: val.startsWith("sk-") || val.startsWith("secret:") || val.includes("••"),
          source: e.from ?? "static",
        };
      }),
      url: undefined as string | undefined,
    })),
  );

  const externalUrls = deployment.external_urls ?? [];
  if (externalUrls.length > 0 && containers.length > 0) {
    const agentContainer = containers.find((c) => c.name.includes("agent")) ?? containers[0];
    if (agentContainer) agentContainer.url = externalUrls[0]?.url;
  }

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

    const cutoff =
      historyPreset === "all" ? 0 : historyPreset === "7d" ? Date.now() - 7 * 86400000 : Date.now() - 30 * 86400000;

    let rows: DeploymentHistoryTableRow[] = merged.map((h, idx) => {
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

    if (cutoff > 0) {
      rows = rows.filter((r) => resolveDeployedAtMs(r.source, deployment) >= cutoff);
    }

    const q = deploySearch.trim().toLowerCase();
    if (q) {
      rows = rows.filter(
        (r) =>
          r.id.toLowerCase().includes(q) ||
          r.build.toLowerCase().includes(q) ||
          deployment.name.toLowerCase().includes(q) ||
          (deployment.display_name?.toLowerCase().includes(q) ?? false),
      );
    }

    if (deployStatus.length > 0) {
      rows = rows.filter((r) => deployStatus.includes(r.status));
    }

    return rows;
  }, [historyData, deployment, historyPreset, deploySearch, deployStatus]);

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr minmax(0, 900px) 1fr", gap: 12, alignItems: "start" }}>
      <div />
      <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
          <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal, flex: 1 }}>Deployments</span>
          <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "5px 10px", borderRadius: 7, border: `1px solid ${C.border}`, background: C.bg }}>
            <Search size={12} color={C.faint} />
            <input
              type="text"
              placeholder="Search by name, build, id"
              value={deploySearch}
              onChange={(e) => setDeploySearch(e.target.value)}
              style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: 12, color: C.muted, width: 200, caretColor: C.tealMid }}
            />
          </div>
          <MultiSelect
            options={[
              { value: "active", label: "Active", color: C.tealMid },
              { value: "ready", label: "Ready", color: C.success },
              { value: "failed", label: "Failed", color: C.coral },
              { value: "undeployed", label: "Undeployed", color: C.stone },
            ]}
            selected={deployStatus}
            onChange={setDeployStatus}
            placeholder="All statuses"
          />
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <Calendar size={12} color={C.faint} />
            <select
              value={historyPreset}
              onChange={(e) => setHistoryPreset(e.target.value as typeof historyPreset)}
              style={{
                padding: "5px 22px 5px 8px",
                borderRadius: 7,
                border: `1px solid ${C.border}`,
                background: C.bg,
                fontFamily: S.body,
                fontSize: 12,
                color: C.muted,
                cursor: "pointer",
                outline: "none",
                appearance: "none" as const,
                backgroundImage:
                  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E\")",
                backgroundRepeat: "no-repeat",
                backgroundPosition: "right 6px center",
              }}
            >
              <option value="all">All time</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
            </select>
          </div>
        </div>

        {historyError && (
          <p style={{ fontFamily: S.mono, fontSize: 11, color: C.coral, margin: "0 0 10px" }}>
            Could not load deployment history from the server.
          </p>
        )}

        <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden" }}>
          <div style={{ display: "grid", gridTemplateColumns: "16px minmax(160px, 1fr) 80px 72px 110px 72px 32px", gap: 12, padding: "8px 16px", borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
            {["", "Deployment", "Status", "Duration", "Build No.", "Deployed on", ""].map((h) => (
              <span key={h} style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: "0.07em", color: C.faint }}>
                {h.toUpperCase()}
              </span>
            ))}
          </div>

          {historyLoading ? (
            <div style={{ padding: "20px 16px", display: "flex", alignItems: "center", gap: 10, fontFamily: S.mono, fontSize: 11, color: C.faint }}>
              <Loader2 size={14} className="dp-spin" />
              Loading deployment history…
            </div>
          ) : allRows.length === 0 ? (
            <div style={{ padding: "20px 16px", fontFamily: S.mono, fontSize: 11, color: C.faint }}>
              No deployments match your filters.
            </div>
          ) : (
            allRows.map((d, i) => {
              const ds = DEPLOY_STATUS_STYLE[d.status];
              const isCurrent = d.isCurrent;
              const isExpanded = expandedDeploy === d.id;
              return (
                <div key={d.id} style={{ borderBottom: i < allRows.length - 1 ? `1px solid ${C.border}` : "none" }}>
                  <div
                    onClick={() => setExpandedDeploy(isExpanded ? null : d.id)}
                    style={{
                      display: "grid",
                      gridTemplateColumns: "16px minmax(160px, 1fr) 80px 72px 110px 72px 32px",
                      gap: 12,
                      padding: "12px 16px",
                      alignItems: "center",
                      cursor: "pointer",
                      borderLeft: isCurrent ? `3px solid ${C.tealMid}` : "3px solid transparent",
                      background: isExpanded ? C.bgDeep : isCurrent ? "rgba(21,130,125,0.02)" : "transparent",
                      transition: "background 0.12s",
                    }}
                  >
                    <ChevronRight size={12} color={C.faint} style={{ transition: "transform 0.15s", transform: isExpanded ? "rotate(90deg)" : "none" }} />
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontFamily: S.body, fontSize: 12, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={d.rowLabel}>
                        {d.rowLabel}
                      </div>
                    </div>
                    <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
                      <span style={{ width: 8, height: 8, borderRadius: "50%", background: ds.color, display: "inline-block", flexShrink: 0 }} />
                      <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: "0.06em", color: ds.color, fontWeight: 500 }}>
                        {ds.label.toUpperCase()}
                      </span>
                    </div>
                    <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{d.duration}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 11, fontWeight: 600, color: C.muted }}>{d.build}</span>
                    <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, whiteSpace: "nowrap" as const }}>{d.time}</span>
                    <div style={{ position: "relative" }} onClick={(e) => e.stopPropagation()}>
                      <button
                        type="button"
                        onClick={() => setOpenDeployMenu(openDeployMenu === d.id ? null : d.id)}
                        style={{ background: "none", border: "none", cursor: "pointer", color: C.faint, display: "flex", padding: 4, borderRadius: 4 }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.background = C.bgDeep;
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.background = "none";
                        }}
                      >
                        <MoreVertical size={13} />
                      </button>
                      {openDeployMenu === d.id && (
                        <>
                          <div onClick={() => setOpenDeployMenu(null)} style={{ position: "fixed", inset: 0, zIndex: 10 }} />
                          <div style={{ position: "absolute", right: 0, top: "calc(100% + 4px)", zIndex: 20, minWidth: 160, background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 8, overflow: "hidden", boxShadow: "0 6px 20px rgba(0,0,0,0.1)" }}>
                            <button
                              type="button"
                              onClick={() => {
                                setOpenDeployMenu(null);
                                onOpenConfigure?.();
                              }}
                              style={{ width: "100%", display: "flex", alignItems: "center", gap: 8, padding: "9px 14px", background: "none", border: "none", cursor: "pointer", fontFamily: S.body, fontSize: 12, color: C.text, textAlign: "left" as const }}
                              onMouseEnter={(e) => {
                                e.currentTarget.style.background = C.bgDeep;
                              }}
                              onMouseLeave={(e) => {
                                e.currentTarget.style.background = "none";
                              }}
                            >
                              Redeploy…
                            </button>
                            <div style={{ height: 1, background: C.border }} />
                            <button
                              type="button"
                              disabled={!isCurrent || containers.length === 0}
                              title={!isCurrent ? "Only the live deployment has pod logs here" : undefined}
                              onClick={() => {
                                setOpenDeployMenu(null);
                                if (isCurrent && containers.length > 0) {
                                  setExpandedDeploy(d.id);
                                  setOpenContainers(new Set([containers[0].id]));
                                }
                              }}
                              style={{
                                width: "100%",
                                display: "flex",
                                alignItems: "center",
                                gap: 8,
                                padding: "9px 14px",
                                background: "none",
                                border: "none",
                                cursor: isCurrent && containers.length > 0 ? "pointer" : "not-allowed",
                                fontFamily: S.body,
                                fontSize: 12,
                                color: C.text,
                                textAlign: "left" as const,
                                opacity: isCurrent && containers.length > 0 ? 1 : 0.45,
                              }}
                              onMouseEnter={(e) => {
                                if (isCurrent && containers.length > 0) e.currentTarget.style.background = C.bgDeep;
                              }}
                              onMouseLeave={(e) => {
                                e.currentTarget.style.background = "none";
                              }}
                            >
                              View pod logs
                            </button>
                            <div style={{ height: 1, background: C.border }} />
                            <button type="button" disabled title="Rollback is not available yet" style={{ width: "100%", display: "flex", alignItems: "center", gap: 8, padding: "9px 14px", background: "none", border: "none", cursor: "not-allowed", fontFamily: S.body, fontSize: 12, color: C.coral, textAlign: "left" as const, opacity: 0.45 }}>
                              Rollback
                            </button>
                          </div>
                        </>
                      )}
                    </div>
                  </div>

                  {isExpanded && (
                    <div style={{ padding: "8px 16px 16px", borderTop: `1px solid ${C.border}`, background: C.bg }}>
                      {isCurrent ? (
                        containers.length === 0 ? (
                          <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0 }}>No container data available</p>
                        ) : (
                          containers.map((c) => (
                            <ActiveContainerAccordion
                              key={c.id}
                              name={c.name}
                              url={c.url}
                              ready={c.ready}
                              uptime={c.uptime}
                              liveLogs={{ deploymentId: deployment.id, podName: c.podName, containerName: c.name }}
                              vars={c.vars}
                              isOpen={openContainers.has(c.id)}
                              onToggle={() => toggleContainer(c.id)}
                            />
                          ))
                        )
                      ) : (
                        <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0 }}>
                          Pod logs are only available for the live deployment ({deployment.id.slice(0, 8)}…).
                        </p>
                      )}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </div>
      <div />
    </div>
  );
}
