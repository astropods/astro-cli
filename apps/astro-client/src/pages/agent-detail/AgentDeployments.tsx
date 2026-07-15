import { useCallback, useMemo, useRef, useState } from "react";
import { motion } from "motion/react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useAgentDetailContext } from "../AgentDetail";
import { PodGraph } from "@/components/agent-detail/pods/PodGraph";
import { PodTile } from "@/components/agent-detail/pods/PodTile";
import { DeploymentHistoryPanel } from "@/components/agent-detail/deployments/DeploymentHistoryPanel";
import { useDeployStuckByAge } from "@/components/agent-detail/deployments/use-stuck-deploy";
import { PodDetailPanel } from "@/components/agent-detail/pods/PodDetailPanel";
import { useContainerSize } from "@/hooks/use-container-size";
import { useDeploymentEvents, useDeploymentHistory, useDeploymentStatus, useStopDeployment } from "@/api/queries/deployments";
import { ActionPanel } from "@/components/ui/status-panel";
import { isPausedState } from "@/lib/deployment-utils";
import type { WorkloadDetail } from "@/lib/api";

const PANEL_SPRING = { type: "spring" as const, bounce: 0.12, duration: 0.5 };
const PANEL_WIDTH_REM = 43; // 42rem pod detail panel + 1rem gap
const SIDEBAR_WIDTH_REM = 26.75;
const OVERLAY_THRESHOLD = 800; // Below this, sidebar overlays instead of sitting side-by-side
const POD_OVERLAY_THRESHOLD = 1050; // Below this, pod detail panel overlays
const MOBILE_TOP_INSET = 64; // Clears the tab bar / identity controls overlaying the top
const PANEL_EDGE_GAP = 24; // Gap between the mobile scroll list and the bottom panel (bottom-3 + margin)
// History statuses that mean a past deploy never came up, so it is not a safe
// rollback target.
const FAILED_DEPLOY_STATUSES = new Set(["failed", "error"]);
const STUCK_DEPLOY_DOCS_URL = "https://docs.astropods.com/troubleshooting-deployments";

function remToPx(rem: number) {
  if (typeof document === "undefined") return rem * 16;
  return rem * parseFloat(getComputedStyle(document.documentElement).fontSize);
}

export default function AgentDeployments() {
  const {
    deployment, runtime, account,
  } = useAgentDetailContext();
  const paused = isPausedState(deployment);

  // Stuck-deploy banner. Detection is event-driven first: while deploying, the
  // server surfaces the blocking Kubernetes event with a humanized title +
  // guidance and a severity, so a real cause (image pull, crash loop, failed
  // scheduling) drives the banner immediately and names itself. A real deploy
  // age (from the server's status_changed_at, so it survives reloads) is the
  // backstop. A redeploy usually hits the same wall, so the headline action is
  // a rollback to the last version that deployed cleanly, with pause alongside.
  const navigate = useNavigate();
  const { data: statusData } = useDeploymentStatus(deployment.id);
  const isDeploying = statusData?.value === "deploying";
  const stopMutation = useStopDeployment(account);
  const { data: history } = useDeploymentHistory(account, deployment.name, deployment.id, isDeploying);
  const { data: events } = useDeploymentEvents(deployment.id, isDeploying);
  // The most recent stuck-severity warning names the specific cause.
  const stuckCause = useMemo(() => {
    const stuck = (events?.events ?? []).filter((e) => e.severity === "stuck" && e.title && e.guidance);
    return stuck.sort((a, b) => b.last_timestamp.localeCompare(a.last_timestamp))[0] ?? null;
  }, [events]);
  const stuckByAge = useDeployStuckByAge(statusData?.status_changed_at, isDeploying);
  const showStuckBanner = isDeploying && (stuckCause !== null || stuckByAge);
  // Last good version = the highest-revision past deploy that is not the current
  // one and did not fail. null on a first deploy, when the banner falls back to
  // Pause as the primary action.
  const lastGood = useMemo(() => {
    const recs = [...(history?.deployments ?? [])].sort((a, b) => b.revision - a.revision);
    return recs.find((r) => !r.is_current && !FAILED_DEPLOY_STATUSES.has((r.status ?? "").toLowerCase())) ?? null;
  }, [history]);
  const pauseDeploy = useCallback(
    () => stopMutation.mutate({ deploymentId: deployment.id }),
    [stopMutation, deployment.id],
  );
  const rollBack = useCallback(() => {
    if (!lastGood) return;
    navigate(`../configure?revision=${lastGood.revision}&build=${encodeURIComponent(lastGood.build_id)}`, {
      relative: "path",
    });
  }, [navigate, lastGood]);
  // On the specific-cause variant, hand the named cause to Claude Code so it can
  // dig into the pod logs and propose a fix.
  const { copy } = useCopyToClipboard();
  const copyFixPrompt = useCallback(async () => {
    const cause = stuckCause ? `: ${stuckCause.title}. ${stuckCause.guidance}` : " in the deploying state";
    const prompt = `My Astro deployment "${deployment.name}" is stuck${cause} Read the pod logs and events, confirm the cause, and tell me how to fix it.`;
    if (await copy(prompt)) toast("Fix prompt copied");
    else toast.error("Couldn't copy the prompt");
  }, [copy, deployment.name, stuckCause]);

  // Merge record (spec) + runtime (live) workloads by name, keyed by the
  // union of both sides. The SPEC list is the stable source of truth for
  // which tiles to render — it doesn't change on pause/resume/redeploy.
  // Live state from runtime is overlaid where available, but the tile's
  // presence is never gated on it. This eliminates the flicker that
  // happened on pause/resume when runtime briefly reported zero containers
  // mid-transition and tiles disappeared. The PodTile itself decides what
  // to show: "Paused" (whole agent off), "Probing" (runtime still loading),
  // or the K8s-derived per-workload status. Runtime-only entries (e.g.
  // manual-trigger ingestion firings whose spec row was filtered at
  // normalization) still get a tile via the name union.
  const workloads = useMemo<WorkloadDetail[]>(() => {
    const specByName = new Map((deployment.workloads ?? []).map((w) => [w.name, w]));
    const liveByName = new Map((runtime?.workloads ?? []).map((w) => [w.name, w]));
    const names = new Set<string>([...specByName.keys(), ...liveByName.keys()]);
    return Array.from(names).map((name) => ({
      // Spec defaults — make sure required-for-component fields exist when
      // we're rendering a runtime-only entry (e.g. manual ingestion pod).
      kind: specByName.get(name)?.kind ?? "Pod",
      component: specByName.get(name)?.component ?? "",
      ...specByName.get(name),
      ...liveByName.get(name),
      name,
    }));
  }, [deployment.workloads, runtime]);
  const [selectedPodIndex, setSelectedPodIndex] = useState<number | null>(null);
  const [podPanelExpanded, setPodPanelExpanded] = useState(false);

  const selectedWorkload =
    selectedPodIndex !== null && selectedPodIndex < workloads.length
      ? workloads[selectedPodIndex]
      : null;

  // Track outer container width
  const { ref: outerRef, width: outerWidth } = useContainerSize();

  const panelOpen = selectedWorkload !== null;
  const threshold = panelOpen ? POD_OVERLAY_THRESHOLD : OVERLAY_THRESHOLD;
  const shouldOverlay = outerWidth > 0 && outerWidth < threshold;
  const podPanelFullWidth = panelOpen && (podPanelExpanded || shouldOverlay);
  const effectiveWidth = panelOpen && !shouldOverlay && !podPanelExpanded
    ? outerWidth - remToPx(PANEL_WIDTH_REM)
    : outerWidth;

  const [expanded, setExpanded] = useState(false);

  // Insets that clear the page chrome overlaying the graph (only take effect in
  // PodGraph's vertical scroll mode). The top bar is always present, so the top
  // inset is unconditional; the bottom inset applies only when the deployment
  // panel is a full-width bottom overlay (measured so its real height is used).
  const { ref: bottomPanelRef, height: bottomPanelHeight } = useContainerSize();
  const liveGraph = {
    insetTop: MOBILE_TOP_INSET,
    insetBottom: shouldOverlay ? bottomPanelHeight + PANEL_EDGE_GAP : 0,
    effectiveWidth,
  };
  // Freeze those inputs while a panel fully covers the graph (mobile pod detail
  // or expanded history): the graph is hidden, so reflowing it would only flash
  // behind the panel's open/close animation. Restores to live values on close.
  const graphCovered = podPanelFullWidth || (expanded && shouldOverlay);
  const stableGraph = useRef(liveGraph);
  if (!graphCovered) stableGraph.current = liveGraph;
  const { insetTop: graphInsetTop, insetBottom: graphInsetBottom, effectiveWidth: graphEffectiveWidth } =
    graphCovered ? stableGraph.current : liveGraph;

  const toggleExpanded = useCallback(() => {
    setExpanded((prev) => !prev);
  }, []);

  const togglePodPanelExpanded = useCallback(() => {
    setPodPanelExpanded((prev) => !prev);
  }, []);

  const handlePodClick = useCallback((index: number) => {
    setSelectedPodIndex((prev) => (prev === index ? null : index));
    setPodPanelExpanded(false);
  }, []);

  const handleClosePodPanel = useCallback(() => {
    setSelectedPodIndex(null);
    setPodPanelExpanded(false);
  }, []);

  // Shift graph left to center in the visible area beside the panel.
  // In overlay/expanded mode: no translate, graph stays centered.
  const graphTransform = panelOpen && !shouldOverlay && !podPanelExpanded
    ? { transform: `translateX(calc(-${PANEL_WIDTH_REM}rem / 2))` }
    : !panelOpen && expanded && !shouldOverlay
    ? { transform: `translateX(calc(-${SIDEBAR_WIDTH_REM}rem / 2))` }
    : undefined;

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {showStuckBanner && (
        // pt-24 clears the absolutely-positioned tab bar and the agent identity
        // header (both anchored at top-4); max-w-3xl keeps the copy readable.
        <div className="mx-auto w-full max-w-3xl shrink-0 px-4 pt-24">
          <ActionPanel
            tone="warning"
            title={stuckCause?.title ?? "This deploy is stuck"}
            primaryLabel={lastGood ? "Roll back" : "Pause"}
            onPrimary={lastGood ? rollBack : pauseDeploy}
            secondaryLabel={lastGood ? "Pause" : undefined}
            onSecondary={lastGood ? pauseDeploy : undefined}
          >
            <p>
              {stuckCause
                ? `${stuckCause.guidance} `
                : lastGood
                  ? "Rolling back to the last clean version is usually the fastest fix, or pause to investigate. "
                  : "Pause to investigate, or check the pod logs below for what is holding it up. "}
              <a
                href={STUCK_DEPLOY_DOCS_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="underline underline-offset-2 hover:opacity-80"
              >
                Why deploys get stuck
              </a>
              {stuckCause && (
                <>
                  <span aria-hidden className="mx-1.5 opacity-50">&middot;</span>
                  <button
                    type="button"
                    onClick={copyFixPrompt}
                    className="underline underline-offset-2 hover:opacity-80"
                  >
                    Copy fix prompt
                  </button>
                </>
              )}
            </p>
          </ActionPanel>
        </div>
      )}
      <div ref={outerRef} className="relative flex flex-1 overflow-hidden">
        {/* Graph wrapper — translates as a unit, PodGraph stays full size */}
        <div
          className="flex w-full shrink-0 transition-transform duration-300 ease-out"
          style={graphTransform}
        >
          {workloads.length === 0 ? (
            <div className="flex flex-1 items-center justify-center">
              <p className="text-muted-foreground text-sm">No active pods</p>
            </div>
          ) : (
            <PodGraph
              count={workloads.length}
              keys={workloads.map((w) => w.name)}
              components={workloads.map((w) => w.component)}
              kinds={workloads.map((w) => w.kind)}
              effectiveWidth={graphEffectiveWidth}
              insetTop={graphInsetTop}
              insetBottom={graphInsetBottom}
              renderTile={(i) => (
                <PodTile
                  workload={workloads[i]}
                  deploymentId={deployment.id}
                  deployment={deployment}
                  // While the runtime query hasn't returned, render each tile
                  // in the grey blinking "Probing" state so the user can tell
                  // the difference between "we don't know yet" and "K8s says
                  // pending/starting".
                  probing={runtime === undefined}
                  // When the whole deployment is paused, every tile renders as
                  // "Paused" regardless of its individual K8s status — see
                  // PodTile's status precedence rules.
                  paused={paused}
                  onClick={() => handlePodClick(i)}
                  selected={selectedPodIndex === i}
                  dimmed={selectedPodIndex !== null && selectedPodIndex !== i}
                />
              )}
            />
          )}
        </div>

        {/* Panel */}
        {selectedWorkload ? (
          <motion.div
            layoutId="deployment-panel"
            className={
              podPanelFullWidth
                ? "absolute inset-3 top-20 z-20"
                : "absolute bottom-3 right-3 top-20 z-20 w-[42rem]"
            }
            transition={PANEL_SPRING}
          >
            <PodDetailPanel
              workload={selectedWorkload}
              deploymentId={deployment.id}
              externalUrls={deployment.external_urls}
              paused={paused}
              probing={runtime === undefined}
              onClose={handleClosePodPanel}
              expanded={podPanelExpanded}
              onToggleExpanded={shouldOverlay ? undefined : togglePodPanelExpanded}
            />
          </motion.div>
        ) : expanded ? (
          <motion.div
            layoutId="deployment-panel"
            className={shouldOverlay ? "absolute inset-3 top-20 z-20" : "absolute bottom-3 right-3 top-20 z-20 w-[26rem]"}
            transition={PANEL_SPRING}
          >
            <DeploymentHistoryPanel
              account={account}
              agentName={deployment.name}
              deploymentId={deployment.id}
              deployment={deployment}

              expanded
              onToggleExpanded={toggleExpanded}
            />
          </motion.div>
        ) : (
          <motion.div
            ref={bottomPanelRef}
            layoutId="deployment-panel"
            className={shouldOverlay ? "absolute inset-x-3 bottom-3 z-20" : "absolute bottom-3 right-3 z-20 w-[26rem]"}
            transition={PANEL_SPRING}
          >
            <DeploymentHistoryPanel
              account={account}
              agentName={deployment.name}
              deploymentId={deployment.id}
              deployment={deployment}

              onToggleExpanded={toggleExpanded}
            />
          </motion.div>
        )}
      </div>
    </div>
  );
}
