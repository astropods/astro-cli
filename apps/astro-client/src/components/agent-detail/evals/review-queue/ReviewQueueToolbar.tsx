import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AutoJudgePopover } from "./AutoJudgePopover";

const REVIEW_QUEUE_FILTER_OPTIONS = [
  { label: "All verdicts", value: "all" },
  { label: "Good", value: "good" },
  { label: "Bad", value: "bad" },
  { label: "Not sure", value: "unknown" },
  { label: "Not judged", value: "none" },
] as const;

export type ReviewQueueFilterValue =
  (typeof REVIEW_QUEUE_FILTER_OPTIONS)[number]["value"];

export function ReviewQueueToolbar({
  deploymentId,
  account,
  autoJudgeDisabled,
  judgingCount,
  onJudgingStarted,
  filter,
  onFilterChange,
}: {
  deploymentId: string;
  account: string;
  autoJudgeDisabled: boolean;
  judgingCount: number;
  onJudgingStarted?: (predictionCount: number) => void;
  filter: ReviewQueueFilterValue;
  onFilterChange: (value: ReviewQueueFilterValue) => void;
}) {
  return (
    <div className="flex flex-none items-center justify-between gap-3 border-b border-border px-4 py-3">
      <AutoJudgePopover
        deploymentId={deploymentId}
        account={account}
        disabled={autoJudgeDisabled}
        judgingCount={judgingCount}
        onJudgingStarted={onJudgingStarted}
      />
      <Select
        value={filter}
        onValueChange={(value) =>
          onFilterChange(value as ReviewQueueFilterValue)
        }
      >
        <SelectTrigger
          aria-label="Filter review queue"
          className="h-7 w-32 bg-background px-2.5 text-body-sm [&_svg]:size-3.5"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {REVIEW_QUEUE_FILTER_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
