import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import {
  AlertCircle,
  Activity,
  ArrowUpRight,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  NotepadText,
  ScrollText,
  TimerReset,
  Wrench,
  X,
  type LucideIcon,
} from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { ChatPanelSectionHeader } from "@/components/chat/ChatPanelSectionHeader";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAccountMembers } from "@/api/queries/accounts";
import {
  useDeploymentRuntime,
  useDeploymentStatus,
} from "@/api/queries/deployments";
import {
  useObservabilitySummaries,
  useObservabilitySummary,
  useObservabilityTraces,
} from "@/api/queries/observability";
import { useDeploymentAgentConfig } from "@/api/queries/chat";
import { DeploymentFilesPanel } from "./DeploymentFilesPanel";
import { RequestSparkline, ZERO_SERIES } from "@/components/RequestSparkline";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import {
  identityRefFromUserID,
  slackIdentityDisplay,
} from "@/components/activity/insights-user-identity";
import type { DayRange } from "@/components/agent-detail/charts/chart-utils";
import { DeploymentStatusBadge } from "@/components/agent-detail/deployments/DeploymentStatusBadge";
import {
  type TraceStatus,
  STATUS_CONFIG,
  formatCost as formatTraceCost,
  normalizeStatus,
} from "@/components/agent-detail/traces/trace-utils";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  deriveChatComposerState,
  type ChatComposerState,
} from "@/lib/deployment-utils";
import {
  DeploymentTab,
  deploymentPath,
  deploymentTracesPath,
  deploymentTracePath,
} from "@/lib/routes";
import {
  formatCompact,
  formatCost as formatAggregateCost,
} from "@/lib/format-utils";
import { cn } from "@/lib/utils";
import type {
  AgentDeploymentSummary,
  DeploymentStatus,
  DeploymentStatusValue,
  TraceEntry,
  UserDetails,
} from "@/lib/api";

export type ChatInspectorTab = "overview" | "settings" | "files";

const TABS: { id: ChatInspectorTab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "files", label: "Files" },
  { id: "settings", label: "Config" },
];
const USAGE_RANGES: { key: DayRange; label: string; days: number }[] = [
  { key: "7d", label: "7D", days: 7 },
  { key: "14d", label: "14D", days: 14 },
  { key: "30d", label: "30D", days: 30 },
];
const DEFAULT_USAGE_RANGE: DayRange = "30d";

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

  // Hide the Files tab entirely unless the agent supports files (the sidecar has
  // storage and the agent declared it). Cached + shared with the composer/settings
  // fetch, so no extra request. If Files is the active tab when it disappears,
  // fall back to Overview.
  const { data: agentConfig } = useDeploymentAgentConfig(deploymentId);
  const filesEnabled = agentConfig?.capabilities?.files === true;
  const visibleTabs = filesEnabled ? TABS : TABS.filter((t) => t.id !== "files");
  useEffect(() => {
    if (tab === "files" && !filesEnabled) onTabChange("overview");
  }, [tab, filesEnabled, onTabChange]);

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      <div className="shrink-0 border-b border-border">
        <div className="flex justify-center pt-2 md:hidden" aria-hidden="true">
          <span className="h-1 w-12 rounded-full bg-muted-foreground/30" />
        </div>
        <div className="flex items-start gap-3 px-5 pt-4">
          <AgentIdentity
            account={account}
            deploymentId={deploymentId}
            name={agentName}
            deploymentName={deployment?.name ?? ""}
            avatarUrl={avatarUrl}
            status={statusData}
            className="flex-1"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="-mt-1 -mr-1 size-7 shrink-0 text-muted-foreground"
            aria-label="Hide panel"
            onClick={onClose}
          >
            <X className="size-4" />
          </Button>
        </div>
        <InspectorTabs tabs={visibleTabs} tab={tab} onTabChange={onTabChange} />
      </div>

      <div className="chat-thread-scroll min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-7 px-5 py-5">
          {tab === "files" && filesEnabled ? (
            <DeploymentFilesPanel deploymentId={deploymentId} />
          ) : tab === "settings" ? (
            <SettingsTab key={deploymentId} deploymentId={deploymentId} />
          ) : (
            <OverviewTab
              deploymentId={deploymentId}
              account={account}
            />
          )}
        </div>
      </div>

      <div className="shrink-0 border-t border-border px-5 pt-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] md:py-3">
        <Button asChild variant="outline" className="w-full">
          <Link to={deploymentPath(account, deploymentId, DeploymentTab.Monitor)}>
            View agent
            <ArrowUpRight className="size-3.5 shrink-0" />
          </Link>
        </Button>
      </div>
    </div>
  );
}

function InspectorTabs({
  tabs,
  tab,
  onTabChange,
}: {
  tabs: { id: ChatInspectorTab; label: string }[];
  tab: ChatInspectorTab;
  onTabChange: (tab: ChatInspectorTab) => void;
}) {
  return (
    <nav className="mt-4 flex h-10 items-stretch gap-4 px-5" aria-label="Inspector tabs">
      {tabs.map((t) => {
        const active = t.id === tab;
        return (
          <button
            key={t.id}
            type="button"
            onClick={() => onTabChange(t.id)}
            className={cn(
              "relative flex items-center text-[13px] transition-colors",
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
  );
}

function AgentIdentity({
  account,
  deploymentId,
  name,
  deploymentName,
  avatarUrl,
  status,
  className,
}: {
  account: string;
  deploymentId: string;
  name: string;
  deploymentName: string;
  avatarUrl: string;
  status?: DeploymentStatus;
  className?: string;
}) {
  return (
    <div className={cn("flex min-w-0 items-start gap-2.5", className)}>
      <BlueprintIdentity
        account={account}
        name={deploymentName}
        size={40}
        url={avatarUrl}
        className="size-10 shrink-0 rounded-sm"
      />
      <div className="flex min-w-0 flex-1 flex-col pt-0.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 truncate text-heading-3 text-foreground">
            {name}
          </span>
          <ChatDeploymentSummary
            status={status}
            className="shrink-0"
          />
        </div>
        <div className="mt-1 flex min-w-0 items-center">
          <TooltipProvider delayDuration={200}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Link
                  to={deploymentPath(account, deploymentId)}
                  className="inline-flex min-w-0 items-center text-muted-foreground transition-colors hover:text-foreground hover:underline"
                >
                  <span className="min-w-0 truncate text-body-sm">
                    {account}/{deploymentName}
                  </span>
                </Link>
              </TooltipTrigger>
              <TooltipContent side="bottom" sideOffset={3}>
                View blueprint
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>
    </div>
  );
}

export function ChatDeploymentSummary({
  status,
  className,
}: {
  status?: DeploymentStatus;
  className?: string;
}) {
  const badge = status ? statusBadge(status) : null;
  if (!badge) return null;

  return (
    <span
      aria-label="Deployment status"
      className={cn("inline-flex min-w-0", className)}
    >
      <span className="shrink-0">
        <DeploymentStatusBadge
          status={badge.status}
          label={badge.label}
        />
      </span>
    </span>
  );
}

function OverviewTab({
  deploymentId,
  account,
}: {
  deploymentId: string;
  account: string;
}) {
  const [range, setRange] = useState<DayRange>(DEFAULT_USAGE_RANGE);
  const selectedRange =
    USAGE_RANGES.find((option) => option.key === range) ??
    USAGE_RANGES[USAGE_RANGES.length - 1];
  const { days } = selectedRange;
  const timeParams = useMemo(() => buildTimeParams(days), [days]);
  const { data: summary } = useObservabilitySummary(deploymentId, timeParams, {
    window: range,
  });
  const { data: summariesData } = useObservabilitySummaries(account);
  const summaryEntry = summariesData?.summaries?.[deploymentId];
  const requestValues =
    summaryEntry?.request_series && summaryEntry.request_series.length > 1
      ? lastN(summaryEntry.request_series, days)
      : undefined;
  const requestSeries = requestValues ?? ZERO_SERIES;
  const tokenSeries = summaryEntry?.token_series
    ? lastN(summaryEntry.token_series, days)
    : undefined;
  const spendSeries = lastN(summaryEntry?.cost_series, days);
  const spend =
    sumSeries(spendSeries) ??
    (range === DEFAULT_USAGE_RANGE ? summaryEntry?.cost_usd : undefined);
  const requestTotal = sumSeries(requestValues);
  const tokenTotal = sumSeries(tokenSeries);
  const traceParams = useMemo(() => ({ limit: "8" }), []);
  const { data: tracesData } = useObservabilityTraces(deploymentId, traceParams, {
    window: "chat-inspector-latest",
  });

  return (
    <div className="flex flex-col gap-8">
      <UsageSummary
        requestSeries={requestSeries}
        tokenSeries={tokenSeries}
        range={range}
        onRangeChange={setRange}
        spend={spend === undefined ? "—" : formatAggregateCost(spend)}
        requests={
          requestTotal !== undefined
            ? formatCompact(requestTotal)
            : summary
              ? formatCompact(summary.total_traces)
              : "—"
        }
        tokens={
          tokenTotal !== undefined
            ? formatCompact(tokenTotal)
            : summary
              ? formatCompact(summary.metrics.total_tokens)
              : "—"
        }
      />

      <Section
        label="Recent traces"
        icon={ScrollText}
        className="gap-2"
        action={
          <Button
            asChild
            variant="ghost"
            size="sm"
            className="-mr-2 h-7 px-2 text-body-sm font-medium text-foreground-accent hover:bg-foreground-accent/10 hover:text-foreground-accent active:bg-foreground-accent/15"
          >
            <Link
              to={deploymentTracesPath(account, deploymentId)}
              aria-label="View all traces"
            >
              View all
            </Link>
          </Button>
        }
      >
        <RecentTraces
          traces={tracesData?.traces ?? []}
          account={account}
          deploymentId={deploymentId}
        />
      </Section>
    </div>
  );
}

function UsageSummary({
  requestSeries,
  tokenSeries,
  range,
  onRangeChange,
  spend,
  requests,
  tokens,
}: {
  requestSeries: number[];
  tokenSeries?: number[];
  range: DayRange;
  onRangeChange: (range: DayRange) => void;
  spend: string;
  requests: string;
  tokens: string;
}) {
  return (
    <Section
      label="Usage"
      icon={Activity}
      action={
        <div className="shrink-0">
          <TimeRangeSelector
            value={range}
            ranges={USAGE_RANGES}
            onChange={(next) => onRangeChange(next as DayRange)}
            layoutId="chat-inspector-usage-range"
          />
        </div>
      }
      bodyClassName="flex flex-col gap-2.5"
    >
      <div className="min-w-0">
        <RequestSparkline series={requestSeries} tokenSeries={tokenSeries} />
      </div>
      <div className="grid grid-cols-3 gap-x-5">
        <UsageMetric label="Requests" value={requests} />
        <UsageMetric label="Tokens" value={tokens} />
        <UsageMetric label="Spend" value={spend} />
      </div>
    </Section>
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
  // is DB-backed and cluster-independent, so it won't 503 on a briefly
  // unreachable cluster — but on a genuine read error, pinning the tab on
  // "Checking…" forever would be worse than attempting the (hardened,
  // fail-fast) request as the old code always did.
  const resolved = !!status && (!!runtimeData || runtimeError);
  const ready = resolved && state === "ready";
  // Only hit the messaging proxy (agent/config) when the agent is actually
  // reachable — otherwise the proxy hangs and 5xxs on an unresponsive sidecar.
  const { data: config, isLoading, isError } = useDeploymentAgentConfig(
    deploymentId,
    ready,
  );
  const [expanded, setExpanded] = useState(false);
  const [expandedTools, setExpandedTools] = useState<Set<string>>(new Set());
  const promptRef = useRef<HTMLParagraphElement>(null);

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
    <div className="flex flex-col gap-5">
      <Section label="System prompt" icon={NotepadText} className="gap-1.5">
        <div className="max-w-full overflow-hidden rounded-md bg-muted/40 px-3.5 py-3 dark:bg-accent/50">
          {prompt ? (
            <>
              <p
                ref={promptRef}
                className={cn(
                  "max-w-full text-body-sm font-normal leading-relaxed whitespace-pre-wrap break-words text-foreground",
                  expanded && "max-h-[min(50dvh,24rem)] overflow-y-auto overscroll-contain pr-1",
                  !expanded && longPrompt && "line-clamp-5",
                )}
              >
                {prompt}
              </p>
              {longPrompt ? (
                <button
                  type="button"
                  aria-expanded={expanded}
                  onClick={() => {
                    if (expanded && promptRef.current) {
                      promptRef.current.scrollTop = 0;
                    }
                    setExpanded((isExpanded) => !isExpanded);
                  }}
                  className="mt-2 text-body-sm font-medium text-foreground-accent transition-colors hover:text-foreground-accent/80"
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

      <Section
        label="Tools"
        icon={Wrench}
        className="gap-1.5"
        labelSuffix={tools.length ? tools.length : undefined}
      >
        {tools.length === 0 ? (
          <p className="text-body-sm text-muted-foreground">No tools configured.</p>
        ) : (
          <div className="flex flex-col">
            {tools.map((tool, i) => {
              const toolKey = `${tool.name}-${i}`;
              const label = tool.title || tool.name;
              const toolExpanded = expandedTools.has(toolKey);
              const ToggleIcon = toolExpanded ? ChevronDown : ChevronRight;
              return (
                <div
                  key={toolKey}
                  className={cn(
                    "flex min-w-0 flex-col transition-colors",
                    i > 0 && "border-t border-border/60",
                    toolExpanded && "bg-muted/40 dark:bg-accent/50",
                  )}
                >
                  <button
                    type="button"
                    aria-expanded={toolExpanded}
                    onClick={() =>
                      setExpandedTools((current) => {
                        const next = new Set(current);
                        if (next.has(toolKey)) next.delete(toolKey);
                        else next.add(toolKey);
                        return next;
                      })
                    }
                    className={cn(
                      "flex min-w-0 items-center gap-3 px-3 py-2.5 text-left transition-colors hover:bg-muted/25",
                      toolExpanded && "hover:bg-transparent",
                      toolExpanded && tool.description && "pb-1.5",
                    )}
                  >
                    <ToggleIcon className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 truncate text-body-sm font-medium text-foreground">
                      {label}
                    </span>
                  </button>
                  {toolExpanded && tool.description ? (
                    <p className="select-text px-3 pb-2.5 pl-9 text-body-sm leading-relaxed text-muted-foreground">
                      {tool.description}
                    </p>
                  ) : null}
                </div>
              );
            })}
          </div>
        )}
      </Section>
    </div>
  );
}

const RECENT_TRACES_LIMIT = 8;

function RecentTraces({
  traces,
  account,
  deploymentId,
}: {
  traces: TraceEntry[];
  account: string;
  deploymentId: string;
}) {
  const recent = traces.slice(0, RECENT_TRACES_LIMIT);
  if (recent.length === 0) {
    return (
      <p className="text-body-sm text-muted-foreground">No recent traces.</p>
    );
  }
  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex flex-col">
        {recent.map((t, i) => {
          const status = normalizeStatus(t.status);
          const timestamp = formatTraceDateTime(t.timestamp);
          const statusIcon = traceStatusIcon(status);
          const StatusIcon = statusIcon.Icon;
          const statusLabel = STATUS_CONFIG[status].label;
          return (
            <Link
              key={t.trace_id}
              to={deploymentTracePath(account, deploymentId, t.trace_id)}
              className={cn(
                "group -mx-1.5 grid grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-x-2 rounded-sm px-1.5 py-1.5 transition-colors hover:bg-muted/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
                i > 0 && "border-t border-border/50",
              )}
              aria-label={`Open ${statusLabel.toLowerCase()} trace from ${timestamp}`}
            >
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    className="inline-flex size-4 shrink-0 items-center justify-center"
                    aria-label={statusLabel}
                  >
                    <StatusIcon
                      aria-hidden="true"
                      className={cn("size-3.5", statusIcon.className)}
                      strokeWidth={2.1}
                    />
                  </span>
                </TooltipTrigger>
                <TooltipContent side="top" sideOffset={6}>
                  {statusLabel}
                </TooltipContent>
              </Tooltip>
              <TraceUserDateIdentity
                userId={t.user_id}
                userDetails={t.user_details}
                account={account}
                timestamp={timestamp}
                timestampIso={t.timestamp}
                allowUserLink={false}
              />
              <div className="flex shrink-0 flex-col items-end gap-0.5 text-right text-body-sm leading-tight tabular-nums">
                <span className="whitespace-nowrap font-mono font-medium text-foreground">
                  {formatTraceCost(t.total_cost)}
                </span>
                <span className="whitespace-nowrap text-faint-foreground">
                  {formatTraceTokens(t.total_tokens)}
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </TooltipProvider>
  );
}

function TraceUserDateIdentity({
  userId,
  userDetails,
  account,
  timestamp,
  timestampIso,
  allowUserLink = true,
}: {
  userId?: string;
  userDetails?: UserDetails;
  account: string;
  timestamp: string;
  timestampIso: string;
  allowUserLink?: boolean;
}) {
  const identity = userId
    ? userDetails
      ? { user_id: userId, user_details: userDetails }
      : identityRefFromUserID(userId)
    : null;
  const shouldResolveMember = !!identity && identity.user_details.kind !== "slack";
  const { data, isLoading } = useAccountMembers(account, {
    enabled: shouldResolveMember && !!account,
  });

  if (!identity) {
    return (
      <TraceUserDateShell
        label="—"
        timestamp={timestamp}
        timestampIso={timestampIso}
      />
    );
  }

  if (identity.user_details.kind === "slack") {
    const display = slackIdentityDisplay(identity);
    const shell = (
      <TraceUserDateShell
        label={display.primary}
        timestamp={timestamp}
        timestampIso={timestampIso}
      />
    );

    return allowUserLink && display.deepLink ? (
      <a
        href={display.deepLink}
        rel="noreferrer"
        className="group block min-w-0 hover:[&_span[data-user-name]]:underline"
      >
        {shell}
      </a>
    ) : (
      <span className="group block min-w-0">{shell}</span>
    );
  }

  const member = userId
    ? data?.members.find((candidate) => candidate.user_id === userId)
    : undefined;
  const username = member?.username ?? identity.user_details.username;
  const displayName =
    member?.display_name ||
    member?.username ||
    identity.user_details.display_name ||
    identity.user_details.username;

  if (!username || !displayName) {
    return (
      <TraceUserDateShell
        label={isLoading ? "…" : "Unknown user"}
        muted
        timestamp={timestamp}
        timestampIso={timestampIso}
      />
    );
  }

  return (
    <TraceUserDateShell
      label={displayName}
      timestamp={timestamp}
      timestampIso={timestampIso}
    />
  );
}

function TraceUserDateShell({
  label,
  timestamp,
  timestampIso,
  muted = false,
}: {
  label: string;
  timestamp: string;
  timestampIso: string;
  muted?: boolean;
}) {
  return (
    <span className="flex min-w-0 flex-col gap-px">
      <time
        dateTime={timestampIso}
        className="block truncate text-body-sm font-medium leading-tight text-foreground"
      >
        {timestamp}
      </time>
      <span
        data-user-name
        className={cn(
          "truncate text-body-sm leading-tight text-faint-foreground",
          muted && "italic",
        )}
      >
        {label}
      </span>
    </span>
  );
}

function traceStatusIcon(status: TraceStatus): {
  Icon: LucideIcon;
  className: string;
} {
  switch (status) {
    case "error":
      return { Icon: AlertCircle, className: "text-error" };
    case "timeout":
      return { Icon: TimerReset, className: "text-warning" };
    default:
      return { Icon: CheckCircle2, className: "text-success" };
  }
}

function formatTraceTokens(tokens?: number): string {
  return tokens && tokens > 0 ? `${formatCompact(tokens)} tokens` : "— tokens";
}

function lastN<T>(items: T[] | undefined, count: number): T[] | undefined {
  if (!items || items.length === 0) return undefined;
  return items.slice(Math.max(items.length - count, 0));
}

function sumSeries(values: number[] | undefined): number | undefined {
  if (!values || values.length === 0) return undefined;
  return values.reduce((sum, value) => sum + value, 0);
}

function formatTraceDateTime(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function Section({
  label,
  labelSuffix,
  icon: Icon,
  action,
  className,
  labelClassName,
  bodyClassName,
  children,
}: {
  label: string;
  labelSuffix?: string | number;
  icon?: LucideIcon;
  action?: React.ReactNode;
  className?: string;
  labelClassName?: string;
  bodyClassName?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={cn("flex flex-col gap-3.5", className)}>
      <div className="flex min-w-0 items-center justify-between gap-3">
        <ChatPanelSectionHeader
          label={label}
          icon={Icon}
          count={labelSuffix}
          className={labelClassName}
        />
        {action}
      </div>
      <div className={cn("min-w-0", bodyClassName)}>
        {children}
      </div>
    </section>
  );
}

function UsageMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <MetricLabel>{label}</MetricLabel>
      <span className="truncate font-mono text-body font-semibold text-foreground tabular-nums">
        {value}
      </span>
    </div>
  );
}

function MetricLabel({ children }: { children: React.ReactNode }) {
  return (
    <span className="truncate text-body-sm text-faint-foreground">
      {children}
    </span>
  );
}

function statusBadge(status?: DeploymentStatus): {
  status: DeploymentStatusValue;
  label?: string;
} {
  if (!status) return { status: "inactive", label: "Unknown" };
  if (status.value === "error" || status.reason === "failed") {
    return { status: "error", label: "Error" };
  }
  if (
    status.reason === "paused" ||
    status.value === "inactive"
  ) {
    return { status: "inactive", label: "Paused" };
  }
  switch (status.value) {
    case "active":
      return { status: "active", label: "Active" };
    case "deploying":
    case "undeploying":
      return { status: "deploying", label: "Deploying" };
    default:
      return { status: "inactive", label: "Paused" };
  }
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
