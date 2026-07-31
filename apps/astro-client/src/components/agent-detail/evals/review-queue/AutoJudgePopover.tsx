import { useState } from "react";
import { ArrowUpRight, Gavel, Loader2, Sparkle } from "lucide-react";
import { Link } from "react-router";
import { usePostDatasetPredictions } from "@/api/queries/evals";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/lib/auth";
import { accountSettingsPath } from "@/lib/settings-paths";

const AUTO_JUDGE_CREDIT_ESTIMATE = 500;

function JudgingGavel() {
  return (
    <span aria-hidden className="relative size-4 flex-none">
      <span className="dp-judging-gavel absolute inset-0">
        <Gavel className="size-4" />
      </span>
      <span className="absolute bottom-0 left-0.5 h-px w-3 rounded-full bg-current opacity-60" />
    </span>
  );
}

export function AutoJudgePopover({
  deploymentId,
  account,
  disabled,
  judgingCount,
  onJudgingStarted,
}: {
  deploymentId: string;
  account: string;
  disabled: boolean;
  judgingCount: number;
  onJudgingStarted?: (predictionCount: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const { accounts } = useAuth();
  const postPredictions = usePostDatasetPredictions(deploymentId);
  const estimatedCredits = new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 2,
  }).format(AUTO_JUDGE_CREDIT_ESTIMATE);
  const billingPath = accountSettingsPath(accounts, account, "billing");
  const nothingToJudge =
    disabled && judgingCount === 0 && !postPredictions.isPending;

  const handleRunJudge = () => {
    if (postPredictions.isPending || disabled) return;
    postPredictions.mutate(undefined, {
      onSuccess: (response) => {
        setOpen(false);
        if (response.enqueued_trace_ids.length > 0) {
          onJudgingStarted?.(response.enqueued_trace_ids.length);
        }
      },
    });
  };

  const unavailable = postPredictions.isError;

  return (
    <TooltipProvider delayDuration={0}>
      <Tooltip open={open} onOpenChange={setOpen}>
        <TooltipTrigger asChild>
          <span
            className="inline-flex flex-none"
            tabIndex={nothingToJudge ? 0 : undefined}
          >
            <Button
              type="button"
              size="sm"
              className="disabled:cursor-not-allowed"
              disabled={postPredictions.isPending || disabled}
              onClick={handleRunJudge}
            >
              {judgingCount > 0 ? (
                <JudgingGavel />
              ) : postPredictions.isPending ? (
                <Loader2 aria-hidden className="size-4 animate-spin" />
              ) : (
                <Sparkle aria-hidden className="size-4" />
              )}
              {judgingCount > 0 ? (
                <span>
                  Judging {judgingCount} {judgingCount === 1 ? "item" : "items"}
                </span>
              ) : (
                <span className="@max-[420px]/review-card:sr-only">
                  Run AI Judge
                </span>
              )}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent
          side="bottom"
          align="start"
          sideOffset={8}
          className={
            nothingToJudge
              ? "max-w-xs"
              : "w-[calc(100vw-2rem)] max-w-xs rounded-xl border border-border bg-popover p-6 text-wrap text-popover-foreground shadow-md dark:bg-popover dark:text-popover-foreground"
          }
          showArrow={nothingToJudge}
        >
          {nothingToJudge ? (
            <span>
              Every trace already has a verdict, so there&apos;s nothing left to
              judge.
            </span>
          ) : (
            <>
              <div className="flex flex-col gap-2">
                <h2 className="text-heading-4 font-semibold text-foreground">
                  Automatically judge traces
                </h2>
                <p className="text-body-sm text-muted-foreground">
                  The judge will score up to 50 of the most recent unjudged traces.
                  You can confirm each verdict in the queue.
                </p>
              </div>

              <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
                <div className="text-body-sm font-semibold text-foreground">
                  Estimated ~{estimatedCredits} credits
                </div>
                <Link
                  to={billingPath}
                  className="inline-flex items-center gap-1.5 text-body-sm font-medium text-foreground-accent transition-colors hover:text-foreground"
                >
                  View usage
                  <ArrowUpRight aria-hidden className="size-3.5" />
                </Link>
              </div>

              {unavailable && (
                <p className="mt-5 text-body-sm text-destructive">
                  Could not enqueue traces. Try again.
                </p>
              )}
            </>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
