import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AutoEvaluateAction,
  type AutoEvaluateState,
} from "./AutoEvaluateAction";

const REVIEW_QUEUE_FILTER_OPTIONS = [
  { label: "All", value: "all" },
  { label: "Evaluated", value: "evaluated" },
  { label: "Not evaluated", value: "not_evaluated" },
] as const;

export type ReviewQueueFilterValue =
  (typeof REVIEW_QUEUE_FILTER_OPTIONS)[number]["value"];

export function ReviewQueueToolbar({
  deploymentId,
  account,
  autoEvaluateState,
  evaluatingCount,
  onEvaluationStarted,
  filter,
  onFilterChange,
}: {
  deploymentId: string;
  account: string;
  autoEvaluateState: AutoEvaluateState;
  evaluatingCount: number;
  onEvaluationStarted?: (predictionCount: number) => void;
  filter: ReviewQueueFilterValue;
  onFilterChange: (value: ReviewQueueFilterValue) => void;
}) {
  return (
    <div className="flex flex-none items-center justify-between gap-3 border-b border-border px-4 py-3">
      <AutoEvaluateAction
        deploymentId={deploymentId}
        account={account}
        state={autoEvaluateState}
        evaluatingCount={evaluatingCount}
        onEvaluationStarted={onEvaluationStarted}
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
