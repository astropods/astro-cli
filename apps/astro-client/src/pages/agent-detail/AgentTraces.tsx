import { useCallback, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { Loader2 } from "lucide-react";
import { useAgentDetailContext } from "../AgentDetail";
import {
  useObservabilityTraceDetail,
  useObservabilityTraceUsers,
  useObservabilityTracesInfinite,
  TRACES_PAGE_SIZE,
} from "@/api/queries/observability";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { TraceDetailPanel } from "@/components/agent-detail/traces/TraceDetailPanel";
import { ContentReveal } from "@/components/ui/content-reveal";
import {
  TracesTable,
  traceUserFilterParams,
  type TraceSortState,
} from "@/components/agent-detail/traces/TracesTable";
import {
  buildTimeParams,
  DAY_RANGES,
  type DayRange,
} from "@/components/agent-detail/charts/chart-utils";
import { Button } from "@/components/ui/button";
import { useContainerSize } from "@/hooks/use-container-size";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import type { TraceEntry } from "@/lib/api";
import { cn } from "@/lib/utils";

const OVERLAY_THRESHOLD = 900;
const PANEL_WIDTH_REM = 41;

export default function AgentTraces() {
  const { deploymentId, account } = useAgentDetailContext();
  const [searchParams, setSearchParams] = useSearchParams();
  const rangeParam = searchParams.get("window");
  const range: DayRange = DAY_RANGES.some((option) => option.key === rangeParam)
    ? (rangeParam as DayRange)
    : "7d";
  const { days } = DAY_RANGES.find((option) => option.key === range)!;
  const traceParams = useMemo(() => buildTimeParams(days), [days]);

  const [search, setSearch] = useState("");
  const [selectedUserKey, setSelectedUserKey] = useState<string | null>(null);
  const [sort, setSort] = useState<TraceSortState>({
    key: "timestamp",
    direction: "desc",
  });
  const [visibleTraceLimit, setVisibleTraceLimit] = useState(TRACES_PAGE_SIZE);
  const debouncedSearch = useDebouncedValue(search.trim(), 250);
  const queryParams = useMemo(() => ({
    ...traceParams,
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
    ...traceUserFilterParams(selectedUserKey),
    sort: sort.key,
    direction: sort.direction,
  }), [
    debouncedSearch,
    selectedUserKey,
    sort.direction,
    sort.key,
    traceParams,
  ]);
  const {
    data: tracesData,
    isLoading: tracesLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useObservabilityTracesInfinite(deploymentId, queryParams, { window: range });
  const { data: traceUsersData } = useObservabilityTraceUsers(
    deploymentId,
    traceParams,
    { window: range },
  );

  const allTraces = useMemo(
    () => tracesData?.pages.flatMap((page) => page.traces) ?? [],
    [tracesData],
  );
  const visibleTraces = useMemo(
    () => allTraces.slice(0, visibleTraceLimit),
    [allTraces, visibleTraceLimit],
  );

  const totalTraceCount = tracesData?.pages[0]?.total ?? allTraces.length;
  const truncatedPage = tracesData?.pages.find((page) => page.truncated);
  const hasMoreTraces = visibleTraceLimit < allTraces.length || hasNextPage;
  const revealedTraceCount = Math.max(
    visibleTraces.length - TRACES_PAGE_SIZE,
    0,
  );

  const selectedTraceId = searchParams.get("trace");
  const selectedIndex = selectedTraceId
    ? visibleTraces.findIndex((trace) => trace.trace_id === selectedTraceId)
    : -1;
  const traceFromList = selectedIndex >= 0 ? visibleTraces[selectedIndex] : null;
  const needsHydration = !!selectedTraceId && !tracesLoading && !traceFromList;
  const { data: hydratedDetail, isError: hydrationError } = useObservabilityTraceDetail(
    deploymentId,
    needsHydration ? selectedTraceId : null,
  );
  const hydratedTrace = useMemo<TraceEntry | null>(() => {
    const trace = hydratedDetail?.trace;
    if (!needsHydration || !trace) return null;
    return {
      trace_id: trace.trace_id,
      name: trace.name,
      status: "",
      latency_ms: trace.latency_ms,
      total_cost: trace.total_cost,
      timestamp: trace.timestamp,
      user_id: trace.user_id,
      user_details: trace.user_details,
    };
  }, [hydratedDetail, needsHydration]);
  const selectedTrace = traceFromList ?? hydratedTrace;

  const updateSearchParams = useCallback(
    (update: (next: URLSearchParams) => void, options?: { replace?: boolean }) => {
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        update(next);
        return next;
      }, options);
    },
    [setSearchParams],
  );
  const setRange = useCallback(
    (nextRange: DayRange) => {
      setVisibleTraceLimit(TRACES_PAGE_SIZE);
      updateSearchParams((next) => next.set("window", nextRange), { replace: true });
    },
    [updateSearchParams],
  );
  const setSelectedTraceId = useCallback(
    (traceId: string | null, options?: { replace?: boolean }) => {
      updateSearchParams((next) => {
        if (traceId) next.set("trace", traceId);
        else next.delete("trace");
      }, options);
    },
    [updateSearchParams],
  );
  const handleNavigate = useCallback(
    (direction: "prev" | "next") => {
      const nextIndex = direction === "prev" ? selectedIndex - 1 : selectedIndex + 1;
      if (nextIndex >= 0 && nextIndex < visibleTraces.length) {
        setSelectedTraceId(visibleTraces[nextIndex].trace_id, { replace: true });
      }
    },
    [selectedIndex, setSelectedTraceId, visibleTraces],
  );
  const handleShowMore = useCallback(() => {
    setVisibleTraceLimit((limit) => limit + TRACES_PAGE_SIZE);
    if (
      visibleTraceLimit >= allTraces.length &&
      hasNextPage &&
      !isFetchingNextPage
    ) {
      void fetchNextPage();
    }
  }, [
    allTraces.length,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    visibleTraceLimit,
  ]);
  const handleShowLess = useCallback(() => {
    if (selectedIndex >= TRACES_PAGE_SIZE) {
      setSelectedTraceId(null, { replace: true });
    }
    setVisibleTraceLimit(TRACES_PAGE_SIZE);
  }, [selectedIndex, setSelectedTraceId]);

  const { ref: outerRef, width: outerWidth } = useContainerSize();
  const [panelExpanded, setPanelExpanded] = useState(false);
  const traceNotFound = needsHydration && hydrationError;
  const traceHydrating = !!selectedTraceId && !selectedTrace && !traceNotFound;
  const panelOpen = selectedTraceId !== null;
  const shouldOverlay = outerWidth > 0 && outerWidth < OVERLAY_THRESHOLD;
  const panelOverlaysContent = panelExpanded || shouldOverlay;

  return (
    <div ref={outerRef} className="relative z-10 flex flex-1 overflow-hidden pt-16">
      <div
        className="dp-scroll relative z-10 min-h-0 flex-1 overflow-y-auto transition-[padding] duration-300 ease-out"
        style={{
          paddingRight:
            panelOpen && !panelOverlaysContent
              ? `${PANEL_WIDTH_REM}rem`
              : undefined,
          maskImage: "linear-gradient(to bottom, transparent, black 2rem)",
          WebkitMaskImage: "linear-gradient(to bottom, transparent, black 2rem)",
        }}
      >
        <ContentReveal
          className="@container/traces-page mx-auto flex w-full max-w-6xl flex-col px-6 pt-8 pb-16"
        >
          <div className="mb-5 flex flex-col gap-3 pb-3 @[680px]/traces-page:flex-row @[680px]/traces-page:items-end @[680px]/traces-page:justify-between">
            <div className="min-w-0">
              <h1 className="text-heading-1 text-foreground">Traces</h1>
              <p className="mt-1.5 max-w-[66ch] text-body-sm text-foreground dark:text-muted-foreground">
                Review requests, users, latency, cost, and execution details.
              </p>
            </div>

            <TimeRangeSelector
              value={range}
              ranges={DAY_RANGES}
              onChange={(value) => setRange(value as DayRange)}
              layoutId="traces-range-pill"
            />
          </div>

          <TracesTable
            traces={visibleTraces}
            userFacets={traceUsersData?.users ?? []}
            account={account}
            search={search}
            onSearchChange={(value) => {
              setVisibleTraceLimit(TRACES_PAGE_SIZE);
              setSearch(value);
            }}
            selectedUserKey={selectedUserKey}
            onSelectedUserKeyChange={(value) => {
              setVisibleTraceLimit(TRACES_PAGE_SIZE);
              setSelectedUserKey(value);
            }}
            sort={sort}
            onSortChange={(value) => {
              setVisibleTraceLimit(TRACES_PAGE_SIZE);
              setSort(value);
            }}
            loading={tracesLoading}
            selectedTraceId={selectedTraceId}
            onSelectTrace={(trace) => setSelectedTraceId(trace.trace_id)}
            hasMore={hasMoreTraces}
            onLoadMore={handleShowMore}
            onShowLess={handleShowLess}
            revealedCount={revealedTraceCount}
            loadingMore={isFetchingNextPage}
            totalCount={totalTraceCount}
            loadedCount={allTraces.length}
            resultsTruncated={!!truncatedPage}
            scannedCount={truncatedPage?.scanned_count}
          />
        </ContentReveal>
      </div>

      <div
        className={cn(
          "absolute z-20 transition-[transform,inset,width] duration-300 ease-out",
          panelOverlaysContent
            ? "inset-3 top-20"
            : "bottom-3 right-3 top-20 w-[40rem]",
        )}
        style={{
          transform: panelOpen
            ? "translateX(0)"
            : "translateX(calc(100% + 0.75rem))",
        }}
        aria-hidden={!panelOpen}
      >
        {selectedTrace ? (
          <TraceDetailPanel
            deploymentId={deploymentId}
            trace={selectedTrace}
            account={account}
            onClose={() => setSelectedTraceId(null, { replace: true })}
            canGoPrev={selectedIndex > 0}
            canGoNext={selectedIndex >= 0 && selectedIndex < visibleTraces.length - 1}
            onNavigate={handleNavigate}
            expanded={panelOverlaysContent}
            onToggleExpanded={shouldOverlay ? undefined : () => setPanelExpanded((value) => !value)}
          />
        ) : traceNotFound ? (
          <div className="flex h-full w-full flex-col items-center justify-center gap-3 px-6 text-center">
            <p className="text-body-sm text-foreground">Trace not found.</p>
            <p className="text-body-sm text-muted-foreground">
              It may be outside the selected time range or no longer available.
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setSelectedTraceId(null, { replace: true })}
              className="mt-1"
            >
              Close
            </Button>
          </div>
        ) : traceHydrating ? (
          <div className="flex h-full w-full items-center justify-center">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : null}
      </div>
    </div>
  );
}
