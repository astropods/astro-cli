import { useLocation, useParams, Link } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { useDeployment } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { isDeployingState } from "@/lib/deployment-utils";
import { MetricCardSkeleton } from "@/components/deployed-agent/detail/monitor/HeadlineMetrics";


export async function loader({ params }: Route.LoaderArgs) {
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  return { account, deploymentId };
}

export const meta: Route.MetaFunction = () => {
  return [{ title: "Agent | Astro" }];
};

function Ghost({ width = "100%", height = 12, radius = 6 }: { width?: string | number; height?: number; radius?: number }) {
  return (
    <span
      className="dp-pulse"
      style={{
        display: "inline-block",
        width,
        height,
        borderRadius: radius,
        background: "linear-gradient(90deg, var(--muted) 0%, var(--border) 45%, var(--muted) 100%)",
      }}
    />
  );
}

function DeployedAgentDetailSkeleton() {
  const pad = "clamp(16px, 4vw, 108px)";
  return (
    <div style={{ display: "flex", flex: 1, minHeight: 0, background: "var(--muted)" }}>
      <div style={{ display: "flex", flex: 1, flexDirection: "column", minWidth: 0 }}>
        {/* top bar */}
        <header
          style={{
            background: "var(--surface)",
            borderBottom: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            padding: "0 clamp(12px, 3vw, 40px)",
            height: 63,
            flexShrink: 0,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <Ghost width={14} height={14} radius={3} />
            <Ghost width={26} height={26} radius={4} />
            <Ghost width={120} height={16} radius={4} />
            <Ghost width={52} height={20} radius={99} />
          </div>
        </header>

        {/* tab bar */}
        <div
          style={{
            display: "flex",
            padding: `0 ${pad}`,
            background: "var(--muted)",
            borderBottom: "1px solid var(--border)",
            flexShrink: 0,
            gap: 0,
          }}
        >
          <div style={{ padding: "11px 16px 11px 0", borderBottom: "2px solid var(--color-teal-600)" }}>
            <Ghost width={80} height={16} radius={4} />
          </div>
          <div style={{ padding: "11px 16px" }}>
            <Ghost width={100} height={16} radius={4} />
          </div>
        </div>

        {/* tab content area */}
        <div style={{ flex: 1, overflowY: "auto", padding: `24px calc(${pad} + 4px)` }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {/* heading + time window select */}
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Ghost width={120} height={24} radius={6} />
              <Ghost width={160} height={36} radius={6} />
            </div>

            {/* 4 headline metric cards */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
              {Array.from({ length: 4 }, (_, i) => (
                <MetricCardSkeleton key={i} />
              ))}
            </div>

            {/* two-column: request volume + token usage */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))", gap: 12 }}>
              {/* request volume */}
              <div
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: 10,
                  padding: "14px 16px",
                }}
              >
                <Ghost width={130} height={16} radius={4} />
                <div style={{ display: "flex", gap: 10, marginTop: 8, marginBottom: 6 }}>
                  <Ghost width={70} height={10} radius={4} />
                  <Ghost width={80} height={10} radius={4} />
                </div>
                <Ghost width="100%" height={130} radius={6} />
              </div>

              {/* token usage */}
              <div
                style={{
                  background: "var(--surface)",
                  border: "1px solid var(--border)",
                  borderRadius: 14,
                  overflow: "hidden",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "12px 18px",
                    borderBottom: "1px solid var(--border)",
                  }}
                >
                  <Ghost width={100} height={16} radius={4} />
                  <Ghost width={90} height={24} radius={4} />
                </div>
                <div style={{ padding: "16px 18px 14px" }}>
                  <Ghost width="35%" height={24} radius={6} />
                  <div style={{ margin: "14px 0 10px" }}>
                    <Ghost width="100%" height={12} radius={999} />
                  </div>
                  <Ghost width="40%" height={12} radius={6} />
                </div>
              </div>
            </div>

            {/* traces panel */}
            <div
              style={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: 10,
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
              }}
            >
              {/* traces header */}
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "12px 16px",
                  borderBottom: "1px solid var(--border)",
                }}
              >
                <Ghost width={60} height={16} radius={4} />
                <Ghost width={120} height={28} radius={6} />
              </div>
              {/* traces grid header */}
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "250px 1fr 80px 80px 80px 132px",
                  gap: 10,
                  padding: "7px 16px",
                  background: "var(--muted)",
                  borderBottom: "1px solid var(--border)",
                }}
              >
                <Ghost width={40} height={10} radius={4} />
                <span />
                <Ghost width={48} height={10} radius={4} />
                <Ghost width={52} height={10} radius={4} />
                <Ghost width={48} height={10} radius={4} />
                <Ghost width={72} height={10} radius={4} />
              </div>
              {/* trace skeleton rows */}
              {Array.from({ length: 4 }, (_, i) => (
                <div
                  key={i}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "250px 1fr 80px 80px 80px 132px",
                    gap: 10,
                    padding: "10px 16px",
                    borderBottom: "1px solid var(--border)",
                    alignItems: "center",
                  }}
                >
                  <Ghost width="70%" height={12} radius={4} />
                  <span />
                  <Ghost width="60%" height={12} radius={4} />
                  <Ghost width="55%" height={12} radius={4} />
                  <Ghost width="50%" height={12} radius={4} />
                  <Ghost width="65%" height={12} radius={4} />
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const location = useLocation();
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? loaderData?.account ?? "";
  const { isAuthenticated, personalAccount } = useAuth();
  const { data: deploymentsData, isLoading } = useDeployment(deploymentId ?? "", isAuthenticated);
  const deployment = deploymentsData?.deployment ?? null;
  const monitorLocked = deployment ? isDeployingState(deployment) : false;
  const isPersonal = personalAccount?.name === account;
  const requestedFromAgents = (location.state as { fromAgents?: boolean } | null)?.fromAgents === true;

  if (isLoading) {
    return <DeployedAgentDetailSkeleton />;
  }

  if (!deployment) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/agents">My Agents</Link>
        </Button>
      </div>
    );
  }

  return (
    <ActiveDetailView
      deployment={deployment}
      account={account}
      isPersonal={isPersonal}
      monitorLocked={monitorLocked}
      backPathOverride={requestedFromAgents ? "/agents" : undefined}
    />
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  return (
    <ProtectedRoute>
      <DeployedAgentDetailContent loaderData={loaderData} />
    </ProtectedRoute>
  );
}
