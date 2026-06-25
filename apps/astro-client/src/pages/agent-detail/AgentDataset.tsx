import { useCallback, useRef, useState } from "react";
import { Download } from "lucide-react";
import { useSearchParams } from "react-router";
import { useAgentDetailContext } from "../AgentDetail";
import { useEvalDataset } from "@/api/queries/evals";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { DatasetGrade } from "@/components/agent-detail/evals/DatasetGrade";
import { DatasetView } from "@/components/agent-detail/evals/DatasetView";
import { ReviewQueueView } from "@/components/agent-detail/evals/ReviewQueueView";
import { TraceDetailPanel } from "@/components/agent-detail/traces/TraceDetailPanel";
import { TabButton } from "@/pages/AccountProfile/TabToolbar";
import { useContainerSize } from "@/hooks/use-container-size";
import type { TraceEntry } from "@/lib/api";
import { cn } from "@/lib/utils";

type Tab = "queue" | "dataset";

const DEFAULT_TAB: Tab = "queue";
const PANEL_WIDTH_REM = 41;
const OVERLAY_THRESHOLD = 900;

function parseTab(value: string | null): Tab {
  return value === "dataset" ? "dataset" : "queue";
}

export default function AgentDataset() {
  const { deployment, deploymentId, account } = useAgentDetailContext();
  const { data, isLoading, isError } = useEvalDataset(deploymentId);
  const [searchParams, setSearchParams] = useSearchParams();
  const gradeTargetRef = useRef<HTMLDivElement | null>(null);
  const [selectedTrace, setSelectedTrace] = useState<TraceEntry | null>(null);
  const [panelExpanded, setPanelExpanded] = useState(false);
  const { ref: outerRef, width: outerWidth } = useContainerSize();
  const reviewQueueTargetRef = useRef<HTMLSpanElement | null>(null);
  const tab = parseTab(searchParams.get("tab"));
  const isReviewQueue = tab === "queue";
  const panelOpen = selectedTrace !== null;
  const shouldOverlay = outerWidth > 0 && outerWidth < OVERLAY_THRESHOLD;
  const isFullWidth = panelExpanded || shouldOverlay;

  const openTracePanel = useCallback((trace: TraceEntry) => {
    setSelectedTrace(trace);
    setPanelExpanded(false);
  }, []);

  const closeTracePanel = useCallback(() => {
    setSelectedTrace(null);
  }, []);

  const setTab = useCallback(
    (next: Tab) => {
      setSelectedTrace(null);
      setPanelExpanded(false);
      setSearchParams(
        (prev) => {
          const out = new URLSearchParams(prev);
          if (next === DEFAULT_TAB) out.delete("tab");
          else out.set("tab", next);
          return out;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const downloadUrl = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/download`;

  return (
    <div ref={outerRef} className="relative z-10 flex flex-1 overflow-hidden pt-16">
      <div
        className="relative z-10 min-h-0 flex-1 overflow-y-auto transition-[padding] duration-300 ease-out"
        style={{
          paddingRight: panelOpen && !isFullWidth ? `${PANEL_WIDTH_REM}rem` : undefined,
        }}
      >
        <div
          className={cn(
            "mx-auto flex w-full max-w-6xl flex-col px-6 pt-8",
            isReviewQueue ? "pb-6" : "pb-16",
          )}
        >
          <div className="mb-5 min-w-0">
            <h1 className="text-heading-1 text-foreground">Eval</h1>
            <p className="mt-1.5 max-w-[66ch] text-body-sm text-muted-foreground">
              Measure how well your agent performs by testing it against a set of examples.
            </p>
          </div>

          <div className="mb-5 flex items-end justify-between gap-4 border-b border-border">
            <div className="flex items-center gap-6">
              <span
                ref={reviewQueueTargetRef}
                data-eval-review-queue-target
                className="inline-flex"
              >
                <TabButton active={tab === "queue"} onClick={() => setTab("queue")}>
                  Review queue
                </TabButton>
              </span>
              <TabButton active={tab === "dataset"} onClick={() => setTab("dataset")}>
                Dataset
                {data && (
                  <span className="ml-1.5 tabular-nums">
                    · {data.item_count.toLocaleString()}
                  </span>
                )}
              </TabButton>
            </div>
            {data && (
              <div className="flex flex-none items-center gap-2.5 pb-2">
                <div className="flex items-center gap-2 pr-1.5">
                  <span className="text-body-sm text-muted-foreground">Grade</span>
                  <div
                    ref={gradeTargetRef}
                    data-eval-grade-target
                    className="inline-flex rounded-sm"
                  >
                    <DatasetGrade grade={data.grade} />
                  </div>
                </div>
                <span aria-hidden className="h-5 w-px bg-border" />
                <Button asChild variant="outline" size="sm">
                  <a href={downloadUrl} download>
                    <Download className="size-4" />
                    Download
                  </a>
                </Button>
              </div>
            )}
          </div>

          {isLoading && (
            <div className="flex h-48 items-center justify-center">
              <Spinner delay={300} />
            </div>
          )}

          {isError && !isLoading && (
            <p className="text-body-sm text-muted-foreground">Dataset not available.</p>
          )}

          {data && tab === "dataset" && (
            <DatasetView
              deploymentId={deploymentId}
              account={account}
              summary={data}
              reviewQueueTargetRef={reviewQueueTargetRef}
            />
          )}
          {data && tab === "queue" && (
            <ReviewQueueView
              deploymentId={deploymentId}
              account={account}
              agentName={deployment.name}
              agentLabel={deployment.display_name || deployment.name}
              agentAvatarUrl={deployment.avatar_url}
              summary={data}
              gradeTargetRef={gradeTargetRef}
              onOpenTrace={panelOpen ? undefined : openTracePanel}
            />
          )}
        </div>
      </div>

      <div
        className={cn(
          "absolute z-20 transition-[transform,inset,width] duration-300 ease-out",
          isFullWidth
            ? "inset-3 top-20"
            : "bottom-3 right-3 top-20 w-[40rem]",
        )}
        style={{
          transform: panelOpen
            ? "translateX(0)"
            : "translateX(calc(100% + 0.75rem))",
        }}
      >
        {selectedTrace && (
          <TraceDetailPanel
            deploymentId={deploymentId}
            trace={selectedTrace}
            account={account}
            onClose={closeTracePanel}
            expanded={panelExpanded}
            onToggleExpanded={
              shouldOverlay ? undefined : () => setPanelExpanded((value) => !value)
            }
          />
        )}
      </div>
    </div>
  );
}
