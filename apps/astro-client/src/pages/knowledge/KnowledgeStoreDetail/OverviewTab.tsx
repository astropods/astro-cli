import { CheckCircleIcon } from "@heroicons/react/24/solid";
import { Bot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorPanel } from "@/components/ui/status-panel";
import { MetricCard } from "@/components/MetricCard";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { useKnowledgeMetrics } from "@/api/queries/knowledge";
import { formatBytes, formatUptime, formatCPU } from "@/lib/format-utils";
import { Tag } from "@/components/Tag";
import type { KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EventTimeline } from "./EventTimeline";
import { BindingsGraph } from "./BindingsGraph";

export function OverviewTab({ store, account, onViewLogs }: { store: KnowledgeStore; account: string; onViewLogs: () => void }) {
  const isReady = store.status === "ready";
  const { data: metrics, isLoading: metricsLoading } = useKnowledgeMetrics(account, store.name, isReady);

  const cpuValue = metrics?.cpu_cores != null ? formatCPU(metrics.cpu_cores) : "—";
  const memValue = metrics?.memory_bytes != null ? formatBytes(metrics.memory_bytes) : "—";
  const storageUsed = metrics?.storage_used != null ? formatBytes(metrics.storage_used) : "—";
  const storageSuffix = metrics?.storage_total != null ? `/ ${formatBytes(metrics.storage_total)}` : (store.storage ? `/ ${store.storage}` : undefined);
  const uptimeValue = metrics?.uptime_seconds != null ? formatUptime(metrics.uptime_seconds) : "—";

  return (
    <div className="space-y-6">
      {store.status === "error" && store.error && (
        <ErrorPanel>{store.error}</ErrorPanel>
      )}

      {store.endpoint && <PrivateLinkSection store={store} />}

      {store.status !== "pending-acceptance" && <>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <MetricCard label="CPU" value={cpuValue} showTrend={false} loading={metricsLoading} className="bg-white dark:bg-card" />
          <MetricCard label="Memory" value={memValue} showTrend={false} loading={metricsLoading} className="bg-white dark:bg-card" />
          <MetricCard label="Storage" value={storageUsed} valueSuffix={storageSuffix} showTrend={false} loading={metricsLoading} className="bg-white dark:bg-card" />
          <MetricCard label="Uptime" value={uptimeValue} showTrend={false} loading={metricsLoading} className="bg-white dark:bg-card"
            description={metrics?.uptime_seconds != null ? <span className="flex items-center gap-1.5 text-body-sm text-muted-foreground"><CheckCircleIcon className="size-3.5 shrink-0 text-teal-600" />No restarts detected</span> : undefined}
          />
        </div>

        <div className={cn("grid gap-8", store.mode === "managed" && "lg:grid-cols-[1fr_420px]")}>
          <div className="rounded-md border border-border bg-white dark:bg-card overflow-hidden">
            <div className="flex items-center gap-2 px-5 py-3 border-b border-border bg-stone-200 dark:bg-muted">
              <h3 className="text-heading-4 text-foreground">Agent bindings</h3>
              <Tag>{store.bound_agents?.length ?? 0}</Tag>
            </div>
            {store.bound_agents && store.bound_agents.length > 0 ? (
              <div className="p-5">
                <BindingsGraph storeName={store.name} provider={store.provider} agents={store.bound_agents} />
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Bot className="size-10 text-muted-foreground" />
                <p className="mt-3 text-body-sm text-foreground font-medium">No agents are bound to this store yet.</p>
                <p className="mt-1 text-body-sm text-muted-foreground">
                  Add a <code className="font-mono text-mono-sm">knowledge</code> block in your astropods.yml to bind an agent.
                </p>
              </div>
            )}
          </div>

          {store.mode === "managed" && (
            <div className="flex flex-col">
              <div className="rounded-md border border-border bg-surface overflow-hidden">
                <div className="flex items-center justify-between px-5 py-3 border-b border-border bg-stone-200 dark:bg-muted">
                  <h3 className="text-heading-4 text-foreground">Event log</h3>
                  <Button variant="ghost" size="sm" onClick={onViewLogs}>View logs</Button>
                </div>
                <EventTimeline store={store} />
              </div>
            </div>
          )}
        </div>
      </>}
    </div>
  );
}
