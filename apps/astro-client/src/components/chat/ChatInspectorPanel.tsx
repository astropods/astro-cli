import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ExternalLink, Wrench, X } from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { Button } from "@/components/ui/button";
import { type StatusIndicatorVariant } from "@/components/StatusIndicator";
import {
  useDeployment,
  useDeploymentRuntime,
  useDeploymentStatus,
} from "@/api/queries/deployments";
import {
  useObservabilitySummaries,
  useObservabilitySummary,
  useObservabilityTraces,
} from "@/api/queries/observability";
import { useDeploymentAgentConfig } from "@/api/queries/chat";
import { RequestSparkline, ZERO_SERIES } from "@/components/RequestSparkline";
import {
  derivePodStatus,
  POD_STATUS_STYLES,
} from "@/components/agent-detail/pods/PodTile";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  deriveChatComposerState,
  type ChatComposerState,
} from "@/lib/deployment-utils";
import { deploymentPath } from "@/lib/routes";
import { formatCompact, formatLatency } from "@/lib/format-utils";
import { cn } from "@/lib/utils";
import type {
  AgentDeploymentSummary,
  DeploymentStatus,
  TraceEntry,
  WorkloadDetail,
} from "@/lib/api";

export type ChatInspectorTab = "overview" | "settings";

const TABS: { id: ChatInspectorTab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "settings", label: "Settings" },
];

export function ChatInspectorPanel({
  account,
  deploymentId,
  deployment,
  tab,
  onTabChange,
  onClose,
}: {
  account: string;
  deploymentId: string;
  deployment?: AgentDeploymentSummary;
  tab: ChatInspectorTab;
  onTabChange: (tab: ChatInspectorTab) => void;
  onClose: () => void;
}) {
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const avatarUrl = avatarBust ?? getDeploymentAvatarUrl(deploymentId);
  const agentName = deployment?.display_name?.trim() || deployment?.name?.trim() || "Agent";
  const { data: statusData } = useDeploymentStatus(deploymentId);

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="flex h-[52px] shrink-0 items-stretch gap-3 pr-2 pl-4">
        <nav className="flex h-full items-stretch gap-4" aria-label="Inspector tabs">
          {TABS.map((t) => {
            const active = t.id === tab;
            return (
              <button
                key={t.id}
                type="button"
                onClick={() => onTabChange(t.id)}
                className={cn(
                  "relative flex items-center text-body-sm transition-colors",
                  active
                    ? "font-semibold text-foreground"
                    : "font-medium text-muted-foreground hover:text-foreground",
                )}
              >
                {t.label}
                {active ? (
                  <span className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-primary" />
                ) : null}
              </button>
            );
          })}
        </nav>
        <span className="flex-1" />
        <div className="flex items-center">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7 text-muted-foreground"
            aria-label="Hide panel"
            onClick={onClose}
          >
            <X className="size-4" />
          </Button>
        </div>
      </div>

      <div className="chat-thread-scroll min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-6 px-5 py-5">
          <AgentIdentity
            account={account}
            deploymentId={deploymentId}
            name={agentName}
            deploymentName={deployment?.name ?? ""}
            avatarUrl={avatarUrl}
            status={statusData}
          />
          {tab === "overview" ? (
            <OverviewTab deploymentId={deploymentId} account={account} />
          ) : (
            <SettingsTab deploymentId={deploymentId} />
          )}
        </div>
      </div>

      <div className="shrink-0 border-t border-border px-5 py-3">
        <Button asChild variant="outline" size="sm" className="w-full">
          <Link to={deploymentPath(account, deploymentId)}>
            <ExternalLink className="size-3.5" />
            View agent
          </Link>
        </Button>
      </div>
    </div>
  );
}

function AgentIdentity({
  account,
  deploymentId,
  name,
  deploymentName,
  avatarUrl,
  status,
}: {
  account: string;
  deploymentId: string;
  name: string;
  deploymentName: string;
  avatarUrl: string;
  status?: DeploymentStatus;
}) {
  const badge = statusBadge(status);
  return (
    <div className="flex items-center gap-3">
      <span className="relative inline-flex shrink-0">
        <BlueprintIdentity
          account={account}
          name={deploymentName}
          size={44}
          url={avatarUrl}
          className="size-11 rounded-xl"
        />
        <span className="absolute -right-0.5 -bottom-0.5 rounded-full bg-surface p-0.5">
          <span
            className={cn(
              "block size-2 rounded-full",
              statusDotClass(badge.variant),
              badge.pulse && "animate-pulse",
            )}
            aria-label={badge.label}
          />
        </span>
      </span>
      <div className="flex min-w-0 flex-col">
        <span className="truncate text-heading-4 text-foreground">{name}</span>
        <Link
          to={deploymentPath(account, deploymentId)}
          className="mt-0.5 inline-flex items-center gap-1 text-muted-foreground hover:text-foreground"
        >
          <span className="truncate font-mono text-mono-sm">
            {account}/{deploymentName}
          </span>
          <ExternalLink className="size-3 shrink-0" />
        </Link>
      </div>
    </div>
  );
}

function statusDotClass(variant: StatusIndicatorVariant): string {
  switch (variant) {
    case "success":
      return "bg-teal-500 dark:bg-teal-300";
    case "pending":
      return "bg-teal-700 dark:bg-teal-600";
    case "warning":
      return "bg-yellow-500 dark:bg-yellow-400";
    case "error":
      return "bg-coral-600 dark:bg-coral-400";
    default:
      return "bg-stone-500 dark:bg-stone-400";
  }
}

function OverviewTab({
  deploymentId,
  account,
}: {
  deploymentId: string;
  account: string;
}) {
  const { data: deploymentData } = useDeployment(deploymentId, true);
  const { data: runtimeData } = useDeploymentRuntime(deploymentId);
  const timeParams = useMemo(() => buildTimeParams(7), []);
  const { data: summary } = useObservabilitySummary(deploymentId, timeParams, {
    window: "7d",
  });
  const { data: summariesData } = useObservabilitySummaries(account);
  const summaryEntry = summariesData?.summaries?.[deploymentId];
  const requestSeries =
    summaryEntry?.request_series && summaryEntry.request_series.length > 1
      ? summaryEntry.request_series
      : ZERO_SERIES;
  const tokenSeries = summaryEntry?.token_series;
  const traceParams = useMemo(
    () => ({ ...timeParams, limit: "6" }),
    [timeParams],
  );
  const { data: tracesData } = useObservabilityTraces(deploymentId, traceParams, {
    window: "chat-inspector-7d",
  });

  const deployment = deploymentData?.deployment;
  const runtime = runtimeData?.runtime;

  const workloads = useMemo<WorkloadDetail[]>(() => {
    const specByName = new Map(
      (deployment?.workloads ?? []).map((w) => [w.name, w]),
    );
    const liveByName = new Map(
      (runtime?.workloads ?? []).map((w) => [w.name, w]),
    );
    const names = new Set<string>([...specByName.keys(), ...liveByName.keys()]);
    return Array.from(names).map((name) => ({
      kind: specByName.get(name)?.kind ?? "Pod",
      component: specByName.get(name)?.component ?? "",
      ...specByName.get(name),
      ...liveByName.get(name),
      name,
    }));
  }, [deployment?.workloads, runtime?.workloads]);

  const healthy = workloads.filter(
    (w) => derivePodStatus(w).status === "healthy",
  ).length;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2.5">
        <Kv k="Build" value={shortHash(deployment?.build_id)} mono />
        <Kv k="Last deployed" value={timeAgo(deployment?.updated_at)} />
        <Kv
          k="Workloads"
          value={
            workloads.length
              ? `${healthy} / ${workloads.length} healthy`
              : "—"
          }
        />
        {workloads.length > 0 ? (
          <div className="mt-1 flex flex-col gap-2">
            {workloads.map((w) => (
              <WorkloadRow key={w.name} workload={w} />
            ))}
          </div>
        ) : null}
      </div>

      <Section label="Usage">
        <RequestSparkline series={requestSeries} tokenSeries={tokenSeries} />
        <div className="grid grid-cols-4 gap-3 border-t border-border pt-3">
          <Stat k="Reqs" value={summary ? formatCompact(summary.total_traces) : "—"} />
          <Stat
            k="Tokens"
            value={summary ? formatCompact(summary.metrics.total_tokens) : "—"}
          />
          <Stat
            k="P95"
            value={summary ? formatLatency(summary.metrics.p95_latency_ms) : "—"}
          />
          <Stat
            k="Err"
            value={summary ? formatErrorRate(summary.metrics.error_rate) : "—"}
          />
        </div>
      </Section>

      <Section label="Recent traces">
        <RecentTraces traces={tracesData?.traces ?? []} />
      </Section>
    </div>
  );
}

const SETTINGS_UNAVAILABLE_NOTICE: Record<
  Exclude<ChatComposerState, "ready">,
  string
> = {
  unknown: "Checking agent status…",
  paused: "Agent is paused — resume it to view its configuration.",
  stopped: "Agent isn't running.",
  starting: "Agent is starting…",
  error: "Agent is in an error state.",
  unreachable: "Agent isn't reachable right now.",
};

function SettingsTab({ deploymentId }: { deploymentId: string }) {
  const { data: status } = useDeploymentStatus(deploymentId);
  const { data: runtimeData, isError: runtimeError } =
    useDeploymentRuntime(deploymentId);
  const state = deriveChatComposerState(status, runtimeData?.runtime);
  // Pessimistic gate: only fetch once BOTH the status and the runtime
  // (messaging reachability) have *settled* AND the agent is ready. Deriving
  // `ready` while either is still loading would fire agent/config against a
  // possibly-paused/unreachable sidecar — the exact request this view exists to
  // avoid. While loading, `state` is "ready" optimistically but `resolved` is
  // false, so we surface the "unknown" notice rather than issuing the request.
  //
  // A runtime *error* counts as settled (not still-loading): the runtime read
  // can persistently 503 (e.g. K8s briefly unreachable) while the agent itself
  // is healthy, and pinning the tab on "Checking…" forever would be worse than
  // attempting the (hardened, fail-fast) request as the old code always did.
  const resolved = !!status && (!!runtimeData || runtimeError);
  const ready = resolved && state === "ready";
  // Only hit the messaging proxy (agent/config) when the agent is actually
  // reachable — otherwise the proxy hangs and 5xxs on an unresponsive sidecar.
  const { data: config, isLoading, isError } = useDeploymentAgentConfig(
    deploymentId,
    ready,
  );
  const [expanded, setExpanded] = useState(false);

  if (!ready) {
    const noticeKey = state === "ready" ? "unknown" : state;
    return (
      <p className="text-body-sm text-muted-foreground">
        {SETTINGS_UNAVAILABLE_NOTICE[noticeKey]}
      </p>
    );
  }
  if (isLoading) {
    return <p className="text-body-sm text-muted-foreground">Loading configuration…</p>;
  }
  if (isError || !config) {
    return (
      <p className="text-body-sm text-muted-foreground">
        Configuration is unavailable for this agent.
      </p>
    );
  }

  const prompt = config.systemPrompt?.trim();
  const tools = config.tools ?? [];
  const longPrompt = !!prompt && prompt.length > 280;

  return (
    <div className="flex flex-col gap-6">
      <Section label="System prompt">
        <div className="rounded-xl border border-border bg-surface/60 px-3.5 py-3">
          {prompt ? (
            <>
              <p
                className={cn(
                  "font-mono text-mono-sm leading-relaxed whitespace-pre-wrap text-muted-foreground",
                  !expanded && longPrompt && "line-clamp-5",
                )}
              >
                {prompt}
              </p>
              {longPrompt ? (
                <button
                  type="button"
                  onClick={() => setExpanded((e) => !e)}
                  className="mt-2 text-body-sm font-medium text-muted-foreground hover:text-foreground"
                >
                  {expanded ? "Show less" : "Show more"}
                </button>
              ) : null}
            </>
          ) : (
            <p className="text-body-sm text-muted-foreground">
              No system prompt configured.
            </p>
          )}
        </div>
      </Section>

      <Section label={`Tools${tools.length ? ` · ${tools.length}` : ""}`}>
        {tools.length === 0 ? (
          <p className="text-body-sm text-muted-foreground">No tools configured.</p>
        ) : (
          <div className="flex flex-col gap-3.5">
            {tools.map((tool, i) => (
              <div key={`${tool.name}-${i}`} className="flex items-start gap-2.5">
                <Wrench className="mt-0.5 size-3.5 shrink-0 text-primary" />
                <div className="flex min-w-0 flex-col gap-0.5">
                  <span className="font-mono text-mono-sm text-foreground">
                    {tool.title || tool.name}
                  </span>
                  {tool.description ? (
                    <span className="text-body-sm leading-snug text-muted-foreground">
                      {tool.description}
                    </span>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </Section>
    </div>
  );
}

function WorkloadRow({ workload }: { workload: WorkloadDetail }) {
  const { status } = derivePodStatus(workload);
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="flex min-w-0 items-center gap-2.5">
        <span
          className={cn(
            "size-1.5 shrink-0 rounded-full",
            POD_STATUS_STYLES[status].dot,
          )}
        />
        <span className="truncate font-mono text-mono-sm text-muted-foreground">
          {workload.name}
        </span>
      </span>
      {workload.age ? (
        <span className="shrink-0 font-mono text-mono-sm text-faint-foreground">
          {workload.age}
        </span>
      ) : null}
    </div>
  );
}

const RECENT_TRACES_LIMIT = 6;

function RecentTraces({ traces }: { traces: TraceEntry[] }) {
  const recent = traces.slice(0, RECENT_TRACES_LIMIT);
  if (recent.length === 0) {
    return (
      <p className="text-body-sm text-muted-foreground">No recent traces.</p>
    );
  }
  return (
    <div className="flex flex-col">
      {recent.map((t, i) => {
        const failed = isFailedTrace(t.status);
        return (
          <div
            key={t.trace_id}
            className={cn(
              "flex items-center gap-2.5 py-2",
              i > 0 && "border-t border-border/60",
            )}
          >
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full",
                failed
                  ? "bg-coral-600 dark:bg-coral-400"
                  : "bg-teal-500 dark:bg-teal-300",
              )}
            />
            <span className="font-mono text-mono-sm text-muted-foreground">
              {shortHash(t.trace_id)}
            </span>
            <span className="min-w-0 flex-1 truncate text-body-sm text-faint-foreground">
              {t.user_details?.display_name ||
                t.user_details?.username ||
                t.user_id ||
                "—"}
            </span>
            <span
              className={cn(
                "shrink-0 font-mono text-mono-sm",
                failed ? "text-coral-600 dark:text-coral-400" : "text-foreground",
              )}
            >
              {failed ? "Failed" : formatLatency(t.latency_ms)}
            </span>
            <span className="shrink-0 text-mono-sm text-faint-foreground">
              {timeAgo(t.timestamp)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function Section({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <span className="font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
        {label}
      </span>
      {children}
    </div>
  );
}

function Kv({ k, value, mono }: { k: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-body-sm text-muted-foreground">{k}</span>
      <span
        className={cn(
          "truncate text-body-sm font-medium text-foreground",
          mono && "font-mono",
        )}
      >
        {value}
      </span>
    </div>
  );
}

function Stat({ k, value }: { k: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className="font-mono text-mono-sm uppercase tracking-wide text-faint-foreground">
        {k}
      </span>
      <span className="truncate font-mono text-body-sm text-foreground">
        {value}
      </span>
    </div>
  );
}

function statusBadge(status?: DeploymentStatus): {
  variant: StatusIndicatorVariant;
  label: string;
  pulse: boolean;
} {
  if (!status) return { variant: "muted", label: "Unknown", pulse: false };
  if (status.reason === "paused") return { variant: "muted", label: "Paused", pulse: false };
  switch (status.value) {
    case "active":
      return { variant: "success", label: "Active", pulse: true };
    case "deploying":
      return { variant: "pending", label: "Deploying", pulse: true };
    case "undeploying":
      return { variant: "pending", label: "Stopping", pulse: true };
    case "error":
      return { variant: "error", label: "Error", pulse: false };
    case "inactive":
      return { variant: "muted", label: "Inactive", pulse: false };
    default:
      return { variant: "muted", label: status.value, pulse: false };
  }
}

function isFailedTrace(status?: string): boolean {
  if (!status) return false;
  const s = status.toLowerCase();
  return s !== "success" && s !== "ok" && s !== "default";
}

function shortHash(value?: string): string {
  if (!value) return "—";
  return value.slice(0, 8);
}

function formatErrorRate(rate: number): string {
  if (rate === 0) return "0%";
  if (rate < 0.001) return "<0.1%";
  return `${(rate * 100).toFixed(rate < 0.1 ? 2 : 1)}%`;
}

function buildTimeParams(days: number): Record<string, string> {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - days);
  return {
    start_time: start.toISOString(),
    end_time: end.toISOString(),
    granularity: "hour",
  };
}

function timeAgo(iso?: string): string {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "—";
  const diff = Date.now() - then;
  if (diff < 0) return "just now";
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d`;
  const weeks = Math.floor(days / 7);
  if (weeks < 5) return `${weeks}w`;
  const months = Math.floor(days / 30);
  return `${months}mo`;
}
