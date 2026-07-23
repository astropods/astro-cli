import { useEffect, useMemo, useRef, useState } from "react";
import { X, ExternalLink, ChevronDown, RotateCw, Loader2, Copy, Check, Maximize2, Minimize2 } from "lucide-react";
import { ErrorPanel } from "@/components/ui/status-panel";
import { ContainerLogErrorProbe, firstContainerError, useContainerErrors } from "./use-container-log-errors";
import { isSensitiveEnvVar, roleFor } from "@/lib/env-utils";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { WorkloadDetail, ServiceEndpointInfo, K8sEvent, ContainerStatus } from "@/lib/api";
import { useDeploymentEvents, useRestartPod } from "@/api/queries/deployments";
import { POD_STATUS_STYLES, resolvePodStatus } from "./PodTile";
import { PanelSection } from "../PanelSection";
import { PodLogsTab } from "./PodLogsTab";
import { PodMetricsTab } from "./PodMetricsTab";

const TABS = ["General", "Logs", "Metrics", "Events"] as const;
type Tab = (typeof TABS)[number];

interface PodDetailPanelProps {
  workload: WorkloadDetail;
  deploymentId: string;
  /** Public-facing URLs from the deployment (external_urls). */
  externalUrls?: ServiceEndpointInfo[];
  /** True when the parent deployment is paused. Mirrors PodTile status precedence. */
  paused?: boolean;
  /** True while the runtime query has not returned. Mirrors PodTile status precedence. */
  probing?: boolean;
  onClose: () => void;
  expanded?: boolean;
  onToggleExpanded?: () => void;
}

export function PodDetailPanel({ workload, deploymentId, externalUrls, paused, probing, onClose, expanded, onToggleExpanded }: PodDetailPanelProps) {
  const [activeTab, setActiveTab] = useState<Tab>("General");

  return (
    <PodDetailPanelInner
      key={workload.name}
      workload={workload}
      deploymentId={deploymentId}
      externalUrls={externalUrls}
      paused={paused}
      probing={probing}
      onClose={onClose}
      expanded={expanded}
      onToggleExpanded={onToggleExpanded}
      activeTab={activeTab}
      setActiveTab={setActiveTab}
    />
  );
}

function PodDetailPanelInner({ workload, deploymentId, externalUrls, paused, probing, onClose, expanded, onToggleExpanded, activeTab, setActiveTab }: PodDetailPanelProps & { activeTab: Tab; setActiveTab: (tab: Tab) => void }) {
  const logsVisited = useRef(false);
  if (activeTab === "Logs") logsVisited.current = true;

  const { status, label: statusLabel } = resolvePodStatus(workload, { paused, probing });
  const name = workload.component || workload.name;

  // Detect error-level logs for this pod (reuses the same cached queries the
  // tile indicator uses). When present, open straight to the Logs tab and show
  // the error as a banner, so the user lands on something useful instead of an
  // empty General tab.
  const { byContainer, report } = useContainerErrors();
  const isLongRunning = workload.kind === "Deployment" || workload.kind === "StatefulSet";
  const probeContainers = isLongRunning && !paused && !probing ? workload.containers ?? [] : [];
  const logErrorMessage = firstContainerError(byContainer, probeContainers.map((c) => c.name));

  useEffect(() => {
    // Auto-open the Logs tab once when the pod has errors and the user has not
    // navigated tabs yet. The query is already warm from the tile, so this
    // resolves immediately rather than flashing the General tab.
    if (logErrorMessage && !logsVisited.current && activeTab === "General") {
      setActiveTab("Logs");
    }
  }, [logErrorMessage, activeTab, setActiveTab]);

  return (
    <div className="flex h-full w-full flex-col rounded-md border border-border bg-card dark:bg-surface">
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-4">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-normal text-foreground">{name}</h2>
          <span className="flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1">
            <span className={cn("size-1.5 shrink-0 rounded-full", POD_STATUS_STYLES[status].dot, POD_STATUS_STYLES[status].glow)} />
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

      {probeContainers.map((c) => (
        <ContainerLogErrorProbe
          key={c.name}
          deploymentId={deploymentId}
          workloadName={workload.name}
          container={c.name}
          onResult={report}
        />
      ))}

      {logErrorMessage && (
        <div className="px-5 pt-4">
          <ErrorPanel title="Errors in logs">{logErrorMessage}</ErrorPanel>
        </div>
      )}

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

  // Group env vars per container, sorted alphabetically within each.
  // Env lives on the record (workload.env, keyed by role) — runtime
  // containers contribute only their name + live status. roleFor maps
  // (component, container.name) → role to pick the right entry.
  // env.is_secret comes from deployment_build_env and is authoritative;
  // isSensitiveEnvVar is a defensive name-heuristic fallback.
  const envByContainer = useMemo(() => {
    const envByRole = workload.env ?? {};
    return (workload.containers ?? []).map((container) => {
      const role = roleFor(workload.component, container.name);
      const entries = role ? envByRole[role] ?? [] : [];
      const vars = entries.map((env) => {
        const value = env.value ?? "";
        return {
          name: env.name,
          value,
          secret: env.is_secret ?? isSensitiveEnvVar(env.name, value, ""),
        };
      }).sort((a, b) => a.name.localeCompare(b.name));
      return { containerName: container.name, vars };
    });
  }, [workload.containers, workload.component, workload.env]);

  const containers = workload.containers ?? [];

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

      {containers.length > 0 && (
        <PanelSection title="Containers" description="Status and environment of each container in this pod.">
          <div className="flex flex-col gap-4">
            {containers.map((c) => (
              <ContainerCard
                key={c.name}
                container={c}
                vars={envByContainer.find((e) => e.containerName === c.name)?.vars ?? []}
              />
            ))}
          </div>
        </PanelSection>
      )}

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
// Container card — status header + environment
// ---------------------------------------------------------------------------

interface EnvVar { name: string; value: string; secret: boolean }

function ContainerCard({ container, vars }: { container: ContainerStatus; vars: EnvVar[] }) {
  const state = container.state?.toLowerCase();
  const problem = !container.ready && (state === "waiting" || state === "terminated");
  const square = container.ready
    ? "bg-green-500 dark:bg-green-400"
    : problem
      ? "bg-red-400 dark:bg-red-400"
      : "bg-blue-400 dark:bg-blue-400";

  const statusText = [
    container.ready ? "Ready" : container.state,
    container.restart_count > 0 ? `${container.restart_count} restart${container.restart_count > 1 ? "s" : ""}` : "",
  ].filter(Boolean).join(" · ");

  return (
    <div className="overflow-hidden rounded border border-border">
      <div className="flex items-center gap-2 px-3 py-2.5">
        <span className={cn("size-3 shrink-0 rounded-sm", square)} />
        <span className="text-mono-sm text-foreground">{container.name}</span>
        <span className="ml-auto text-mono-sm text-muted-foreground">{statusText}</span>
      </div>
      {container.message && (
        <p className={cn("px-3 pb-2.5 text-body-sm", problem ? "text-red-500 dark:text-red-400" : "text-muted-foreground")}>
          {container.message}
        </p>
      )}
      {vars.length > 0 && (
        <div className="grid grid-cols-[auto_1fr] border-t border-border @max-[450px]:grid-cols-1">
          {vars.map((env, i) => (
            <EnvVarRow key={env.name} name={env.name} value={env.value} secret={env.secret} isLast={i === vars.length - 1} />
          ))}
        </div>
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
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  if (isLoading) {
    return <p className="text-body-sm text-muted-foreground">Loading events…</p>;
  }
  if (events.length === 0) {
    return <p className="text-body-sm text-faint-foreground">No events</p>;
  }

  const toggle = (i: number) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-3 border-b border-border px-2 py-2 text-label uppercase text-faint-foreground">
        <span className="min-w-0 flex-1">Summary</span>
        <span className="w-14 text-right">Count</span>
        <span className="w-12 text-right">Age</span>
        <span className="w-4 shrink-0" />
      </div>
      {events.map((evt, i) => (
        <EventRow key={i} event={evt} expanded={expanded.has(i)} onToggle={() => toggle(i)} />
      ))}
    </div>
  );
}

// Left-bar color has three levels: red for stuck states that need action
// (e.g. ImagePullBackOff), amber for other Warnings, and muted for Normal events
// (including back-offs, which aren't errors on their own).
function eventBarColor(event: K8sEvent): string {
  if (event.severity === "stuck") return "bg-red-400 dark:bg-red-400";
  if (event.type === "Warning") return "bg-amber-400 dark:bg-amber-400";
  return "bg-muted-foreground/40";
}

// compactAge renders a K8s-style short duration since an RFC3339 timestamp, e.g.
// "5d1h", "3h20m", "45m", "30s".
function compactAge(iso?: string): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  let s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  const d = Math.floor(s / 86400); s -= d * 86400;
  const h = Math.floor(s / 3600); s -= h * 3600;
  const m = Math.floor(s / 60); s -= m * 60;
  if (d > 0) return h > 0 ? `${d}d${h}h` : `${d}d`;
  if (h > 0) return m > 0 ? `${h}h${m}m` : `${h}h`;
  if (m > 0) return `${m}m`;
  return `${s}s`;
}

function EventRow({ event, expanded, onToggle }: { event: K8sEvent; expanded: boolean; onToggle: () => void }) {
  // Summary is the friendly humanized line — never the raw K8s message/code. The
  // color bar (type) and the expanded detail distinguish otherwise-similar events.
  const summary = event.title || event.reason;
  return (
    <div className="border-b border-border/50">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="flex w-full items-center gap-3 px-2 py-2.5 text-left transition-colors hover:bg-muted/40"
      >
        <span className={cn("h-6 w-1 shrink-0 rounded-full", eventBarColor(event))} />
        <span className="min-w-0 flex-1 truncate text-body-sm text-foreground" title={summary}>{summary}</span>
        <span className="w-14 shrink-0 text-right text-mono-sm text-muted-foreground">{event.count > 0 ? event.count : ""}</span>
        <span className="w-12 shrink-0 text-right text-mono-sm text-muted-foreground">{compactAge(event.last_timestamp)}</span>
        <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} />
      </button>
      {expanded && (
        <div className="flex flex-col gap-2 pb-3 pl-4 pr-2">
          {event.guidance && <p className="text-body-sm text-foreground/80">{event.guidance}</p>}
          <p className="whitespace-pre-line break-words text-mono-sm text-muted-foreground">{event.message}</p>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-mono-sm text-faint-foreground">
            <span>{event.reason}</span>
            <span>{event.object_kind}/{event.object_name}</span>
            {event.first_timestamp && <span>First seen {compactAge(event.first_timestamp)} ago</span>}
          </div>
        </div>
      )}
    </div>
  );
}
