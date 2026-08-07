import { useState } from "react";
import { Gavel, Loader2, Sparkle } from "lucide-react";
import { usePostDatasetPredictions } from "@/api/queries/evals";
import { Button } from "@/components/ui/button";
import { usePersistentCoachmark } from "@/hooks/use-persistent-coachmark";
import { useAuth } from "@/lib/auth";
import { AutoJudgeHoverPopover } from "./AutoJudgeHoverPopover";
import { AutoJudgeOnboardingCoachmark } from "./AutoJudgeOnboardingCoachmark";

const AUTO_JUDGE_ONBOARDING_ID = "llm-judge";

export type AutoJudgeState =
  | "loading"
  | "ready"
  | "judging"
  | "nothing-to-judge";

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

export function AutoJudgeAction({
  deploymentId,
  account,
  state,
  judgingCount,
  onJudgingStarted,
}: {
  deploymentId: string;
  account: string;
  state: AutoJudgeState;
  judgingCount: number;
  onJudgingStarted?: (predictionCount: number) => void;
}) {
  const [hoverOpen, setHoverOpen] = useState(false);
  const { user } = useAuth();
  const { dismissed: onboardingDismissed, dismiss: dismissOnboarding } =
    usePersistentCoachmark(AUTO_JUDGE_ONBOARDING_ID, user?.id);
  const postPredictions = usePostDatasetPredictions(deploymentId);
  const nothingToJudge = state === "nothing-to-judge";
  const autoJudgeActionable =
    state === "ready" && !postPredictions.isPending;
  const showOnboarding = !onboardingDismissed && autoJudgeActionable;
  const showHover =
    !postPredictions.isPending &&
    hoverOpen &&
    (nothingToJudge || (state === "ready" && onboardingDismissed));

  const handleRunJudge = () => {
    if (!autoJudgeActionable) return;
    if (!onboardingDismissed) dismissOnboarding();
    postPredictions.mutate(undefined, {
      onSuccess: (response) => {
        setHoverOpen(false);
        if (response.enqueued_trace_ids.length > 0) {
          onJudgingStarted?.(response.enqueued_trace_ids.length);
        }
      },
    });
  };

  const trigger = (
    <span
      className="inline-flex flex-none"
      tabIndex={nothingToJudge ? 0 : undefined}
    >
      <Button
        type="button"
        size="sm"
        className="disabled:cursor-not-allowed"
        disabled={!autoJudgeActionable}
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
          <span className="@max-[420px]/review-card:sr-only">Run AI Judge</span>
        )}
      </Button>
    </span>
  );

  return (
    <AutoJudgeOnboardingCoachmark
      open={showOnboarding}
      onDismiss={dismissOnboarding}
      anchor={
        <AutoJudgeHoverPopover
          trigger={trigger}
          account={account}
          open={showHover}
          onOpenChange={setHoverOpen}
          nothingToJudge={nothingToJudge}
          unavailable={postPredictions.isError}
        />
      }
    />
  );
}
