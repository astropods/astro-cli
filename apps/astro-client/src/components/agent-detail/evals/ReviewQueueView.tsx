import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
  type RefObject,
} from "react";
import type { LucideIcon } from "lucide-react";
import {
  Check,
  ChevronDown,
  ChevronUp,
  Frown,
  Meh,
  Minus,
  Smile,
  UserRound,
  X,
} from "lucide-react";
import { PillToggle } from "@/components/activity/PillToggle";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { StatusBadge } from "@/components/StatusBadge";
import { ContentSection } from "@/components/agent-detail/traces/detail/ContentSection";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { summarize } from "@/lib/content-parse";
import { formatTimeAgo } from "@/lib/time-format";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  useDatasetReviewQueue,
  usePostDatasetJudgment,
} from "@/api/queries/evals";
import type {
  DatasetJudgmentVerdict,
  EvalDatasetResponse,
  ReviewQueueItem,
  ReviewQueueSentiment,
} from "@/lib/api";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "./EvalTabCard";
import type { RawMode } from "./DatasetItemRow";
import { flyVerdictToGrade } from "./review-queue-motion";

const REVIEW_QUEUE_VERDICT_OPTIONS: Array<{
  verdict: DatasetJudgmentVerdict;
  label: string;
  shortcut: string;
  Icon: LucideIcon;
  iconClassName: string;
}> = [
  {
    verdict: "good",
    label: "Good",
    shortcut: "G",
    Icon: Check,
    iconClassName: "text-success",
  },
  {
    verdict: "bad",
    label: "Bad",
    shortcut: "B",
    Icon: X,
    iconClassName: "text-destructive",
  },
  {
    verdict: "unknown",
    label: "Neutral",
    shortcut: "N",
    Icon: Minus,
    iconClassName: "text-muted-foreground",
  },
];

const REVIEW_QUEUE_VERDICT_SHORTCUTS: Record<string, DatasetJudgmentVerdict> = {
  g: "good",
  b: "bad",
  n: "unknown",
};

const EMPTY_REVIEW_QUEUE_ITEMS: ReviewQueueItem[] = [];

export interface ReviewQueueViewProps {
  deploymentId: string;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl?: string;
  summary: EvalDatasetResponse;
  gradeTargetRef?: RefObject<HTMLDivElement | null>;
}

export function ReviewQueueView({
  deploymentId,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
  summary,
  gradeTargetRef,
}: ReviewQueueViewProps) {
  const { data, isLoading, isError } = useDatasetReviewQueue(deploymentId);
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [rawMode, setRawMode] = useState<RawMode>("pretty");
  const items = data?.items ?? EMPTY_REVIEW_QUEUE_ITEMS;
  const selectedItem =
    items.find((item) => item.trace_id === selectedId) ?? items[0] ?? null;
  const activeSelectedId = selectedItem?.trace_id ?? null;
  const { previousTraceId, nextTraceId } = getAdjacentTraceIds(
    items,
    activeSelectedId,
  );
  const postJudgment = usePostDatasetJudgment(deploymentId, {
    onSuccess: (_data, variables) => {
      setSelectedId((current) =>
        current === variables.traceId ? variables.nextTraceId ?? null : current,
      );
    },
  });
  const resolvedAgentAvatarUrl =
    avatarBust ?? agentAvatarUrl ?? getDeploymentAvatarUrl(deploymentId);
  const handleSelectTrace = (traceId: string) => {
    postJudgment.reset();
    setSelectedId(traceId);
  };
  const handleJudgeTrace = (
    traceId: string,
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => {
    const { previousTraceId, nextTraceId } = getAdjacentTraceIds(items, traceId);
    const nextSelectedTraceId = nextTraceId ?? previousTraceId;
    flyVerdictToGrade(
      trigger?.getBoundingClientRect() ?? null,
      gradeTargetRef?.current,
      verdict,
    );

    if (activeSelectedId === traceId) {
      setSelectedId((current) => current ?? traceId);
    }
    postJudgment.mutate({ traceId, verdict, nextTraceId: nextSelectedTraceId });
  };

  return (
    <EvalTabCard className="h-[calc(100dvh-20rem)] max-h-[720px] min-h-96">
      <EvalTabCardHeader label="Review queue" datasetName={summary.dataset_name}>
        <PillToggle<RawMode>
          layoutId="review-queue-raw-mode"
          value={rawMode}
          onChange={setRawMode}
          size="md"
          options={[
            { key: "pretty", label: "Pretty" },
            { key: "raw", label: "Raw" },
          ]}
        />
      </EvalTabCardHeader>
      <EvalTabCardBody>
        <aside className="flex w-[392px] flex-none flex-col overflow-y-auto border-r border-border bg-card dark:bg-surface">
          <ReviewQueueList
            items={items}
            selectedId={activeSelectedId}
            onSelect={handleSelectTrace}
            isLoading={isLoading}
            isError={isError}
          />
        </aside>

        <div className="flex min-w-0 flex-1 flex-col bg-background">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Spinner delay={300} />
            </div>
          ) : isError ? (
            <div className="flex flex-1 flex-col items-center justify-center p-12 text-center">
              <div className="text-heading-3 font-semibold text-foreground">
                Queue unavailable
              </div>
              <p className="mt-1.5 text-body-sm text-muted-foreground">
                Failed to load the trace review queue.
              </p>
            </div>
          ) : selectedItem ? (
            <ReviewQueueDetail
              item={selectedItem}
              account={account}
              agentName={agentName}
              agentLabel={agentLabel}
              agentAvatarUrl={resolvedAgentAvatarUrl}
              rawMode={rawMode}
              onJudge={handleJudgeTrace}
              isJudging={postJudgment.isPending}
              showJudgmentError={postJudgment.isError}
              queueSize={items.length}
              onPrevious={previousTraceId ? () => handleSelectTrace(previousTraceId) : undefined}
              onNext={nextTraceId ? () => handleSelectTrace(nextTraceId) : undefined}
            />
          ) : (
            <ReviewQueueDetailEmpty />
          )}
        </div>
      </EvalTabCardBody>
    </EvalTabCard>
  );
}

function getAdjacentTraceIds(
  items: ReviewQueueItem[],
  traceId: string | null,
) {
  const index = traceId
    ? items.findIndex((item) => item.trace_id === traceId)
    : -1;

  return {
    previousTraceId:
      index > 0 ? items[index - 1]?.trace_id ?? null : null,
    nextTraceId:
      index >= 0 && index < items.length - 1
        ? items[index + 1]?.trace_id ?? null
        : null,
  };
}

function getReviewQueueShortcutVerdict(event: KeyboardEvent) {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isEditableShortcutTarget(event.target)
  ) {
    return null;
  }

  return REVIEW_QUEUE_VERDICT_SHORTCUTS[event.key.toLowerCase()] ?? null;
}

function isEditableShortcutTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  return (
    target.isContentEditable ||
    Boolean(
      target.closest(
        "input, textarea, select, [contenteditable='true'], [contenteditable='plaintext-only']",
      ),
    )
  );
}

interface ReviewQueueListProps {
  items: ReviewQueueItem[];
  selectedId: string | null;
  onSelect: (traceId: string) => void;
  isLoading: boolean;
  isError: boolean;
}

function ReviewQueueList({
  items,
  selectedId,
  onSelect,
  isLoading,
  isError,
}: ReviewQueueListProps) {
  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner delay={300} />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="px-6 py-12 text-center text-body-sm text-muted-foreground">
        Failed to load the queue.
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="px-6 py-12 text-center text-body-sm text-muted-foreground">
        No traces waiting for review.
      </div>
    );
  }

  return <ReviewQueueListBody items={items} selectedId={selectedId} onSelect={onSelect} />;
}

function ReviewQueueListBody({
  items,
  selectedId,
  onSelect,
}: {
  items: ReviewQueueItem[];
  selectedId: string | null;
  onSelect: (traceId: string) => void;
}) {
  // Freeze relative timestamps to the moment items arrive; otherwise every
  // selection re-render bumps the "12s ago" → "13s ago" for recent rows.
  const agoByTraceId = useMemo(() => {
    const map = new Map<string, string>();
    for (const item of items) {
      map.set(item.trace_id, item.timestamp ? formatTimeAgo(item.timestamp) : "");
    }
    return map;
  }, [items]);

  return (
    <ul className="flex flex-col">
      {items.map((item) => (
        <li key={item.trace_id}>
          <ReviewQueueRow
            item={item}
            ago={agoByTraceId.get(item.trace_id) ?? ""}
            selected={selectedId === item.trace_id}
            onSelect={() => onSelect(item.trace_id)}
          />
        </li>
      ))}
    </ul>
  );
}

const SENTIMENT_DOT: Record<ReviewQueueSentiment, string> = {
  positive: "bg-success",
  negative: "bg-destructive",
  "": "bg-muted-foreground/60",
};

const SENTIMENT_BADGE: Record<
  ReviewQueueSentiment,
  { label: string; color: "success" | "error" | "muted"; Icon: LucideIcon }
> = {
  positive: { label: "Likely positive", color: "success", Icon: Smile },
  negative: { label: "Likely negative", color: "error", Icon: Frown },
  "": { label: "No signal", color: "muted", Icon: Meh },
};

function ReviewQueueRow({
  item,
  ago,
  selected,
  onSelect,
}: {
  item: ReviewQueueItem;
  ago: string;
  selected: boolean;
  onSelect: () => void;
}) {
  const title = summarize(item.input);

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "flex min-h-16 w-full items-center gap-3 border-b border-l-2 border-border px-4 py-3.5 text-left transition-colors",
        selected
          ? "border-l-primary bg-primary/10"
          : "border-l-transparent hover:bg-muted/40",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "size-2 flex-none rounded-full",
          SENTIMENT_DOT[item.sentiment] ?? SENTIMENT_DOT[""],
        )}
      />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="truncate text-body text-foreground" title={title}>
          {title}
        </div>
        {ago && (
          <div className="truncate font-mono text-mono-sm text-muted-foreground">{ago}</div>
        )}
      </div>
    </button>
  );
}

function ReviewQueueDetail({
  item,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
  rawMode,
  onJudge,
  isJudging,
  showJudgmentError,
  queueSize,
  onPrevious,
  onNext,
}: {
  item: ReviewQueueItem;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl: string;
  rawMode: RawMode;
  onJudge: (
    traceId: string,
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => void;
  isJudging: boolean;
  showJudgmentError: boolean;
  queueSize: number;
  onPrevious?: () => void;
  onNext?: () => void;
}) {
  const sentiment = SENTIMENT_BADGE[item.sentiment] ?? SENTIMENT_BADGE[""];
  const traceLabel = truncateTraceId(item.trace_id);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-none items-center justify-between gap-4 border-b border-border px-6 py-4">
        <div className="flex min-w-0 items-center">
          <div className="flex min-w-0 flex-wrap items-center gap-3">
            <span
              className="font-mono text-mono-sm text-muted-foreground"
              title={item.trace_id}
            >
              {traceLabel}
            </span>
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
          </div>
        </div>
        <ReviewQueueDetailNavigation
          total={queueSize}
          onPrevious={onPrevious}
          onNext={onNext}
        />
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        <div className="mx-auto flex max-w-3xl flex-col gap-5">
          <ContentSection
            label="User"
            content={item.input}
            icon={<UserSectionIcon />}
            mode={rawMode}
          />
          <ContentSection
            label={agentLabel}
            content={item.output}
            mode={rawMode}
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

      <ReviewQueueVerdictFooter
        isPending={isJudging}
        showError={showJudgmentError}
        onSelect={(verdict, trigger) => onJudge(item.trace_id, verdict, trigger)}
      />
    </div>
  );
}

function ReviewQueueDetailNavigation({
  total,
  onPrevious,
  onNext,
}: {
  total: number;
  onPrevious?: () => void;
  onNext?: () => void;
}) {
  if (total <= 0) {
    return null;
  }

  return (
    <TooltipProvider delayDuration={300}>
      <div className="flex flex-none items-center gap-1">
        <ReviewQueueNavigationButton
          label="Previous"
          disabled={!onPrevious}
          onClick={onPrevious}
        >
          <ChevronUp className="size-5" />
        </ReviewQueueNavigationButton>
        <ReviewQueueNavigationButton
          label="Next"
          disabled={!onNext}
          onClick={onNext}
        >
          <ChevronDown className="size-5" />
        </ReviewQueueNavigationButton>
      </div>
    </TooltipProvider>
  );
}

function ReviewQueueNavigationButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled: boolean;
  onClick?: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
            className="size-[30px] text-muted-foreground"
          >
            {children}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function ReviewQueueVerdictFooter({
  isPending,
  showError,
  onSelect,
}: {
  isPending: boolean;
  showError: boolean;
  onSelect: (
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => void;
}) {
  const verdictButtonRefs = useRef<
    Record<DatasetJudgmentVerdict, HTMLButtonElement | null>
  >({
    good: null,
    bad: null,
    unknown: null,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const verdict = getReviewQueueShortcutVerdict(event);
      if (!verdict || isPending) {
        return;
      }

      event.preventDefault();
      onSelect(verdict, verdictButtonRefs.current[verdict]);
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isPending, onSelect]);

  return (
    <div className="flex flex-none flex-wrap items-center justify-between gap-3 border-t border-border px-6 py-4">
      <div className="min-w-0 flex-1 text-body-sm text-muted-foreground">
        {showError ? (
          "Could not save verdict. Try again."
        ) : (
          <>
            Select an option or use{" "}
            <ShortcutKey className="mx-0.5">G</ShortcutKey>
            {" / "}
            <ShortcutKey className="mx-0.5">B</ShortcutKey>
            {" / "}
            <ShortcutKey className="mx-0.5">N</ShortcutKey>.
          </>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {REVIEW_QUEUE_VERDICT_OPTIONS.map(
          ({ verdict, label, shortcut, Icon, iconClassName }) => (
            <Button
              key={verdict}
              ref={(node) => {
                verdictButtonRefs.current[verdict] = node;
              }}
              type="button"
              variant="outline"
              size="sm"
              disabled={isPending}
              onClick={(event: MouseEvent<HTMLButtonElement>) =>
                onSelect(verdict, event.currentTarget)
              }
            >
              <Icon className={cn("size-4", iconClassName)} />
              {label}
              <ShortcutKey ariaHidden>{shortcut}</ShortcutKey>
            </Button>
          ),
        )}
      </div>
    </div>
  );
}

function ShortcutKey({
  children,
  className,
  ariaHidden = false,
}: {
  children: ReactNode;
  className?: string;
  ariaHidden?: boolean;
}) {
  return (
    <kbd
      aria-hidden={ariaHidden || undefined}
      className={cn(
        "inline-flex h-5 min-w-5 items-center justify-center rounded border border-border-strong bg-muted/40 px-1.5 font-mono text-[11px] font-medium leading-none text-muted-foreground",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

function truncateTraceId(traceId: string) {
  return `trace_${traceId.replace(/^trace_/, "").slice(0, 6)}`;
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

function ReviewQueueDetailEmpty() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3.5 p-12 text-center">
      <span
        aria-hidden
        className="flex size-12 items-center justify-center rounded-full bg-success/10 text-success"
      >
        <Check className="size-6" />
      </span>
      <div>
        <div className="text-heading-3 font-semibold text-foreground">You're all caught up</div>
        <p className="mt-1.5 text-body-sm text-muted-foreground">Every trace has a verdict.</p>
      </div>
    </div>
  );
}
