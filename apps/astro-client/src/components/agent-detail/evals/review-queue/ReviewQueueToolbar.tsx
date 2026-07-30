import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ReactNode } from "react";
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
  filter,
  onFilterChange,
  children,
}: {
  deploymentId: string;
  account: string;
  autoJudgeDisabled: boolean;
  judgingCount: number;
  filter: ReviewQueueFilterValue;
  onFilterChange: (value: ReviewQueueFilterValue) => void;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-none flex-col border-b border-border bg-card dark:bg-surface @[760px]/review-card:flex-row">
      <div className="flex w-full flex-none items-center justify-between gap-3 border-b border-border px-4 py-3 @[760px]/review-card:w-[clamp(18rem,34%,24.5rem)] @[760px]/review-card:border-b-0 @[760px]/review-card:border-r">
        <AutoJudgePopover
          deploymentId={deploymentId}
          account={account}
          disabled={autoJudgeDisabled}
          judgingCount={judgingCount}
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
      <div className="flex min-w-0 flex-1 items-center px-4 py-3 @[520px]/review-card:px-6">
        {children}
      </div>
    </div>
  );
}
