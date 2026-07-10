import { useCallback, useMemo, useRef, useState } from "react";
import { motion } from "motion/react";
import { useAgentDetailContext } from "../AgentDetail";
import { PodGraph } from "@/components/agent-detail/pods/PodGraph";
import { PodTile } from "@/components/agent-detail/pods/PodTile";
import { DeploymentHistoryPanel } from "@/components/agent-detail/deployments/DeploymentHistoryPanel";
import { PodDetailPanel } from "@/components/agent-detail/pods/PodDetailPanel";
import { useContainerSize } from "@/hooks/use-container-size";
import { isPausedState } from "@/lib/deployment-utils";
import type { WorkloadDetail } from "@/lib/api";

const PANEL_SPRING = { type: "spring" as const, bounce: 0.12, duration: 0.5 };
const PANEL_WIDTH_REM = 43; // 42rem pod detail panel + 1rem gap
const SIDEBAR_WIDTH_REM = 26.75;
const OVERLAY_THRESHOLD = 800; // Below this, sidebar overlays instead of sitting side-by-side
const POD_OVERLAY_THRESHOLD = 1050; // Below this, pod detail panel overlays
const MOBILE_TOP_INSET = 64; // Clears the tab bar / identity controls overlaying the top
const PANEL_EDGE_GAP = 24; // Gap between the mobile scroll list and the bottom panel (bottom-3 + margin)

function remToPx(rem: number) {
  if (typeof document === "undefined") return rem * 16;
  return rem * parseFloat(getComputedStyle(document.documentElement).fontSize);
}

export default function AgentDeployments() {
  const {
    deployment, runtime, account,
  } = useAgentDetailContext();
  const paused = isPausedState(deployment);
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
  );
}
