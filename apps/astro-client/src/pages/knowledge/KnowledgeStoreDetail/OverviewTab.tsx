import { Bot } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorPanel } from "@/components/ui/status-panel";
import { MetricCard } from "@/components/MetricCard";
import { PrivateLinkSection } from "@/components/knowledge/PrivateLinkSection";
import { useSupabaseProjectHealth } from "@/api/queries/supabase";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { PROVIDER_LABELS, displayProvider, supabaseHealthLabel } from "@/components/knowledge/knowledge-utils";
import { formatUptime } from "@/lib/format-utils";
import { Tag } from "@/components/Tag";
import type { KnowledgeStore } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EventTimeline } from "./EventTimeline";
import { BindingsGraph } from "./BindingsGraph";

export function OverviewTab({ store, account, onViewLogs }: { store: KnowledgeStore; account: string; onViewLogs: () => void }) {
  // Live Supabase project health for Supabase-origin stores (Tier-1 metric).
  const supabaseRef = store.annotations?.source === "supabase" ? (store.annotations.supabase_project_id ?? "") : "";
  const { data: supabaseHealthData } = useSupabaseProjectHealth(account, supabaseRef, { enabled: !!supabaseRef });
  const supabaseDb =
    supabaseHealthData?.services?.find((s) => s.name === "db") ?? supabaseHealthData?.services?.[0];
  const supabaseHealthy = supabaseDb ? (supabaseDb.healthy ?? supabaseDb.status === "ACTIVE_HEALTHY") : false;
  const supabaseRegion = supabaseRef ? store.annotations?.region : undefined;
  const supabaseName = supabaseRef ? store.annotations?.supabase_project_name : undefined;

  // Knowledge stores are external now (no self-hosted CPU/memory/storage to
  // report), so uptime is the store's age, derived from created_at.
  const uptimeValue = formatUptime(Math.max(0, (Date.now() - new Date(store.created_at).getTime()) / 1000));
  const agentCount = store.bound_agents?.length ?? 0;
  const typeValue = PROVIDER_LABELS[displayProvider(store)] ?? store.provider;

  return (
    <div className="space-y-6">
      {store.status === "error" && store.error && (
        <ErrorPanel>{store.error}</ErrorPanel>
      )}

      {store.endpoint && <PrivateLinkSection store={store} account={account} />}

      {store.status !== "pending-acceptance" && <>
        {supabaseRef && (
          <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-card px-5 py-3">
            <div className="flex min-w-0 items-center gap-2.5">
              <ProviderIcon provider="supabase" className="size-4 shrink-0" />
              <h3 className="text-heading-4 text-foreground shrink-0">Supabase project</h3>
              {supabaseName && <span className="truncate text-body-sm text-foreground">{supabaseName}</span>}
              <code className="shrink-0 text-mono-sm text-muted-foreground">{supabaseRef}</code>
            </div>
            {supabaseDb && (
              <span className="inline-flex shrink-0 items-center gap-1.5 text-body-sm">
                <span
                  className={cn("size-1.5 shrink-0 rounded-full", supabaseHealthy ? "bg-success" : "bg-destructive")}
                  aria-hidden
                />
                <span className={supabaseHealthy ? "text-success" : "text-destructive"}>
                  {supabaseHealthLabel(supabaseDb.status, supabaseDb.healthy)}
                </span>
              </span>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <MetricCard label="Uptime" value={uptimeValue} showTrend={false} />
          <MetricCard label="Agents" value={String(agentCount)} showTrend={false} />
          <MetricCard label="Type" value={typeValue} showTrend={false} />
          {supabaseRegion && <MetricCard label="Region" value={supabaseRegion} showTrend={false} />}
        </div>

        <div className={cn("grid gap-3", store.mode === "managed" && "lg:grid-cols-[1fr_420px]")}>
          <div className="overflow-hidden rounded-md border border-border bg-card">
            <div className="flex items-center gap-2 px-5 py-3 border-b border-border">
              <h3 className="text-heading-4 text-foreground">Agent bindings</h3>
              <Tag>{store.bound_agents?.length ?? 0}</Tag>
            </div>
            {store.bound_agents && store.bound_agents.length > 0 ? (
              <div className="p-5">
                <BindingsGraph storeName={store.name} provider={store.provider} agents={store.bound_agents} />
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <div className="flex justify-center mb-3 text-muted-foreground">
                  <Bot className="size-6" />
                </div>
                <p className="text-sm font-medium text-foreground">No agents bound</p>
                <p className="text-xs text-muted-foreground mt-1">
                  Select this store as a shared database when deploying an agent.
                </p>
              </div>
            )}
          </div>

          {store.mode === "managed" && (
            <div className="flex flex-col">
              <div className="overflow-hidden rounded-md border border-border bg-card">
                <div className="flex items-center justify-between px-5 py-3 border-b border-border">
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
