import { Calendar, ChevronRight, Loader2, MoreVertical, Search } from "lucide-react";
import { MultiSelect } from "../../shared/MultiSelect";
import type { ContainerRow, DeploymentHistoryTableRow, DeployHistoryStatus } from "./types";

const C = {
  bg: "var(--muted)",
  bgAlt: "var(--surface)",
  bgDeep: "var(--muted)",
  panel: "var(--surface)",
  border: "var(--border)",
  teal: "var(--primary)",
  tealMid: "var(--color-teal-600)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  faint: "var(--faint-foreground)",
  stone: "var(--color-stone-500)",
  coral: "var(--color-coral-600)",
  success: "var(--color-green-700)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  body: "var(--text-body)",
  heading4: "var(--text-heading-4)",
  label: "var(--text-label)",
  monoSm: "var(--text-mono-sm)",
} as const;

const I = {
  sm: 12,
  md: 14,
} as const;

const DEPLOY_STATUS_STYLE: Record<DeployHistoryStatus, { color: string; label: string }> = {
  active: { color: C.success, label: "Active" },
  ready: { color: C.success, label: "Ready" },
  failed: { color: C.coral, label: "Failed" },
  undeployed: { color: C.stone, label: "Undeployed" },
};

interface HistoryDeploymentsTableProps {
  rows: DeploymentHistoryTableRow[];
  historyLoading: boolean;
  historyError: boolean;
  deploySearch: string;
  onDeploySearchChange: (value: string) => void;
  deployStatus: string[];
  onDeployStatusChange: (value: string[]) => void;
  historyPreset: "all" | "7d" | "30d";
  onHistoryPresetChange: (value: "all" | "7d" | "30d") => void;
  expandedDeploy: string | null;
  onExpandedDeployChange: (id: string | null) => void;
  openDeployMenu: string | null;
  onOpenDeployMenuChange: (id: string | null) => void;
  containers: ContainerRow[];
  onOpenConfigure?: () => void;
  onViewPodLogs: (deploymentId: string) => void;
  renderExpandedDeployment: (deploymentId: string, isCurrent: boolean) => React.ReactNode;
}

export function HistoryDeploymentsTable({
  rows,
  historyLoading,
  historyError,
  deploySearch,
  onDeploySearchChange,
  deployStatus,
  onDeployStatusChange,
  historyPreset,
  onHistoryPresetChange,
  expandedDeploy,
  onExpandedDeployChange,
  openDeployMenu,
  onOpenDeployMenuChange,
  containers,
  onOpenConfigure,
  onViewPodLogs,
  renderExpandedDeployment,
}: HistoryDeploymentsTableProps) {
  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "5px 10px", borderRadius: 7, border: `1px solid ${C.border}`, background: C.bg }}>
          <Search size={I.sm} color={C.faint} />
          <input
            type="text"
            placeholder="Search by name, build, id"
            value={deploySearch}
            onChange={(e) => onDeploySearchChange(e.target.value)}
            style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: T.body, color: C.muted, width: 200, caretColor: C.tealMid }}
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
          onChange={onDeployStatusChange}
          placeholder="All statuses"
        />
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <Calendar size={I.sm} color={C.faint} />
          <select
            value={historyPreset}
            onChange={(e) => onHistoryPresetChange(e.target.value as "all" | "7d" | "30d")}
            style={{
              padding: "5px 22px 5px 8px",
              borderRadius: 7,
              border: `1px solid ${C.border}`,
              background: C.bg,
              fontFamily: S.body,
              fontSize: T.body,
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
        <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.coral, margin: "0 0 10px" }}>
          Could not load deployment history from the server.
        </p>
      )}

      <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "16px minmax(160px, 1fr) 88px 84px 116px 116px 32px", gap: 12, padding: "8px 16px", borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
          {["", "Deployment", "Status", "Duration", "Build No.", "Deployed on", ""].map((h) => (
            <span key={h} style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint, whiteSpace: "nowrap" }}>
              {h.toUpperCase()}
            </span>
          ))}
        </div>

        {historyLoading ? (
          <div style={{ padding: "20px 16px", display: "flex", alignItems: "center", gap: 10, fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
            <Loader2 size={I.md} className="dp-spin" />
            Loading deployment history…
          </div>
        ) : rows.length === 0 ? (
          <div style={{ padding: "20px 16px", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
            No deployments match your filters.
          </div>
        ) : (
          rows.map((d, i) => {
            const ds = DEPLOY_STATUS_STYLE[d.status];
            const isCurrent = d.isCurrent;
            const isExpanded = expandedDeploy === d.id;
            return (
              <div key={d.id} style={{ borderBottom: i < rows.length - 1 ? `1px solid ${C.border}` : "none" }}>
                <div
                  onClick={() => onExpandedDeployChange(isExpanded ? null : d.id)}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "16px minmax(160px, 1fr) 88px 84px 116px 116px 32px",
                    gap: 12,
                    padding: "12px 16px",
                    alignItems: "center",
                    cursor: "pointer",
                    borderLeft: isCurrent ? `3px solid ${C.tealMid}` : "3px solid transparent",
                    background: isExpanded ? C.bgDeep : isCurrent ? "rgba(21,130,125,0.02)" : "transparent",
                    transition: "background 0.12s",
                  }}
                >
                  <ChevronRight size={I.sm} color={C.faint} style={{ transition: "transform 0.15s", transform: isExpanded ? "rotate(90deg)" : "none" }} />
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 500, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }} title={d.rowLabel}>
                      {d.rowLabel}
                    </div>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
                    <span style={{ width: 8, height: 8, borderRadius: "50%", background: ds.color, display: "inline-block", flexShrink: 0 }} />
                    <span style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.06em", color: ds.color, fontWeight: 500 }}>
                      {ds.label.toUpperCase()}
                    </span>
                  </div>
                  <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>{d.duration}</span>
                  <span style={{ fontFamily: S.mono, fontSize: T.monoSm, fontWeight: 600, color: C.muted }}>{d.build}</span>
                  <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, whiteSpace: "nowrap" as const }}>{d.time}</span>
                  <div style={{ position: "relative" }} onClick={(e) => e.stopPropagation()}>
                    <button
                      type="button"
                      onClick={() => onOpenDeployMenuChange(openDeployMenu === d.id ? null : d.id)}
                      style={{ background: "none", border: "none", cursor: "pointer", color: C.faint, display: "flex", padding: 4, borderRadius: 4 }}
                      onMouseEnter={(e) => {
                        e.currentTarget.style.background = C.bgDeep;
                      }}
                      onMouseLeave={(e) => {
                        e.currentTarget.style.background = "none";
                      }}
                    >
                      <MoreVertical size={I.md} />
                    </button>
                    {openDeployMenu === d.id && (
                      <>
                        <div onClick={() => onOpenDeployMenuChange(null)} style={{ position: "fixed", inset: 0, zIndex: 10 }} />
                        <div style={{ position: "absolute", right: 0, top: "calc(100% + 4px)", zIndex: 20, minWidth: 160, background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 8, overflow: "hidden", boxShadow: "0 6px 20px rgba(0,0,0,0.1)" }}>
                          <button
                            type="button"
                            onClick={() => {
                              onOpenDeployMenuChange(null);
                              onOpenConfigure?.();
                            }}
                            style={{ width: "100%", display: "flex", alignItems: "center", gap: 8, padding: "9px 14px", background: "none", border: "none", cursor: "pointer", fontFamily: S.body, fontSize: T.body, color: C.text, textAlign: "left" as const }}
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
                              onOpenDeployMenuChange(null);
                              if (isCurrent && containers.length > 0) {
                                onExpandedDeployChange(d.id);
                                onViewPodLogs(d.id);
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
                              fontSize: T.body,
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
                          <button type="button" disabled title="Rollback is not available yet" style={{ width: "100%", display: "flex", alignItems: "center", gap: 8, padding: "9px 14px", background: "none", border: "none", cursor: "not-allowed", fontFamily: S.body, fontSize: T.body, color: C.coral, textAlign: "left" as const, opacity: 0.45 }}>
                            Rollback
                          </button>
                        </div>
                      </>
                    )}
                  </div>
                </div>

                {isExpanded && renderExpandedDeployment(d.id, isCurrent)}
              </div>
            );
          })
        )}
      </div>
    </>
  );
}
