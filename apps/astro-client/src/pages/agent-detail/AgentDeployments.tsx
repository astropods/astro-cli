import { useCallback, useState } from "react";
import { motion } from "motion/react";
import { useAgentDetailContext } from "../AgentDetail";
import { PodGraph } from "@/components/agent-detail/pods/PodGraph";
import { PodTile } from "@/components/agent-detail/pods/PodTile";
import { DeploymentHistoryPanel } from "@/components/agent-detail/deployments/DeploymentHistoryPanel";
import { PodDetailPanel } from "@/components/agent-detail/pods/PodDetailPanel";
import { useContainerSize } from "@/hooks/use-container-size";

const PANEL_SPRING = { type: "spring" as const, bounce: 0.12, duration: 0.5 };
const PANEL_WIDTH_REM = 43; // 42rem pod detail panel + 1rem gap
const SIDEBAR_WIDTH_REM = 26.75;
const OVERLAY_THRESHOLD = 800; // Below this, sidebar overlays instead of sitting side-by-side
const POD_OVERLAY_THRESHOLD = 1050; // Below this, pod detail panel overlays

function remToPx(rem: number) {
  if (typeof document === "undefined") return rem * 16;
  return rem * parseFloat(getComputedStyle(document.documentElement).fontSize);
}

export default function AgentDeployments() {
  const {
    deployment, account,
  } = useAgentDetailContext();
  const workloads = deployment.workloads ?? [];
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
            effectiveWidth={effectiveWidth}
            renderTile={(i) => (
              <PodTile
                workload={workloads[i]}
                deploymentId={deployment.id}
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
