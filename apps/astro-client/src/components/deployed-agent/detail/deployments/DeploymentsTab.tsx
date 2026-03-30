import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useDeploymentHistory } from "@/api/queries/deployments";
import { formatDate, isDeployingState } from "@/lib/deployment-utils";
import type { AgentDeployment, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { deploymentKeys } from "@/api/queries/keys";
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
  onOpenConfigure,
}: {
  deployment: AgentDeployment;
  account: string;
  onOpenConfigure?: () => void;
}) {
  const queryClient = useQueryClient();
  const isCompact = useCompactLayout();
  const [openContainers, setOpenContainers] = useState<Set<string>>(new Set());
  const hasAutoOpenedOverview = useRef(false);

  const { data: historyData, isLoading: historyLoading, isError: historyError } = useDeploymentHistory(account, deployment.name);

  const isDeploying = isDeployingState(deployment);
  useEffect(() => {
    if (!isDeploying) return;
    const interval = setInterval(() => {
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(deployment.id) });
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.history(account, deployment.name) });
    }, 4000);
    return () => clearInterval(interval);
  }, [isDeploying, queryClient, deployment.id, account, deployment.name]);

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
    const seen = new Set(fromApi.map((h) => h.id));
    const merged: ApiDeploymentHistoryRecord[] = [...fromApi];
    if (!seen.has(deploymentId)) {
      merged.unshift({
        id: deploymentId,
        agent_name: deploymentName,
        build_id: deploymentBuildId,
        namespace: deploymentNamespace,
        status: deploymentStatus,
        deployed_at: deploymentCreatedAt,
        spec: {},
      });
    }
    merged.sort((a, b) => resolveDeployedAtMs(b, deployment) - resolveDeployedAtMs(a, deployment));

    return merged.map((h, idx) => {
      const isCurrent = h.id === deploymentId;
      const status = deploymentHistoryUiStatus(h, deployment);
      const build = h.build_id?.slice(0, 8) || "—";
      const rowLabel = isCurrent ? deploymentDisplayName || deploymentName : `${deploymentName} · ${build}`;
      const durMs = deploymentHistoryDurationMs(h, idx, merged, deployment, isCurrent);
      const deployedAtMs = resolveDeployedAtMs(h, deployment);
      const deployedAtIso = new Date(deployedAtMs).toISOString();
      const timeOfDay = Number.isFinite(deployedAtMs)
        ? new Date(deployedAtMs).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false })
        : "—";
      return { id: h.id, status, build, duration: durMs !== null ? formatDurationMs(durMs) : "—", time: formatDate(deployedAtIso), timeOfDay, isCurrent, rowLabel, source: h };
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
          onOpenConfigure={onOpenConfigure}
          historyLoading={historyLoading}
          historyError={historyError}
        />
      </div>
    </div>
  );
}
