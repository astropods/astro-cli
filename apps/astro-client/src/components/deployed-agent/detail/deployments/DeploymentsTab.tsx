import { useEffect, useMemo, useRef, useState } from "react";
import { useDeploymentHistory, useDeploymentEvents } from "@/api/queries/deployments";
import { formatDate, isDeployingState } from "@/lib/deployment-utils";
import type { AgentDeployment, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { cn } from "@/lib/utils";
import { isSensitiveEnvVar } from "@/lib/env-utils";
import { useCompactLayout } from "@/hooks/use-compact-layout";
import { DeploymentHistoryTable } from "./DeploymentHistoryTable";
import {
  formatDurationMs,
  resolveDeployedAtMs,
  deploymentHistoryDurationMs,
  deploymentHistoryUiStatus,
} from "./history/utils";
import type { DeploymentHistoryTableRow, ServiceRow } from "./history/types";

export function DeploymentsTab({
  deployment,
  account,
  isPausing = false,
  isResuming = false,
  isRestarting = false,
  isGloballyRestarting = false,
  onRollback,
  onPodRestartStateChange,
}: {
  deployment: AgentDeployment;
  account: string;
  isPausing?: boolean;
  isResuming?: boolean;
  isRestarting?: boolean;
  isGloballyRestarting?: boolean;
  onRollback?: (revision: number, buildId: string) => void;
  onPodRestartStateChange?: (isRestarting: boolean) => void;
}) {
  const isCompact = useCompactLayout();
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set());
  const hasAutoOpenedOverview = useRef(false);

  const isDeploying = isDeployingState(deployment);
  const { data: eventsData } = useDeploymentEvents(deployment.id);
  const allEvents = eventsData?.events ?? [];
  const { data: historyData, isLoading: historyLoading, isError: historyError } = useDeploymentHistory(
    account, deployment.name, deployment.id, true,
    { refetchInterval: isDeploying ? 4000 : false },
  );

  const workloads = deployment.workloads;
  const externalUrls = deployment.external_urls;
  const serviceRows = useMemo((): ServiceRow[] => {
    const primaryUrl = (externalUrls ?? [])[0]?.url;
    return (workloads ?? []).map((wl) => {
      const mappedContainers = (wl.containers ?? []).map((c) => ({
        name: c.name,
        ready: c.ready,
        vars: (c.env ?? []).map((e) => {
          const val = e.value ?? "";
          return {
            key: e.name,
            value: val,
            source: e.from ?? "static",
            secret: isSensitiveEnvVar(e.name, val, e.from ?? "static"),
          };
        }).sort((a, b) => a.key.localeCompare(b.key)),
      }));
      const readyCount = mappedContainers.filter((c) => c.ready).length;
      const url = primaryUrl && wl.component === "agent" ? primaryUrl : undefined;
      return {
        id: wl.name,
        workloadName: wl.name,
        podName: wl.pod_name,
        title: wl.component || wl.name,
        isAgentService: wl.component === "agent",
        readyText: `${readyCount}/${mappedContainers.length || 0}`,
        uptime: wl.age ?? "—",
        containers: mappedContainers,
        url,
        urls: wl.urls,
      };
    });
  }, [workloads, externalUrls]);

  const totalServiceCount = serviceRows.length;

  const deploymentId = deployment.id;
  const deploymentName = deployment.name;
  const deploymentDisplayName = deployment.display_name;
  const deploymentBuildId = deployment.build_id;
  const deploymentNamespace = deployment.namespace;
  const deploymentStatus = deployment.status;
  const deploymentCreatedAt = deployment.created_at;

  const allRows = useMemo((): DeploymentHistoryTableRow[] => {
    const fromApi = historyData?.deployments ?? [];
    const merged: ApiDeploymentHistoryRecord[] = [...fromApi];
    const hasCurrentInApi = fromApi.some((h) => h.is_current);
    if (!hasCurrentInApi) {
      merged.unshift({
        id: deploymentId,
        agent_name: deploymentName,
        revision: 1,
        build_id: deploymentBuildId,
        namespace: deploymentNamespace,
        display_name: deployment.display_name ?? "",
        is_current: true,
        status: deploymentStatus,
        deployed_at: deploymentCreatedAt,
        spec: {},
      });
    }
    merged.sort((a, b) => resolveDeployedAtMs(b, deployment) - resolveDeployedAtMs(a, deployment));

    return merged.map((h, idx) => {
      const isCurrent = h.is_current;
      const status = deploymentHistoryUiStatus(h, deployment);
      const build = h.revision ? `#${h.revision}` : (h.build_id?.slice(0, 8) || "—");
      const rowLabel = isCurrent ? deploymentDisplayName || deploymentName : `${deploymentName} · ${h.build_id?.slice(0, 8) || "—"}`;
      const durMs = deploymentHistoryDurationMs(h, idx, merged, deployment, isCurrent);
      const deployedAtMs = resolveDeployedAtMs(h, deployment);
      const deployedAtIso = new Date(deployedAtMs).toISOString();
      const timeOfDay = Number.isFinite(deployedAtMs)
        ? new Date(deployedAtMs).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false })
        : "—";
      const rowKey = `${h.id}-${h.revision}`;
      return { id: rowKey, status, build, duration: durMs !== null ? formatDurationMs(durMs) : "—", time: formatDate(deployedAtIso), timeOfDay, isCurrent, rowLabel, source: h };
    });
  }, [historyData, deploymentId, deploymentName, deploymentDisplayName, deploymentBuildId, deploymentNamespace, deploymentStatus, deploymentCreatedAt, deployment]);

  const pastRows = useMemo(() => allRows.filter((row) => !row.isCurrent), [allRows]);
  const currentRow = useMemo(() => allRows.find((row) => row.isCurrent) ?? null, [allRows]);

  // Use the most recent row (allRows is sorted descending) so the stat card
  // always shows when the agent was last deployed, matching the top table row.
  const lastDeployedOnMs = useMemo(() => {
    if (allRows.length > 0) return resolveDeployedAtMs(allRows[0].source, deployment);
    if (deploymentCreatedAt) return new Date(deploymentCreatedAt).getTime();
    return null;
  }, [allRows, deployment, deploymentCreatedAt]);

  useEffect(() => {
    hasAutoOpenedOverview.current = false;
    setOpenContainers(new Set());
  }, [deploymentId]);

  useEffect(() => {
    if (serviceRows.length === 0) return;
    if (hasAutoOpenedOverview.current) return;
    hasAutoOpenedOverview.current = true;
    setOpenContainers(new Set([serviceRows[0].id]));
  }, [serviceRows]);

  const toggleContainer = (id: string) =>
    setOpenContainers((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2.5">
        <span className="font-sans text-heading-1 font-semibold text-foreground flex-1">
          Deployments
        </span>
      </div>

      <div className="flex flex-col gap-3">
        <div className="grid grid-cols-4 gap-2.5">
          {[
            { label: "CURRENT BUILD", value: deploymentBuildId?.slice(0, 8) || "—", wrap: false },
            {
              label: "DEPLOYMENT STATUS",
              value: String(deploymentStatus || "unknown").charAt(0).toUpperCase() + String(deploymentStatus || "unknown").slice(1).toLowerCase(),
              wrap: false,
            },
            {
              label: "LAST DEPLOYED ON",
              value: lastDeployedOnMs !== null && Number.isFinite(lastDeployedOnMs)
                ? `${formatDate(new Date(lastDeployedOnMs).toISOString())},${isCompact ? "\n" : " "}${new Date(lastDeployedOnMs).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false })}`
                : "—",
              wrap: true,
            },
            { label: "SERVICES", value: String(totalServiceCount), wrap: false },
          ].map((item) => (
            <div key={item.label} className="bg-surface border border-border rounded-[10px] px-3.5 py-3 min-w-0">
              <span className="block font-mono text-label tracking-[0.07em] text-faint-foreground mb-2">
                {item.label}
              </span>
              <span
                className={cn(
                  "block font-sans text-heading-4 font-semibold text-foreground",
                  item.wrap
                    ? isCompact ? "whitespace-pre-line" : "whitespace-nowrap"
                    : "truncate",
                  item.wrap && "leading-tight",
                )}
              >
                {item.value}
              </span>
            </div>
          ))}
        </div>

        <DeploymentHistoryTable
          currentRow={currentRow}
          pastRows={pastRows}
          serviceRows={serviceRows}
          deploymentId={deploymentId}
          isCompact={isCompact}
          openContainers={openContainers}
          onToggleContainer={toggleContainer}
          onRollback={onRollback}
          historyLoading={historyLoading}
          historyError={historyError}
          isPausing={isPausing}
          isResuming={isResuming}
          isRestarting={isRestarting}
          isGloballyRestarting={isGloballyRestarting}
          onPodRestartStateChange={onPodRestartStateChange}
          events={allEvents}
        />
      </div>
    </div>
  );
}
