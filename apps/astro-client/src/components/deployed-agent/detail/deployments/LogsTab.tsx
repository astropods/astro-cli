import { useEffect, useMemo, useReducer, useState } from "react";
import { ChevronDown, Plus, Terminal, X } from "lucide-react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { useDeploymentLogs } from "@/api/queries/deployments";
import { useLogStream } from "../LogStreamProvider";
import type { AgentDeployment, WorkloadDetail } from "@/lib/api";

interface LogTab {
  workloadName: string;
  containerName: string;
}

function tabKey(t: LogTab) {
  return `${t.workloadName}|${t.containerName}`;
}

/** "agent / app" for multi-container workloads, "agent" for single-container */
function tabLabel(wl: WorkloadDetail, containerName: string): string {
  const service = wl.component || wl.name;
  return (wl.containers ?? []).length <= 1 ? service : `${service} / ${containerName}`;
}

type TabState = { tabs: LogTab[]; activeKey: string | null };
type TabAction =
  | { type: "open"; tab: LogTab }
  | { type: "close"; key: string }
  | { type: "focus"; key: string }
  | { type: "reset"; preload?: LogTab[]; activeKey?: string | null };

function tabReducer(state: TabState, action: TabAction): TabState {
  switch (action.type) {
    case "open": {
      const key = tabKey(action.tab);
      if (state.tabs.some((t) => tabKey(t) === key)) return { ...state, activeKey: key };
      return { tabs: [...state.tabs, action.tab], activeKey: key };
    }
    case "close": {
      const idx = state.tabs.findIndex((t) => tabKey(t) === action.key);
      if (idx === -1) return state;
      const next = state.tabs.filter((_, i) => i !== idx);
      if (state.activeKey !== action.key) return { tabs: next, activeKey: state.activeKey };
      const activeKey = next.length === 0 ? null : tabKey(next[Math.min(idx, next.length - 1)]);
      return { tabs: next, activeKey };
    }
    case "focus":
      return { ...state, activeKey: action.key };
    case "reset":
      return { tabs: action.preload ?? [], activeKey: action.activeKey ?? null };
  }
}

interface LogsTabProps {
  deployment: AgentDeployment;
  isCompact: boolean;
  isVisible?: boolean;
}

export function LogsTab({ deployment, isCompact, isVisible = true }: LogsTabProps) {
  const workloads = deployment.workloads ?? [];

  const [{ tabs: openTabs, activeKey }, dispatch] = useReducer(tabReducer, { tabs: [], activeKey: null });
  const [timeRange, setTimeRange] = useState<LogTimeRange>("15m");
  const [isTailing, setIsTailing] = useState(false);

  // Pre-load the first container tab and make it active when the deployment changes.
  // Also stop any running stream so it doesn't carry over to a different deployment.
  useEffect(() => {
    const first = workloads
      .flatMap((wl) => (wl.containers ?? []).map((c) => ({ workloadName: wl.name, containerName: c.name })))
      .slice(0, 1);
    dispatch({ type: "reset", preload: first, activeKey: first[0] ? tabKey(first[0]) : null });
    setIsTailing(false);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deployment.id]);

  const activeTab = useMemo(
    () => openTabs.find((t) => tabKey(t) === activeKey) ?? null,
    [openTabs, activeKey],
  );

  // Auto-disconnect after 30 s when the user navigates away from the Logs tab while live.
  useEffect(() => {
    if (isVisible || !isTailing) return;
    const timer = setTimeout(() => {
      stopStream();
      setIsTailing(false);
    }, 30_000);
    return () => clearTimeout(timer);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isVisible, isTailing]);

  const tabEnabled = activeTab !== null;

  // Historical (non-live) query — one-shot fetch, no polling.
  const { data: logsRaw, isLoading: histLoading, isError } = useDeploymentLogs(
    deployment.id,
    activeTab?.workloadName ?? "",
    activeTab?.containerName ?? "",
    timeRange,
    {
      enabled: !isTailing && tabEnabled,
      refetchInterval: false,
    },
  );

  // Live streaming — managed by LogStreamProvider so the connection survives tab switches.
  const { lines: streamLines, status: streamStatus, error: streamError, startStream, stopStream } = useLogStream();
  const isReconnecting = streamStatus === "reconnecting";

  // Start/stop the stream when live mode or the active container changes.
  useEffect(() => {
    if (isTailing && tabEnabled && activeTab) {
      startStream(deployment.id, activeTab.workloadName, activeTab.containerName);
    } else if (!isTailing) {
      stopStream();
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isTailing, activeTab?.workloadName, activeTab?.containerName, deployment.id]);

  const logs = isTailing ? streamLines : (logsRaw ?? []);
  const isLoading = !isTailing && histLoading;
  let logError: string | undefined;
  if (isTailing) logError = streamError;
  else if (isError) logError = "Failed to load logs.";


  if (workloads.length === 0) {
    return (
      <div className="flex items-center justify-center h-full font-mono text-mono-sm text-faint-foreground">
        No services available.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">

      <div className="flex items-center bg-muted border-b border-border flex-shrink-0 px-[clamp(16px,4vw,108px)] py-0 gap-0">
        {openTabs.map((tab) => {
          const key = tabKey(tab);
          const wl = workloads.find((w) => w.name === tab.workloadName);
          const label = wl ? tabLabel(wl, tab.containerName) : tab.containerName;
          const isActive = key === activeKey;
          return (
            <div
              key={key}
              role="tab"
              tabIndex={0}
              onClick={() => { dispatch({ type: "focus", key }); setIsTailing(false); }}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { dispatch({ type: "focus", key }); setIsTailing(false); } }}
              className={cn(
                "group flex items-center gap-1.5 font-sans text-heading-4 py-[11px] px-2.5 border-b transition-colors duration-150 cursor-pointer whitespace-nowrap",
                isActive
                  ? "font-medium text-foreground border-b-[var(--color-teal-600)]"
                  : "font-normal text-faint-foreground border-b-transparent hover:text-foreground",
              )}
            >
              {label}
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Close ${label}`}
                onClick={(e) => { e.stopPropagation(); dispatch({ type: "close", key }); setIsTailing(false); }}
                className={cn(
                  "rounded-full transition-colors",
                  isActive
                    ? "text-muted-foreground"
                    : "text-transparent group-hover:text-muted-foreground",
                )}
              >
                <X size={9} />
              </Button>
            </div>
          );
        })}

        {/* + Add tab */}
        {workloads.reduce((sum, wl) => sum + (wl.containers ?? []).length, 0) > openTabs.length && <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="xs"
              className="my-[9px] px-2.5 text-faint-foreground font-sans text-heading-4 font-normal focus-visible:ring-0 focus-visible:border-transparent"
            >
              {(() => {
                if (!activeKey) return <><Plus size={11} />Containers<ChevronDown size={11} /></>;
                const remaining = workloads.reduce((sum, wl) => sum + (wl.containers ?? []).length, 0) - openTabs.length;
                return <><Plus size={11} />{remaining} more {remaining === 1 ? "container" : "containers"}<ChevronDown size={11} /></>;
              })()}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="bottom" sideOffset={6} className="min-w-[200px]">
            {workloads.map((wl) => (
              <DropdownMenuGroup key={wl.name}>
                <DropdownMenuLabel className="px-2 py-1 font-mono text-mono-sm text-faint-foreground">
                  {wl.component || wl.name}
                </DropdownMenuLabel>
                {(wl.containers ?? []).map((c) => {
                  const alreadyOpen = openTabs.some(
                    (t) => t.workloadName === wl.name && t.containerName === c.name,
                  );
                  return (
                    <DropdownMenuItem
                      key={c.name}
                      onClick={() => dispatch({ type: "open", tab: { workloadName: wl.name, containerName: c.name } })}
                      className="cursor-pointer gap-2"
                    >
                      <span className="flex size-3.5 items-center justify-center shrink-0">
                        {alreadyOpen && <CheckIcon className="size-3 text-primary" />}
                      </span>
                      <span className={cn("font-mono text-mono-sm", alreadyOpen && "text-faint-foreground")}>
                        {c.name}
                      </span>
                    </DropdownMenuItem>
                  );
                })}
              </DropdownMenuGroup>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>}
      </div>

      <div className="flex-1 min-h-0 overflow-hidden py-5 px-[clamp(16px,4vw,108px)]">
        {activeKey === null ? (
          <div className="flex flex-col items-center justify-center h-full gap-3 bg-surface rounded-lg border border-border">
            <div className="flex items-center justify-center size-10 rounded border border-border text-faint-foreground">
              <Terminal size={18} />
            </div>
            <div className="flex flex-col items-center gap-1">
              <p className="text-heading-4 font-medium">Select a container to view logs</p>
              <p className="text-body-sm text-faint-foreground">Choose a container above to load its log output.</p>
            </div>
          </div>
        ) : (
          <LogViewer
            logs={logs}
            isLoading={isLoading}
            isCompact={isCompact}
            timeRange={timeRange}
            onTimeRangeChange={setTimeRange}
            error={logError}
            isTailing={isTailing}
            isReconnecting={isReconnecting}
            onTailToggle={() => setIsTailing((v) => !v)}
          />
        )}
      </div>
    </div>
  );
}
