import type { LucideIcon } from "lucide-react";
import { ArrowRight, Check, Frown, Meh, Smile, UserRound } from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { StatusBadge } from "@/components/StatusBadge";
import { ContentSection } from "@/components/agent-detail/traces/detail/ContentSection";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type {
  DatasetJudgmentVerdict,
  ReviewQueueItem,
  ReviewQueueSentiment,
} from "@/lib/api";
import { ReviewQueueLoadMoreButton } from "./ReviewQueueList";
import { ReviewQueueVerdictControls } from "./ReviewQueueVerdictControls";
import { truncateTraceId } from "./review-queue-utils";

// Let the dataset page own vertical scrolling; resized sections still scroll
// once the user gives them an explicit height.
const REVIEW_QUEUE_CONTENT_CLASS =
  "dp-scroll overflow-y-auto overscroll-contain";

const SENTIMENT_BADGE: Record<
  ReviewQueueSentiment,
  { label: string; color: "success" | "error" | "muted"; Icon: LucideIcon }
> = {
  positive: { label: "Likely positive", color: "success", Icon: Smile },
  negative: { label: "Likely negative", color: "error", Icon: Frown },
  "": { label: "No signal", color: "muted", Icon: Meh },
};

export function ReviewQueueDetail({
  item,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
  onJudge,
  isJudging,
  activeVerdict,
  showJudgmentError,
  position,
  queueSize,
  onOpenTrace,
}: {
  item: ReviewQueueItem;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl: string;
  onJudge: (
    traceId: string,
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => void;
  isJudging: boolean;
  activeVerdict: DatasetJudgmentVerdict | null;
  showJudgmentError: boolean;
  position: number;
  queueSize: number;
  onOpenTrace?: () => void;
}) {
  const sentiment = SENTIMENT_BADGE[item.sentiment] ?? SENTIMENT_BADGE[""];
  const traceLabel = truncateTraceId(item.trace_id);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-none flex-wrap items-center justify-between gap-x-4 gap-y-3 border-b border-border px-4 py-4 @[520px]/review-card:px-6">
        <ReviewQueueVerdictControls
          isPending={isJudging}
          activeVerdict={activeVerdict}
          showError={showJudgmentError}
          onSelect={(verdict, trigger) =>
            onJudge(item.trace_id, verdict, trigger)
          }
        />
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-3">
          <TraceDetailHoverLink
            traceId={item.trace_id}
            traceLabel={traceLabel}
            onOpenTrace={onOpenTrace}
          />
          <TooltipProvider delayDuration={300}>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex cursor-help">
                  <StatusBadge
                    color={sentiment.color}
                    className="flex-none gap-1.5 px-2.5 py-1 font-sans text-label font-medium tracking-normal"
                  >
                    <sentiment.Icon aria-hidden className="size-3" />
                    <span className="underline decoration-current/45 decoration-dotted underline-offset-2">
                      {sentiment.label}
                    </span>
                  </StatusBadge>
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-56 text-left">
                Inferred from keywords in the user's next reply. Only affects queue order,
                never the verdict.
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <ReviewQueueDetailNavigation position={position} total={queueSize} />
        </div>
      </div>

      <div
        data-review-queue-detail
        className="min-h-0 flex-1 p-4 @[520px]/review-card:p-6"
      >
        <div className="mx-auto flex max-w-3xl flex-col gap-5">
          <ContentSection
            label="User"
            content={item.input}
            icon={<UserSectionIcon />}
            mode="pretty"
            contentClassName={REVIEW_QUEUE_CONTENT_CLASS}
            resizableContent
          />
          <ContentSection
            label={agentLabel}
            content={item.output}
            mode="pretty"
            contentClassName={REVIEW_QUEUE_CONTENT_CLASS}
            resizableContent
            icon={
              <BlueprintIdentity
                account={account}
                name={agentName}
                size={20}
                url={agentAvatarUrl}
                className="size-5 rounded-sm"
              />
            }
          />
        </div>
      </div>
    </div>
  );
}

function TraceDetailHoverLink({
  traceId,
  traceLabel,
  onOpenTrace,
}: {
  traceId: string;
  traceLabel: string;
  onOpenTrace?: () => void;
}) {
  if (!onOpenTrace) {
    return (
      <span
        className="font-mono text-mono-sm text-muted-foreground"
        title={traceId}
      >
        {traceLabel}
      </span>
    );
  }

  return (
    <button
      type="button"
      aria-label={`View ${traceLabel}`}
      title={traceId}
      onClick={onOpenTrace}
      className={cn(
        "group/trace inline-flex cursor-pointer items-center gap-1.5 rounded-sm py-1 text-foreground transition-colors",
        "hover:text-foreground-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
      )}
    >
      <span className="font-sans text-body-sm font-medium">
        View {traceLabel}
      </span>
      <ArrowRight
        aria-hidden
        className="size-3.5 transition-transform group-hover/trace:translate-x-0.5 group-focus-visible/trace:translate-x-0.5"
      />
    </button>
  );
}

function ReviewQueueDetailNavigation({
  position,
  total,
}: {
  position: number;
  total: number;
}) {
  if (total <= 0) {
    return null;
  }

  return (
    <span
      className="min-w-12 text-center font-mono text-mono-sm text-muted-foreground tabular-nums"
      aria-label={`Trace ${position} of ${total}`}
    >
      {position} / {total}
    </span>
  );
}

function UserSectionIcon() {
  return (
    <span
      aria-hidden
      className="flex size-5 items-center justify-center rounded-full border border-border bg-muted text-muted-foreground"
    >
      <UserRound className="size-3" />
    </span>
  );
}

export function ReviewQueueDetailEmpty({
  showJudgmentError,
  canLoadMore,
  isLoadingMore,
  onLoadMore,
}: {
  showJudgmentError: boolean;
  canLoadMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}) {
  const title = canLoadMore ? "Ready for more traces" : "You're all caught up";
  const description = canLoadMore
    ? "Load the next page of queue items to keep reviewing."
    : "Every trace has a verdict.";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-1 flex-col items-center justify-center gap-3.5 p-6 text-center @[520px]/review-card:p-12">
        <span
          aria-hidden
          className="flex size-12 items-center justify-center rounded-full bg-success/10 text-success"
        >
          <Check className="size-6" />
        </span>
        <div>
          <div className="text-heading-3 font-semibold text-foreground">{title}</div>
          <p className="mt-1.5 text-body-sm text-muted-foreground">{description}</p>
        </div>
        {canLoadMore && (
          <ReviewQueueLoadMoreButton
            isLoading={isLoadingMore}
            onLoadMore={onLoadMore}
          />
        )}
      </div>
      {showJudgmentError && (
        <div className="flex flex-none items-center border-t border-border px-6 py-4 text-body-sm text-muted-foreground">
          Could not save verdict. Try again.
        </div>
      )}
    </div>
  );
}
