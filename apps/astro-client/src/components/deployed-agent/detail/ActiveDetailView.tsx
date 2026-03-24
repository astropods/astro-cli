import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { deploymentKeys } from "@/api/queries/keys";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Cog6ToothIcon, PauseCircleIcon, PlayCircleIcon } from "@heroicons/react/24/outline";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { isDeployingState, isPausedState, mapDeploymentStatus, formatDate } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";
import { useStopDeployment, useWakeUpDeployment } from "@/api/queries/deployments";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { BuildUpdateBadge } from "@/components/BuildUpdateBadge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { KebabMenu } from "./shared/KebabMenu";
import { MonitorTab } from "./monitor/MonitorTab";
import { DeploymentsTab } from "./deployments/DeploymentsTab";
import { ConfigurePanel } from "./configure/ConfigurePanel";

const C = {
  bg: "var(--muted)",
  bgDeep: "var(--muted)",
  panel: "var(--surface)",
  border: "var(--border)",
  teal: "var(--primary)",
  tealMid: "var(--color-teal-600)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  faint: "var(--faint-foreground)",
  amber: "var(--color-amber-700)",
  amberBg: "color-mix(in oklch, var(--color-amber-700) 12%, transparent)",
  amberBdr: "color-mix(in oklch, var(--color-amber-700) 28%, transparent)",
  warning: "var(--color-yellow-500)",
  warningBg: "color-mix(in oklch, var(--color-yellow-500) 12%, transparent)",
  warningBdr: "color-mix(in oklch, var(--color-yellow-500) 28%, transparent)",
  coral: "var(--color-coral-600)",
  coralBg: "color-mix(in oklch, var(--color-coral-600) 12%, transparent)",
  coralBdr: "color-mix(in oklch, var(--color-coral-600) 28%, transparent)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  heading2: "var(--text-heading-2)",
  heading4: "var(--text-heading-4)",
  body: "var(--text-body)",
  bodySm: "var(--text-body-sm)",
  label: "var(--text-label)",
  monoSm: "var(--text-mono-sm)",
} as const;

const I = {
  sm: 12,
  md: 14,
  lg: 16,
} as const;

const DETAIL_LEFT_ALIGN_PX = 108;
const TOP_BAR_HEIGHT_PX = 63;
const CONFIG_PANEL_WIDTH_PX = 420;
const DETAIL_HORIZONTAL_PAD = `clamp(16px, 4vw, ${DETAIL_LEFT_ALIGN_PX}px)`;

// ─── main component ───────────────────────────────────────────────────────────
interface ActiveDetailViewProps {
  deployment: AgentDeployment;
  account: string;
  isPersonal: boolean;
  monitorLocked?: boolean;
  monitorLockReason?: string;
  backPathOverride?: string;
  onRedeploy?: () => void;
}

export function ActiveDetailView({
  deployment,
  account,
  isPersonal,
  monitorLocked = false,
  monitorLockReason = "Available once deployment is live.",
  backPathOverride,
  onRedeploy,
}: ActiveDetailViewProps) {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const rawTab = searchParams.get("tab")
  const tab: "monitor" | "deployments" =
    monitorLocked ? "deployments" :
    rawTab === "monitor" ? "monitor" :
    "deployments"
  const [configOpen, setConfigOpen] = useState(false)
  const [isCompact, setIsCompact] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return window.innerWidth < 1180;
  })
  const [contentScale, setContentScale] = useState<number>(1)
  const contentViewportRef = useRef<HTMLDivElement | null>(null)
  const contentInnerRef = useRef<HTMLDivElement | null>(null)
  const [optimisticDeploying, setOptimisticDeploying] = useState(false)
  const [pausing, setPausing] = useState(false)
  const pausePollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const { data: accountAgents } = useAccountBlueprints(account);
  const pauseMutation = useStopDeployment(account);
  const wakeupMutation = useWakeUpDeployment(account);
  const renderedDeployment = optimisticDeploying
    ? { ...deployment, status: "pending", ready: 0 }
    : deployment;
  const displayName = renderedDeployment.display_name || renderedDeployment.name
  const backPath = backPathOverride ?? (isPersonal ? '/agents' : `/${account}`)
  const isDeploying = isDeployingState(renderedDeployment);
  const isPaused = isPausedState(renderedDeployment);
  const showConfigureAsPage = isCompact && configOpen;
  const controlsBusy = pauseMutation.isPending || wakeupMutation.isPending;
  const latestBuildId = accountAgents?.agents
    ?.find((a) => a.name === renderedDeployment.name)
    ?.versions?.reduce((latest, current) =>
      new Date(current.published_at).getTime() > new Date(latest.published_at).getTime()
        ? current
        : latest,
    )?.build_id;
  const hasNewBuildAvailable = !!latestBuildId && latestBuildId !== renderedDeployment.build_id;

  useEffect(() => {
    if (!pausing) return;
    if (isPaused || !isDeploying) {
      setPausing(false);
      if (pausePollRef.current) { clearInterval(pausePollRef.current); pausePollRef.current = null; }
      return;
    }
    pausePollRef.current = setInterval(() => {
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    }, 3000);
    return () => {
      if (pausePollRef.current) { clearInterval(pausePollRef.current); pausePollRef.current = null; }
    };
  }, [pausing, isPaused, isDeploying, account, queryClient]);

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


  useLayoutEffect(() => {
    const fitContentToViewport = () => {
      const viewport = contentViewportRef.current;
      const inner = contentInnerRef.current;
      if (!viewport || !inner) return;

      // Skip autoscaling for narrow layouts to preserve readability.
      if (window.innerWidth < 1180) {
        setContentScale(1);
        return;
      }

      const availableHeight = viewport.clientHeight;
      const naturalHeight = inner.scrollHeight;
      if (availableHeight <= 0 || naturalHeight <= 0) {
        setContentScale(1);
        return;
      }

      const nextScale = Math.min(1, Math.max(0.78, availableHeight / naturalHeight));
      setContentScale((prev) => (Math.abs(prev - nextScale) < 0.005 ? prev : nextScale));
    };

    const raf = requestAnimationFrame(fitContentToViewport);
    window.addEventListener("resize", fitContentToViewport);
    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", fitContentToViewport);
    };
  }, [tab, configOpen, renderedDeployment.id, isCompact]);

  return (
    <div style={{ display: 'flex', flex: 1, minHeight: 0, background: C.bg }}>
      <div style={{ display: 'flex', flex: 1, flexDirection: 'column', minWidth: 0 }}>
      {/* ── TOP BAR ── */}
      <header style={{
        background: C.panel,
        borderBottom: `1px solid ${C.border}`,
        position: 'sticky', top: 0, zIndex: 40,
        display: 'flex', alignItems: 'center', justifyContent: isCompact ? 'flex-start' : 'space-between', flexWrap: isCompact ? 'wrap' : 'nowrap',
        padding: isCompact ? '10px clamp(12px, 3vw, 40px)' : '0 clamp(12px, 3vw, 40px)',
        height: isCompact ? 'auto' : TOP_BAR_HEIGHT_PX,
        rowGap: isCompact ? 8 : 0,
        flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0, flexWrap: isCompact ? 'wrap' : 'nowrap' }}>
          <button
            onClick={() => navigate(backPath)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: C.faint, display: 'flex', padding: 4 }}
          >
            <ArrowLeft size={I.md} />
          </button>
          <div style={{ borderRadius: 4, overflow: 'hidden', flexShrink: 0, lineHeight: 0 }}>
            <BlueprintIdentity account={account} name={deployment.name} size={26} avatarUrl={deployment.avatar_url} className="rounded-sm" />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <h1
              style={{
                fontFamily: S.body,
                fontSize: T.heading4,
                fontWeight: 600,
                color: C.text,
                margin: 0,
                lineHeight: 1.2,
              }}
            >
              {displayName}
            </h1>
            <KebabMenu
            deploymentId={deployment.id}
            deploymentName={deployment.name}
            displayName={deployment.display_name}
            account={account}
            installedAt={formatDate(deployment.created_at)}
            />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <h1
              style={{
                fontFamily: S.body,
                fontSize: T.heading4,
                fontWeight: 600,
                color: C.text,
                margin: 0,
                lineHeight: 1.2,
              }}
            >
              {displayName}
            </h1>
            <KebabMenu
            deploymentId={deployment.id}
            deploymentName={deployment.name}
            displayName={deployment.display_name}
            account={account}
            installedAt={formatDate(deployment.created_at)}
            />
          </div>
          {hasNewBuildAvailable ? (
            <BuildUpdateBadge
              currentBuildId={renderedDeployment.build_id}
              latestBuildId={latestBuildId}
            />
          ) : null}
          {(() => {
            const ds = mapDeploymentStatus(renderedDeployment)
            const badge =
              ds === 'error'
                ? { bg: C.coralBg, bdr: C.coralBdr, dot: C.coral, label: 'Error', spinning: false }
                : ds === 'undeploying'
                  ? { bg: C.bgDeep, bdr: C.border, dot: C.faint, label: 'Undeploying', spinning: true }
                : ds === 'pending'
                  ? { bg: C.warningBg, bdr: C.warningBdr, dot: C.warning, label: 'Deploying', spinning: true }
                  : ds === 'inactive'
                  ? { bg: C.bgDeep, bdr: C.border, dot: C.faint, label: 'Inactive', spinning: false }
                    : { bg: 'rgba(21,130,125,0.08)', bdr: 'rgba(21,130,125,0.22)', dot: C.tealMid, label: 'LIVE', spinning: false }
            return (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 5,
                padding: '2px 10px', borderRadius: 99,
                background: badge.bg, border: `1px solid ${badge.bdr}`,
                fontFamily: S.mono, fontSize: T.label, letterSpacing: '0.06em', color: badge.dot,
              }}>
                {badge.spinning ? (
                  <Loader2 size={I.sm} style={{ color: badge.dot, animation: "dp-spin 1.2s linear infinite" }} />
                ) : (
                  <span style={{ width: 5, height: 5, borderRadius: '50%', background: badge.dot, display: 'inline-block' }} />
                )}
                {badge.label}
              </span>
            )
          })()}
        </div>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: isCompact ? 'flex-start' : 'flex-end',
            flexWrap: isCompact ? 'wrap' : 'nowrap',
            gap: 8,
            width: isCompact ? '100%' : 'auto',
            marginTop: isCompact ? 2 : 0,
          }}
        >
          {!isPaused && !isDeploying && (
            <Button
              variant="outline"
              size="default"
              onClick={() => { setPausing(true); pauseMutation.mutate({ deploymentId: renderedDeployment.id }); }}
              disabled={pausing || controlsBusy}
              title="Pause deployment (scale instances to zero)"
              className="text-[var(--color-coral-600)] border-[var(--color-coral-300)] hover:text-[var(--color-coral-600)] dark:border-[var(--color-coral-800)]"
            >
              {pausing || pauseMutation.isPending ? <Loader2 className="animate-spin" /> : <PauseCircleIcon className="size-4" />}
              Pause
            </Button>
          )}
          {isPaused && (
            <Button
              variant="outline"
              size="default"
              onClick={() => wakeupMutation.mutate({ deploymentId: renderedDeployment.id })}
              disabled={controlsBusy}
              title="Resume deployment"
            >
              {wakeupMutation.isPending ? <Loader2 className="animate-spin" /> : <PlayCircleIcon className="size-4" />}
              Resume
            </Button>
          )}
          <Button
            variant="outline"
            size="default"
            onClick={() => setConfigOpen(o => !o)}
            data-active={configOpen || undefined}
          >
            <Cog6ToothIcon className="size-4" /> Configure
          </Button>
        </div>
      </header>

      {/* ── MAIN AREA (tab bar + content) ── */}
      <div
        style={{
          display: 'flex',
          flex: 1,
          minHeight: 0,
        }}
      >

        {/* left: tabs + content */}
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0, minHeight: 0 }}>

          {/* tab bar */}
          <div
            style={{
              display: 'flex',
              padding: `0 ${DETAIL_HORIZONTAL_PAD}`,
              background: C.bg,
              borderBottom: `1px solid ${C.border}`,
              flexShrink: 0,
            }}
          >
            {([
              { id: 'monitor' as const, label: 'Monitor', icon: (
                <svg style={{ width: I.md, height: I.md, flexShrink: 0 }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5M9 11.25v1.5M12 9v3.75m3-6v6" />
                </svg>
              )},
{ id: 'deployments' as const, label: 'Deployments', icon: (
                <svg style={{ width: I.md, height: I.md, flexShrink: 0 }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699 2.7c-.103.021-.207.041-.311.06a15.09 15.09 0 01-2.448-2.448 14.9 14.9 0 01.06-.312m-2.24 2.39a4.493 4.493 0 00-1.757 4.306 4.493 4.493 0 004.306-1.758M16.5 9a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0z" />
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
                  style={{
                    display: 'flex', alignItems: 'center', gap: 6,
                    background: 'none', border: 'none', cursor: isLockedMonitor ? 'not-allowed' : 'pointer',
                    fontFamily: S.body, fontSize: T.heading4,
                    fontWeight: tab === id ? 500 : 400,
                    color: isLockedMonitor ? C.faint : (tab === id ? C.text : C.faint),
                    padding: '11px 16px',
                    paddingLeft: id === 'monitor' ? 0 : 16,
                    borderBottom: tab === id && !isLockedMonitor ? `2px solid ${C.tealMid}` : '2px solid transparent',
                    opacity: isLockedMonitor ? 0.65 : 1,
                    transition: 'color 0.15s',
                  }}
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

          {/* tab content */}
          <div
            className="dp-scroll"
            ref={contentViewportRef}
            style={{
              flex: 1,
              overflowY: 'auto',
              overflowX: 'auto',
              padding: `24px calc(${DETAIL_HORIZONTAL_PAD} + 4px) 24px`,
            }}
          >
            <div
              ref={contentInnerRef}
              style={{
                transform: contentScale < 1 ? `scale(${contentScale})` : undefined,
                transformOrigin: "top left",
                width: contentScale < 1 ? `${100 / contentScale}%` : "100%",
              }}
            >
              {showConfigureAsPage ? (
                <ConfigurePanel
                  deployment={renderedDeployment}
                  account={account}
                  fullPage
                  onClose={() => setConfigOpen(false)}
                  onRedeployStart={() => {
                    setOptimisticDeploying(true);
                  }}
                  onRedeploy={() => {
                    setOptimisticDeploying(true);
                    onRedeploy?.();
                  }}
                />
              ) : tab === 'monitor' ? (
                <MonitorTab deployment={renderedDeployment} />
              ) : (
                <DeploymentsTab
                  deployment={renderedDeployment}
                  account={account}
                  onOpenConfigure={() => setConfigOpen(true)}
                />
              )}
            </div>
          </div>
        </div>

      </div>
      </div>

      {/* right: configure panel — sticky so it always aligns with the agent header */}
      {!isCompact && (
        <div
          style={{
            position: 'sticky',
            top: 0,
            alignSelf: 'flex-start',
            height: '100vh',
            width: configOpen ? CONFIG_PANEL_WIDTH_PX : 0,
            flexShrink: 0,
            overflowX: 'clip',
            transition: 'width 0.3s cubic-bezier(0.16, 1, 0.3, 1)',
            zIndex: 45,
          }}
        >
          {configOpen && (
            <ConfigurePanel
              deployment={renderedDeployment}
              account={account}
              onClose={() => setConfigOpen(false)}
              onRedeployStart={() => {
                setOptimisticDeploying(true);
              }}
              onRedeploy={() => {
                setOptimisticDeploying(true);
                onRedeploy?.();
              }}
            />
          )}
        </div>
      )}
    </div>
  )
}
