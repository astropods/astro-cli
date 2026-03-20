import type { AgentDeployment } from "@/lib/api";
import type { ContainerRow, DeploymentHistoryTableRow } from "./types";

const C = {
  bgAlt: "var(--surface)",
  bgDeep: "var(--muted)",
  border: "var(--border)",
  faint: "var(--faint-foreground)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  teal: "var(--primary)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

interface HistoryOverviewProps {
  deployment: AgentDeployment;
  allRows: DeploymentHistoryTableRow[];
  containers: ContainerRow[];
  onOpenConfigure?: () => void;
  onOpenHistoryDeployments: () => void;
}

export function HistoryOverview({
  deployment,
  allRows,
  containers,
  onOpenConfigure,
  onOpenHistoryDeployments,
}: HistoryOverviewProps) {
  const recentRows = allRows.slice(0, 3);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
        {[
          { label: "CURRENT BUILD", value: deployment.build_id?.slice(0, 8) || "—" },
          { label: "STATUS", value: String(deployment.status || "unknown").toUpperCase() },
          { label: "DEPLOYED", value: deployment.created_at ? new Date(deployment.created_at).toLocaleString() : "—" },
          { label: "CONTAINERS", value: String(containers.length) },
        ].map((item) => (
          <div key={item.label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px" }}>
            <span style={{ display: "block", fontFamily: S.mono, fontSize: 9, letterSpacing: "0.07em", color: C.faint, marginBottom: 8 }}>
              {item.label}
            </span>
            <span style={{ display: "block", fontFamily: S.body, fontSize: 14, fontWeight: 600, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {item.value}
            </span>
          </div>
        ))}
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <button
          type="button"
          onClick={onOpenHistoryDeployments}
          style={{
            padding: "7px 12px",
            borderRadius: 7,
            border: `1px solid ${C.border}`,
            background: C.bgAlt,
            cursor: "pointer",
            fontFamily: S.body,
            fontSize: 12,
            color: C.text,
          }}
        >
          View full history
        </button>
        <button
          type="button"
          onClick={() => onOpenConfigure?.()}
          style={{
            padding: "7px 12px",
            borderRadius: 7,
            border: `1px solid ${C.border}`,
            background: "transparent",
            cursor: "pointer",
            fontFamily: S.body,
            fontSize: 12,
            color: C.faint,
          }}
        >
          Configure
        </button>
      </div>

      <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden" }}>
        <div style={{ padding: "8px 12px", borderBottom: `1px solid ${C.border}`, background: C.bgDeep }}>
          <span style={{ fontFamily: S.mono, fontSize: 10, letterSpacing: "0.08em", color: C.faint }}>RECENT DEPLOYMENTS</span>
        </div>
        {recentRows.length === 0 ? (
          <div style={{ padding: "14px 12px", fontFamily: S.mono, fontSize: 11, color: C.faint }}>No deployments yet.</div>
        ) : (
          recentRows.map((row, idx) => (
            <div
              key={row.id}
              style={{
                display: "grid",
                gridTemplateColumns: "minmax(160px, 1fr) 90px 120px",
                gap: 10,
                padding: "10px 12px",
                borderBottom: idx < recentRows.length - 1 ? `1px solid ${C.border}` : "none",
                alignItems: "center",
              }}
            >
              <span style={{ fontFamily: S.body, fontSize: 12, color: C.text, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {row.rowLabel}
              </span>
              <span style={{ fontFamily: S.mono, fontSize: 10, color: C.muted }}>{row.build}</span>
              <span style={{ fontFamily: S.mono, fontSize: 10, color: C.faint }}>{row.time}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
