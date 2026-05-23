import { useMemo, useRef, useState } from "react";
import { X, ExternalLink, TriangleAlert, CheckCircle2, RotateCw, Loader2, Copy, Check, Maximize2, Minimize2 } from "lucide-react";
import { isSensitiveEnvVar } from "@/lib/env-utils";
import { formatTimeAgo } from "@/lib/time-format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { WorkloadDetail, ServiceEndpointInfo, K8sEvent } from "@/lib/api";
import { useDeploymentEvents, useRestartPod } from "@/api/queries/deployments";
import { derivePodStatus } from "./PodTile";
import { PanelSection } from "../PanelSection";
import { PodLogsTab } from "./PodLogsTab";
import { PodMetricsTab } from "./PodMetricsTab";

const STATUS_CONFIG = {
  healthy: { dot: "bg-green-400", glow: "shadow-[0_0_6px_2px] shadow-green-400/50" },
  warning: { dot: "bg-amber-400", glow: "shadow-[0_0_6px_2px] shadow-amber-400/50" },
  unhealthy: { dot: "bg-red-400", glow: "shadow-[0_0_6px_2px] shadow-red-400/50" },
  pending: { dot: "bg-blue-400", glow: "shadow-[0_0_6px_2px] shadow-blue-400/50" },
} as const;

const TABS = ["General", "Logs", "Metrics", "Events"] as const;
type Tab = (typeof TABS)[number];

interface PodDetailPanelProps {
  workload: WorkloadDetail;
  deploymentId: string;
  /** Public-facing URLs from the deployment (external_urls). */
  externalUrls?: ServiceEndpointInfo[];
  onClose: () => void;
  expanded?: boolean;
  onToggleExpanded?: () => void;
}

export function PodDetailPanel({ workload, deploymentId, externalUrls, onClose, expanded, onToggleExpanded }: PodDetailPanelProps) {
  const [activeTab, setActiveTab] = useState<Tab>("General");

  return (
    <PodDetailPanelInner
      key={workload.name}
      workload={workload}
      deploymentId={deploymentId}
      externalUrls={externalUrls}
      onClose={onClose}
      expanded={expanded}
      onToggleExpanded={onToggleExpanded}
      activeTab={activeTab}
      setActiveTab={setActiveTab}
    />
  );
}

function PodDetailPanelInner({ workload, deploymentId, externalUrls, onClose, expanded, onToggleExpanded, activeTab, setActiveTab }: PodDetailPanelProps & { activeTab: Tab; setActiveTab: (tab: Tab) => void }) {
  const logsVisited = useRef(false);
  if (activeTab === "Logs") logsVisited.current = true;

  const { status, label: statusLabel } = derivePodStatus(workload);
  const name = workload.component || workload.name;

  return (
    <div className="flex h-full w-full flex-col rounded-md border border-border bg-card dark:bg-surface">
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-4">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-normal text-foreground">{name}</h2>
          <span className="flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1">
            <span className={cn("size-1.5 shrink-0 rounded-full", STATUS_CONFIG[status].dot, STATUS_CONFIG[status].glow)} />
            <span className="text-mono-sm text-muted-foreground">{statusLabel}</span>
          </span>
        </div>
        <div className="flex items-center gap-1">
          {onToggleExpanded && (
            <button
              onClick={onToggleExpanded}
              aria-label={expanded ? "Collapse pod details" : "Expand pod details"}
              className="flex items-center justify-center rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
            >
              {expanded ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
            </button>
          )}
          <button
            onClick={onClose}
            aria-label="Close pod details"
            className="flex items-center justify-center rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-border px-3">
        {TABS.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={cn(
              "relative px-3 py-2 text-body font-medium transition-colors",
              activeTab === tab
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground/70",
            )}
          >
            {tab}
            {activeTab === tab && (
              <span className="absolute inset-x-0 -bottom-px h-px bg-foreground/60" />
            )}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === "General" && (
        <div className="@container flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          <GeneralTab workload={workload} deploymentId={deploymentId} externalUrls={externalUrls} />
        </div>
      )}
      {logsVisited.current && (
        <div className={cn("min-h-0 flex-1 flex-col p-5", activeTab === "Logs" ? "flex" : "hidden")}>
          <PodLogsTab workload={workload} deploymentId={deploymentId} />
        </div>
      )}
      {activeTab === "Metrics" && (
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          <PodMetricsTab deploymentId={deploymentId} podName={workload.pod_name} />
        </div>
      )}
      {activeTab === "Events" && (
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-5">
          <EventsTab deploymentId={deploymentId} />
        </div>
      )}
    </div>
  );
}

function UrlRow({ url }: { url: string }) {
  const { copy, copied } = useCopyToClipboard();
  return (
    <div className="flex items-center gap-2 rounded border border-border px-3 py-2">
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        className="flex min-w-0 items-center gap-2 text-body text-muted-foreground transition-colors hover:text-foreground"
      >
        <ExternalLink className="size-4 shrink-0" />
        <span className="truncate">{url}</span>
      </a>
      <button
        onClick={() => void copy(url)}
        className="ml-auto flex shrink-0 items-center gap-1.5 rounded px-2 py-1 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        {copied ? <Check className="size-3.5 text-teal-400" /> : <Copy className="size-3.5" />}
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// General tab
// ---------------------------------------------------------------------------

interface GeneralTabProps {
  workload: WorkloadDetail;
  deploymentId: string;
  externalUrls?: ServiceEndpointInfo[];
}

function GeneralTab({ workload, deploymentId, externalUrls }: GeneralTabProps) {
  // Combine workload-level URLs with deployment external URLs for agent component.
  // Dedupe by url string — the deployment's external URL for the agent pod is
  // also present on the workload's own service endpoints, so concatenating
  // would otherwise show the same URL twice.
  const urls = useMemo(() => {
    const byUrl = new Map<string, ServiceEndpointInfo>();
    const add = (entry: ServiceEndpointInfo) => {
      if (!byUrl.has(entry.url)) byUrl.set(entry.url, entry);
    };
    if (workload.component === "agent" && externalUrls?.length) {
      externalUrls.forEach(add);
    }
    workload.urls?.forEach(add);
    return Array.from(byUrl.values());
  }, [workload, externalUrls]);

  // Group env vars per container, sorted alphabetically within each
  const envByContainer = useMemo(() => {
    return (workload.containers ?? []).map((container) => {
      const vars = (container.env ?? []).map((env) => {
        const value = env.value ?? "";
        const source = env.from ?? "static";
        return {
          name: env.name,
          value,
          source,
          secret: isSensitiveEnvVar(env.name, value, source),
        };
      }).sort((a, b) => a.name.localeCompare(b.name));
      return { containerName: container.name, vars };
    });
  }, [workload.containers]);

  return (
    <div className="flex flex-col gap-6">
      <PanelSection
        title="Domains"
        description="External URLs exposed by this pod."
        isEmpty={urls.length === 0}
        emptyState="No domains configured"
      >
        <div className="flex flex-col gap-2">
          {urls.map((url) => (
            <UrlRow key={url.url} url={url.url} />
          ))}
        </div>
      </PanelSection>

      <PanelSection
        title="Environment Variables"
        description="Variables injected into this pod on startup."
        isEmpty={envByContainer.every((c) => c.vars.length === 0)}
        emptyState="No environment variables"
      >
        <div className="flex flex-col gap-4">
          {envByContainer.map((container) => (
            container.vars.length > 0 && (
              <div key={container.containerName}>
                {envByContainer.length > 1 && (
                  <h4 className="mb-2 text-mono-sm font-medium text-muted-foreground">{container.containerName}</h4>
                )}
                <div className="grid grid-cols-[auto_1fr] overflow-hidden rounded border border-border @max-[450px]:grid-cols-1">
                  {container.vars.map((env, i) => (
                    <EnvVarRow key={env.name} name={env.name} value={env.value} secret={env.secret} isLast={i === container.vars.length - 1} />
                  ))}
                </div>
              </div>
            )
          ))}
        </div>
      </PanelSection>

      {workload.pod_name && (
        <PanelSection
          title="Danger Zone"
          description="Actions that may cause downtime."
        >
          <RestartPodButton deploymentId={deploymentId} podName={workload.pod_name} />
        </PanelSection>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Restart pod button
// ---------------------------------------------------------------------------

function RestartPodButton({ deploymentId, podName }: { deploymentId: string; podName: string }) {
  const restartMutation = useRestartPod();
  const isRestarting = restartMutation.isPending;

  return (
    <div className="flex items-center gap-3">
      <Button
        variant="destructive"
        size="sm"
        disabled={isRestarting}
        onClick={() => restartMutation.mutate({ deploymentId, podName })}
      >
        {isRestarting ? <Loader2 className="size-3.5 animate-spin" /> : <RotateCw className="size-3.5" />}
        {isRestarting ? "Restarting…" : "Restart Pod"}
      </Button>
      <p className="text-body-sm text-muted-foreground">
        This will terminate the pod and start a new one. There may be a brief interruption.
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Env var row
// ---------------------------------------------------------------------------

function EnvVarRow({ name, value, secret, isLast }: { name: string; value: string; secret: boolean; isLast: boolean }) {
  const borderClass = isLast ? "" : "border-b border-border/40";
  return (
    <>
      <div className={cn("bg-muted/50 px-3 py-3 text-mono-sm text-foreground/70", borderClass, "@max-[450px]:border-b-0 @max-[450px]:pb-1")}>
        {name}
      </div>
      <div className={cn("bg-muted/50 px-3 py-3 text-mono-sm text-muted-foreground break-all", borderClass, "@max-[450px]:pt-1")}>
        {secret ? "••••••••" : value}
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Placeholder tabs
// ---------------------------------------------------------------------------

function EventsTab({ deploymentId }: { deploymentId: string }) {
  const { data, isLoading } = useDeploymentEvents(deploymentId);
  const events = data?.events ?? [];

  if (isLoading) {
    return <p className="text-body-sm text-muted-foreground">Loading events…</p>;
  }

  if (events.length === 0) {
    return <p className="text-body-sm text-faint-foreground">No events</p>;
  }

  return (
    <div className="flex flex-col">
      {events.map((evt, i) => (
        <EventRow key={`${evt.reason}-${evt.object_name}-${i}`} event={evt} isLast={i === events.length - 1} />
      ))}
    </div>
  );
}

function EventRow({ event, isLast }: { event: K8sEvent; isLast: boolean }) {
  const isWarning = event.type === "Warning";

  return (
    <div className={cn(
      "flex gap-3 py-3",
      !isLast && "border-b border-border/40",
    )}>
      <div className="mt-0.5 shrink-0">
        {isWarning ? (
          <TriangleAlert className="size-4 text-amber-400" />
        ) : (
          <CheckCircle2 className="size-4 text-green-600 dark:text-green-400/60" />
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-baseline justify-between gap-2">
          <div className="flex items-baseline gap-2">
            <span className="text-mono-sm font-medium text-foreground">{event.reason}</span>
            <span className="text-mono-sm text-muted-foreground break-all">
              {event.object_kind}/{event.object_name}
              {event.count > 1 && ` ×${event.count}`}
            </span>
          </div>
          <span className="shrink-0 text-mono-sm text-faint-foreground">
            {formatTimeAgo(event.last_timestamp)}
          </span>
        </div>
        <p className="line-clamp-2 text-body-sm text-muted-foreground">{event.message}</p>
      </div>
    </div>
  );
}
