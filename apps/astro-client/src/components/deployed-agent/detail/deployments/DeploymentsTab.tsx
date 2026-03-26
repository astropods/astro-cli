import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useDeploymentHistory } from "@/api/queries/deployments";
import { formatDate, isDeployingState } from "@/lib/deployment-utils";
import type { AgentDeployment, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { deploymentKeys } from "@/api/queries/keys";
import { cn } from "@/lib/utils";
import { useCompactLayout } from "@/hooks/use-compact-layout";
import { DeploymentHistoryTable } from "./DeploymentHistoryTable";
import {
  formatDurationMs,
  resolveDeployedAtMs,
  deploymentHistoryDurationMs,
  deploymentHistoryUiStatus,
} from "./history/utils";
import type { DeploymentHistoryTableRow } from "./history/types";

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
  }, [deployment]);

  const totalServiceCount = serviceRows.length;

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

    return merged.map((h, idx) => {
      const isCurrent = h.id === deployment.id;
      const status = deploymentHistoryUiStatus(h, deployment);
      const build = h.build_id?.slice(0, 8) || "—";
      const rowLabel = isCurrent ? deployment.display_name || deployment.name : `${deployment.name} · ${build}`;
      const durMs = deploymentHistoryDurationMs(h, idx, merged, deployment, isCurrent);
      const deployedAtIso = new Date(resolveDeployedAtMs(h, deployment)).toISOString();
      return { id: h.id, status, build, duration: durMs !== null ? formatDurationMs(durMs) : "—", time: formatDate(deployedAtIso), isCurrent, rowLabel, source: h };
    });
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
        {/* Summary cards */}
        <div className="grid grid-cols-4 gap-2.5">
          {[
            { label: "CURRENT BUILD", value: deployment.build_id?.slice(0, 8) || "—", wrap: false },
            {
              label: "DEPLOYMENT STATUS",
              value: String(deployment.status || "unknown").charAt(0).toUpperCase() + String(deployment.status || "unknown").slice(1).toLowerCase(),
              wrap: false,
            },
            {
              label: "DEPLOYED ON",
              value: deployment.created_at
                ? `${formatDate(deployment.created_at)},${isCompact ? "\n" : " "}${new Date(deployment.created_at).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`
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

        {/* History table */}
        <DeploymentHistoryTable
          currentRow={currentRow}
          pastRows={pastRows}
          serviceRows={serviceRows}
          deploymentId={deployment.id}
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
