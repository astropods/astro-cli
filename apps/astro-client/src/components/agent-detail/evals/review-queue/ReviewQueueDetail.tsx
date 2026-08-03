import { Check, UserRound } from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { ContentSection } from "@/components/agent-detail/traces/detail/ContentSection";
import { TraceUserIdentity } from "@/components/agent-detail/traces/TraceUserIdentity";
import type { ReviewQueueItem } from "@/lib/api";
import { formatTimeAgo } from "@/lib/time-format";
import { ReviewQueueLoadMoreButton } from "./ReviewQueueList";

// Let the dataset page own vertical scrolling; resized sections still scroll
// once the user gives them an explicit height.
const REVIEW_QUEUE_CONTENT_CLASS =
  "dp-scroll overflow-y-auto overscroll-contain";

export function ReviewQueueDetail({
  item,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
}: {
  item: ReviewQueueItem;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl: string;
}) {
  const timestamp = item.timestamp ? formatTimeAgo(item.timestamp) : "";
  const userLabel =
    item.user_details?.display_name || item.user_details?.username || "User";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        data-review-queue-detail
        className="min-h-0 flex-1 p-4 @[520px]/review-card:p-6"
      >
        <div className="mx-auto flex max-w-3xl flex-col gap-5">
          <ContentSection
            label={
              item.user_id ? (
                <TraceUserIdentity
                  userId={item.user_id}
                  userDetails={item.user_details}
                  account={account}
                />
              ) : (
                "User"
              )
            }
            ariaLabel={userLabel}
            content={item.input}
            icon={!item.user_id ? <UserSectionIcon /> : undefined}
            headerMeta={<TraceTime value={timestamp} />}
            mode="pretty"
            contentClassName={REVIEW_QUEUE_CONTENT_CLASS}
            resizableContent
          />
          <ContentSection
            label={agentLabel}
            content={item.output}
            mode="pretty"
            headerMeta={<TraceTime value={timestamp} />}
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

function TraceTime({ value }: { value: string }) {
  if (!value) return null;
  return (
    <time className="inline-flex items-center font-mono text-mono-sm text-faint-foreground">
      {value}
    </time>
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
