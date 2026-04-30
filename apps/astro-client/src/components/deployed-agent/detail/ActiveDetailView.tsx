import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { deploymentKeys } from "@/api/queries/keys";
import { ArrowLeft, Loader2 } from "lucide-react";
import { ChatBubbleLeftRightIcon, Cog6ToothIcon, PauseCircleIcon, PlayCircleIcon } from "@heroicons/react/24/outline";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { isDeployingState, isPausedState, isLiveState, mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";
import { formatDateLong } from "@/lib/format-utils";
import { dashboardPath } from "@/lib/routes";
import type { AgentDeployment } from "@/lib/api";
import { useRestartDeployment, useStopDeployment, useWakeUpDeployment } from "@/api/queries/deployments";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { SidePanel } from "./SidePanel";
import { DeploymentStatusBadge } from "@/components/deployed-agent/DeploymentStatusBadge";
import { KebabMenu } from "./shared/KebabMenu";
import { MonitorTab } from "./monitor/MonitorTab";
import type { TraceRow } from "./monitor/MonitorTab";
import { TraceDetailPanel } from "./monitor/TraceDetailPanel";
import { DeploymentsTab } from "./deployments/DeploymentsTab";
import { LogsTab } from "./deployments/LogsTab";
import { LogStreamProvider } from "./LogStreamProvider";
import { ConfigurePanel } from "./configure/ConfigurePanel";
import { ActionPanel } from "@/components/ui/status-panel";
import { useLogTimezone } from "@/lib/timezone";
import { cn } from "@/lib/utils";

// ─── main component ───────────────────────────────────────────────────────────
interface ActiveDetailViewProps {
  deployment: AgentDeployment;
  account: string;
  isPersonal: boolean;
  monitorLocked?: boolean;
  monitorLockReason?: string;
  backPathOverride?: string;
  onRedeploy?: () => void;
  /**
   * When true, opens the configure panel in "new build" mode automatically
   * once a newer build is detected. Passed via router state from the
   * dashboard's "Update available" affordance so a single click upgrades.
   */
  autoConfigureNewBuild?: boolean;
}

export function ActiveDetailView({
  deployment,
  account,
  monitorLocked = false,
  monitorLockReason = "Available once deployment is live.",
  backPathOverride,
  onRedeploy,
  autoConfigureNewBuild = false,
}: ActiveDetailViewProps) {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const rawTab = searchParams.get("tab")
  const tab: "monitor" | "deployments" | "logs" =
    rawTab === "monitor" && !monitorLocked ? "monitor" :
    rawTab === "logs" ? "logs" :
    "deployments"
  const [configOpen, setConfigOpen] = useState(false)
  const [configRevision, setConfigRevision] = useState<number | null>(null)
  const [configIsNewBuild, setConfigIsNewBuild] = useState(false)
  const [configRollbackBuildId, setConfigRollbackBuildId] = useState<string | null>(null)
  const deploymentAvatarBust = useDeploymentAvatarBust(deployment.id);
  const deploymentAvatarUrl = deploymentAvatarBust ?? getDeploymentAvatarUrl(deployment.id);
  const messagingUrl = deployment.external_urls?.find(u => u.type === 'messaging')?.url;

  const [panelWidth, setPanelWidth] = useState(420);
  const [selectedTrace, setSelectedTrace] = useState<TraceRow | null>(null)
  const [navTraces, setNavTraces] = useState<TraceRow[]>([])
  const selectedIndex = navTraces.findIndex((t) => t.id === selectedTrace?.id)
  const canGoPrev = selectedIndex > 0
  const canGoNext = selectedIndex < navTraces.length - 1
  const handleNavigate = (dir: "prev" | "next") => {
    const next = dir === "prev" ? navTraces[selectedIndex - 1] : navTraces[selectedIndex + 1]
    if (next) setSelectedTrace(next)
  }
  const [isCompact, setIsCompact] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return window.innerWidth < 1180;
  })
  const [optimisticDeploying, setOptimisticDeploying] = useState(false)
  const [pausing, setPausing] = useState(false)
  const pausePollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const queryClient = useQueryClient()
  const [isGloballyRestarting, setIsGloballyRestarting] = useState(false)
  const [isPodLevelRestarting, setIsPodLevelRestarting] = useState(false)
  const isRestarting = isGloballyRestarting || isPodLevelRestarting
  // For cross-account deploys (e.g. you deployed an org's blueprint into
  // your personal account) the upgrade signal must come from the source
  // account's blueprint, not the URL/owning account. The owning account
  // may have a same-named but lineage-unrelated blueprint that would
  // otherwise produce a false "Update available" pointing at a build the
  // deployed pod was never built from.
  const sourceAccount = deployment.source_account || account;
  const { data: accountAgents } = useAccountBlueprints(sourceAccount);
  const { timezone } = useLogTimezone();
  const pauseMutation = useStopDeployment(account);
  const wakeupMutation = useWakeUpDeployment(account);
  const restartAllMutation = useRestartDeployment(account);
  const renderedDeployment = optimisticDeploying
    ? { ...deployment, status: "pending", ready: 0 }
    : deployment;
  const displayName = renderedDeployment.display_name || renderedDeployment.name
  const backPath = backPathOverride ?? `${dashboardPath}?account=${encodeURIComponent(account)}`
  const isDeploying = isDeployingState(renderedDeployment);
  const isPaused = isPausedState(renderedDeployment);
  const showConfigureAsPage = isCompact && configOpen;
  const showTraceAsPage = isCompact && selectedTrace !== null;
  const panelOpen = configOpen || selectedTrace !== null;
  const controlsBusy = pauseMutation.isPending || wakeupMutation.isPending;
  // Use replicas===0 as a fallback for "fully paused" since some clusters don't set scaled_down/stopped status.
  const isActuallyPaused = isPaused || mapDeploymentStatus(renderedDeployment) === "inactive";
  const isPausing = (pausing || pauseMutation.isPending) && !isActuallyPaused;
  // isResuming persists until live, so the badge doesn't flash then hand off to "Deploying".
  const isResuming = (wakeupMutation.isPending || wakeupMutation.isSuccess) && !isLiveState(renderedDeployment) && !isActuallyPaused;
  const blueprintAgent = accountAgents?.agents?.find((a) => a.name === renderedDeployment.name);
  const latestVersion = blueprintAgent?.versions?.reduce((latest, current) =>
    new Date(current.published_at).getTime() > new Date(latest.published_at).getTime()
      ? current
      : latest,
  );
  const latestBuildId = latestVersion?.build_id;
  const canShowUpgradeSignal = sourceAccount === account || blueprintAgent?.visibility !== "private";
  const hasNewBuildAvailable = canShowUpgradeSignal && !!latestBuildId && latestBuildId !== renderedDeployment.build_id;

  // Honor the dashboard's one-click upgrade affordance: when a newer build is
  // available and the caller asked us to, auto-open the configure panel in
  // new-build mode. Single-shot via ref so re-renders or state changes don't
  // re-open the panel after the user closes it.
  const autoConfigureFiredRef = useRef(false);
  useEffect(() => {
    if (autoConfigureFiredRef.current) return;
    if (!autoConfigureNewBuild) return;
    if (!hasNewBuildAvailable) return;
    autoConfigureFiredRef.current = true;
    setConfigOpen(true);
    setConfigIsNewBuild(true);
    setConfigRevision(null);
  }, [autoConfigureNewBuild, hasNewBuildAvailable]);

  useEffect(() => {
    if (!pausing) return;
    if (isActuallyPaused) {
      setPausing(false);
      if (pausePollRef.current) { clearInterval(pausePollRef.current); pausePollRef.current = null; }
      return;
    }
    pausePollRef.current = setInterval(() => {
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(renderedDeployment.id) });
    }, 3000);
    return () => {
      if (pausePollRef.current) { clearInterval(pausePollRef.current); pausePollRef.current = null; }
    };
  }, [pausing, isActuallyPaused, queryClient, renderedDeployment.id]);

  useEffect(() => {
    if (!isGloballyRestarting) return;
    const timer = setTimeout(() => setIsGloballyRestarting(false), 15000);
    return () => clearTimeout(timer);
  }, [isGloballyRestarting]);

  useEffect(() => {
    if (!optimisticDeploying) return;
    // Stop forcing UI once the live query reflects a deploying status.
    if (isDeployingState(deployment)) {
      setOptimisticDeploying(false);
      return;
    }
    // Safety fallback to avoid sticky optimistic state.
    const timer = setTimeout(() => setOptimisticDeploying(false), 10000);
    return () => clearTimeout(timer);
  }, [optimisticDeploying, deployment]);

  useEffect(() => {
    const onResize = () => setIsCompact(window.innerWidth < 1180);
    onResize();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  return (
    <LogStreamProvider>
    <div className="flex flex-1 min-h-0 overflow-hidden relative bg-surface">
      <div className="flex flex-1 flex-col min-w-0 min-h-0">

      {/* ── TOP BAR ── */}
      <header
        className={cn(
          "bg-surface border-b border-border sticky top-0 z-40 flex items-center shrink-0 px-[clamp(12px,3vw,40px)]",
          isCompact ? "flex-wrap justify-start py-[10px] h-auto gap-y-2" : "flex-nowrap justify-between py-0 h-[63px] gap-y-0",
        )}
      >
        <div className={cn("flex items-center gap-2.5 min-w-0", isCompact ? "flex-wrap" : "flex-nowrap")}>
          <button
            onClick={() => navigate(backPath)}
            className="bg-transparent border-0 cursor-pointer text-faint-foreground flex p-1"
          >
            <ArrowLeft size={14} />
          </button>
          <div className="rounded overflow-hidden shrink-0 leading-none">
            <BlueprintIdentity account={account} name={deployment.name} size={26} url={deploymentAvatarUrl} className="rounded-sm" />
          </div>
          <div className="flex items-center gap-1.5">
            <h1 className="font-sans text-heading-4 font-semibold text-foreground m-0 leading-tight">
              {displayName}
            </h1>
            <KebabMenu
              deploymentId={deployment.id}
              deploymentName={deployment.name}
              displayName={deployment.display_name}
              account={account}
              installedAt={formatDate(deployment.created_at)}
              avatarColors={deployment.avatar_colors}
              onRestart={!isPaused && !isDeploying ? () => { setIsGloballyRestarting(true); restartAllMutation.mutate({ deploymentId: renderedDeployment.id }); } : undefined}
            />
          </div>
          <DeploymentStatusBadge
            status={isRestarting ? 'restarting' : isPausing ? 'pausing' : isResuming ? 'resuming' : mapDeploymentStatus(renderedDeployment)}
            errorMessage={renderedDeployment.error_message}
          />
        </div>
        <div
          className={cn(
            "flex items-center gap-2",
            isCompact ? "justify-start flex-wrap w-full mt-0.5" : "justify-end flex-nowrap w-auto",
          )}
        >
          <Button
            variant="outline"
            size="default"
            onClick={isPaused || isResuming
              ? () => { setPausing(false); wakeupMutation.mutate({ deploymentId: renderedDeployment.id }); }
              : () => { setPausing(true); pauseMutation.mutate({ deploymentId: renderedDeployment.id }); }}
            disabled={isPaused || isResuming ? (controlsBusy || isResuming) : (isDeploying || isGloballyRestarting || controlsBusy)}
            title={isPaused || isResuming ? "Resume deployment" : "Pause deployment (scale instances to zero)"}
            className={isPaused || isResuming ? undefined : "text-[var(--color-coral-600)] border-[var(--color-coral-300)] hover:text-[var(--color-coral-600)] dark:border-[var(--color-coral-800)]"}
          >
            {isPaused || isResuming
              ? (isResuming ? <Loader2 className="animate-spin" /> : <PlayCircleIcon className="size-4" />)
              : (pausing || pauseMutation.isPending ? <Loader2 className="animate-spin" /> : <PauseCircleIcon className="size-4" />)}
            {isPaused || isResuming ? "Resume" : "Pause"}
          </Button>
          {messagingUrl && (
            <Button
              variant="outline"
              size="default"
              onClick={() => window.open(messagingUrl, '_blank', 'noopener,noreferrer')}
            >
              <ChatBubbleLeftRightIcon className="size-4" /> Chat
            </Button>
          )}
          <Button
            variant="outline"
            size="default"
            onClick={() => { setConfigOpen(o => !o); setConfigRevision(null); setConfigIsNewBuild(false); setSelectedTrace(null); }}
            disabled={isRestarting}
            data-active={configOpen || undefined}
          >
            <Cog6ToothIcon className="size-4" /> Configure
          </Button>
        </div>
      </header>

      {/* ── MAIN AREA (tab bar + content) ── */}
      <div className="flex flex-1 min-h-0">

        {/* left: tabs + content */}
        <div className="flex flex-col flex-1 min-w-0 min-h-0">

          {/* tab bar */}
          <div
            className="flex bg-surface border-b border-border shrink-0 px-[clamp(16px,4vw,108px)] py-0"
          >
            {([
              { id: 'monitor' as const, label: 'Monitor', icon: (
                <svg className="size-3.5 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5M9 11.25v1.5M12 9v3.75m3-6v6" />
                </svg>
              )},
              { id: 'deployments' as const, label: 'Deployments', icon: (
                <svg className="size-3.5 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699 2.7c-.103.021-.207.041-.311.06a15.09 15.09 0 01-2.448-2.448 14.9 14.9 0 01.06-.312m-2.24 2.39a4.493 4.493 0 00-1.757 4.306 4.493 4.493 0 004.306-1.758M16.5 9a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0z" />
                </svg>
              )},
              { id: 'logs' as const, label: 'Logs', icon: (
                <svg className="size-3.5 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 9.75h16.5m-16.5 4.5h16.5m-16.5 4.5h8.25M3 5.25h18" />
                </svg>
              )},
            ]).map(({ id, label, icon }) => {
              const isLockedMonitor = id === "monitor" && monitorLocked;
              const tabButton = (
                <button
                  key={id}
                  onClick={() => {
                    if (isLockedMonitor) return;
                    setSearchParams((prev) => {
                      const next = new URLSearchParams(prev);
                      next.set("tab", id);
                      return next;
                    }, { replace: true });
                  }}
                  className={cn(
                    "flex items-center gap-1.5 bg-transparent border-0 font-sans text-heading-4 py-[11px] px-4 border-b-2 transition-colors duration-150",
                    id === 'monitor' && "pl-0",
                    isLockedMonitor
                      ? "cursor-not-allowed opacity-65 text-faint-foreground border-b-transparent"
                      : tab === id
                        ? "cursor-pointer font-medium text-foreground border-b-[var(--color-teal-600)]"
                        : "cursor-pointer font-normal text-faint-foreground border-b-transparent",
                  )}
                >
                  {icon}
                  {label}
                </button>
              );

              if (!isLockedMonitor) return tabButton;

              return (
                <TooltipProvider key={id} delayDuration={100}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      {tabButton}
                    </TooltipTrigger>
                    <TooltipContent side="bottom" sideOffset={6}>
                      {monitorLockReason}
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              );
            })}
          </div>

          {/* tab content — LogsTab stays mounted to preserve open tabs across tab switches */}
          <div className={cn("flex flex-col flex-1 min-h-0 overflow-hidden", tab !== 'logs' && "hidden")}>
            <LogsTab
              deployment={renderedDeployment}
              isCompact={isCompact}
              isVisible={tab === 'logs'}
            />
          </div>
          <div
            className={cn("dp-scroll flex-1 min-h-0 overflow-y-auto overflow-x-auto py-6 px-[calc(clamp(16px,4vw,108px)+4px)]", tab === 'logs' && "hidden")}
          >
              <div>
                {hasNewBuildAvailable && !showConfigureAsPage && !showTraceAsPage && (
                  <div className="mb-4">
                    <ActionPanel
                      tone="warning"
                      title={<>A new build (<span className="font-mono">{latestBuildId?.slice(0, 8)}</span>) for this agent was released on {formatDateLong(latestVersion?.published_at ?? "", timezone)}.</>}
                      primaryLabel="Redeploy →"
                      onPrimary={() => { setConfigOpen(true); setConfigIsNewBuild(true); setConfigRevision(null); setSelectedTrace(null); }}
                      confirmTitle="Are you sure?"
                      confirmBody="This upstream build may contain breaking changes. Upgrading could affect your agent's behavior or state."
                      confirmLabel="Redeploy"
                      dismissible
                    />
                  </div>
                )}
                {showConfigureAsPage ? (
                  <ConfigurePanel
                    deployment={renderedDeployment}
                    account={account}
                    fullPage
                    onClose={() => { setConfigOpen(false); setConfigRevision(null); setConfigIsNewBuild(false); setConfigRollbackBuildId(null); }}
                    onRedeployStart={() => { setOptimisticDeploying(true); }}
                    onRedeploy={() => { setOptimisticDeploying(true); void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(renderedDeployment.id) }); onRedeploy?.(); }}
                    revisionOverride={configRevision ?? undefined}
                    readOnly={configRevision !== null && configRollbackBuildId === null}
                    isNewBuild={configIsNewBuild}
                    newBuildId={configIsNewBuild ? latestBuildId : undefined}
                    rollbackContext={configRevision !== null && configRollbackBuildId !== null ? { revision: configRevision, buildId: configRollbackBuildId } : undefined}
                  />
                ) : showTraceAsPage && selectedTrace ? (
                  <TraceDetailPanel
                    trace={selectedTrace}
                    fullPage
                    canGoPrev={canGoPrev}
                    canGoNext={canGoNext}
                    onNavigate={handleNavigate}
                    onClose={() => setSelectedTrace(null)}
                  />
                ) : tab === 'monitor' ? (
                  <MonitorTab
                    deployment={renderedDeployment}
                    selectedTraceId={selectedTrace?.id ?? null}
                    onSelectTrace={(trace) => { setSelectedTrace((prev) => prev?.id === trace.id ? null : trace); setConfigOpen(false); }}
                    onVisibleTracesChange={setNavTraces}
                    compactCharts={panelOpen && panelWidth > 420}
                  />
                ) : (
                  <DeploymentsTab
                    deployment={renderedDeployment}
                    account={account}
                    isPausing={isPausing}
                    isResuming={isResuming}
                    isRestarting={isRestarting}
                    isGloballyRestarting={isGloballyRestarting}
                    onRollback={(revision, buildId) => { setConfigRevision(revision); setConfigRollbackBuildId(buildId); setConfigIsNewBuild(false); setConfigOpen(true); setSelectedTrace(null); }}
                    onPodRestartStateChange={setIsPodLevelRestarting}
                  />
                )}
              </div>
            </div>
        </div>

      </div>
      </div>

      {/* right panel */}
      {!isCompact && (
        <SidePanel open={panelOpen} onWidthChange={setPanelWidth}>
          {configOpen && (
            <ConfigurePanel
              deployment={renderedDeployment}
              account={account}
              onClose={() => { setConfigOpen(false); setConfigRevision(null); setConfigIsNewBuild(false); setConfigRollbackBuildId(null); }}
              onRedeployStart={() => { setOptimisticDeploying(true); }}
              onRedeploy={() => { setOptimisticDeploying(true); void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(renderedDeployment.id) }); onRedeploy?.(); }}
              revisionOverride={configRevision ?? undefined}
              readOnly={configRevision !== null && configRollbackBuildId === null}
              isNewBuild={configIsNewBuild}
              newBuildId={configIsNewBuild ? latestBuildId : undefined}
              rollbackContext={configRevision !== null && configRollbackBuildId !== null ? { revision: configRevision, buildId: configRollbackBuildId } : undefined}
            />
          )}
          {selectedTrace && !configOpen && (
            <TraceDetailPanel
              trace={selectedTrace}
              canGoPrev={canGoPrev}
              canGoNext={canGoNext}
              onNavigate={handleNavigate}
              onClose={() => setSelectedTrace(null)}
            />
          )}
        </SidePanel>
      )}
    </div>
    </LogStreamProvider>
  )
}
