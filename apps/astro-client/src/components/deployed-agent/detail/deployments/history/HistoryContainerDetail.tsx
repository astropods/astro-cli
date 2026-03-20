import type { ContainerRow } from "./types";

const C = {
  bgAlt: "var(--surface)",
  border: "var(--border)",
  teal: "var(--primary)",
  faint: "var(--faint-foreground)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

interface HistoryContainerDetailProps {
  deploymentId: string;
  containers: ContainerRow[];
  renderContainer: (container: ContainerRow) => React.ReactNode;
}

export function HistoryContainerDetail({ deploymentId, containers, renderContainer }: HistoryContainerDetailProps) {
  return (
    <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "10px 10px 6px" }}>
      <div style={{ marginBottom: 8 }}>
        <span style={{ display: "block", fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.teal }}>
          Live Container Detail
        </span>
        <span style={{ display: "block", fontFamily: S.mono, fontSize: 10, color: C.faint, marginTop: 2 }}>
          Deployment {deploymentId.slice(0, 8)}...
        </span>
      </div>
      {containers.length === 0 ? (
        <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: "8px 4px" }}>No container data available.</p>
      ) : (
        containers.map((c) => <div key={c.id}>{renderContainer(c)}</div>)
      )}
    </div>
  );
}
