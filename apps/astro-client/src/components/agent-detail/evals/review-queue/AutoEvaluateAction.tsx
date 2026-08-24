import { useState } from "react";
import { Gavel, Loader2, Sparkle } from "lucide-react";
import { usePostDatasetEvaluations } from "@/api/queries/evals";
import { Button } from "@/components/ui/button";
import { usePersistentCoachmark } from "@/hooks/use-persistent-coachmark";
import { useAuth } from "@/lib/auth";
import { AutoEvaluateHoverPopover } from "./AutoEvaluateHoverPopover";
import { AutoEvaluateOnboardingCoachmark } from "./AutoEvaluateOnboardingCoachmark";

const AUTO_EVALUATE_ONBOARDING_ID = "llm-judge";

export type AutoEvaluateState =
  | "loading"
  | "ready"
  | "evaluating"
  | "nothing-to-evaluate";

function EvaluatingGavel() {
  return (
    <span aria-hidden className="relative size-4 flex-none">
      <span className="dp-evaluating-gavel absolute inset-0">
        <Gavel className="size-4" />
      </span>
      <span className="absolute bottom-0 left-0.5 h-px w-3 rounded-full bg-current opacity-60" />
    </span>
  );
}

export function AutoEvaluateAction({
  deploymentId,
  account,
  state,
  evaluatingCount,
  onEvaluationStarted,
}: {
  deploymentId: string;
  account: string;
  state: AutoEvaluateState;
  evaluatingCount: number;
  onEvaluationStarted?: (predictionCount: number) => void;
}) {
  const [hoverOpen, setHoverOpen] = useState(false);
  const { user } = useAuth();
  const { dismissed: onboardingDismissed, dismiss: dismissOnboarding } =
    usePersistentCoachmark(AUTO_EVALUATE_ONBOARDING_ID, user?.id);
  const postPredictions = usePostDatasetEvaluations(deploymentId);
  const nothingToEvaluate = state === "nothing-to-evaluate";
  const autoEvaluateActionable =
    state === "ready" && !postPredictions.isPending;
  const showOnboarding = !onboardingDismissed && autoEvaluateActionable;
  const showHover =
    !postPredictions.isPending &&
    hoverOpen &&
    (nothingToEvaluate || (state === "ready" && onboardingDismissed));

  const handleRunEvaluation = () => {
    if (!autoEvaluateActionable) return;
    if (!onboardingDismissed) dismissOnboarding();
    postPredictions.mutate(undefined, {
      onSuccess: (response: { enqueued_trace_ids: string[] }) => {
        setHoverOpen(false);
        if (response.enqueued_trace_ids.length > 0) {
          onEvaluationStarted?.(response.enqueued_trace_ids.length);
        }
      },
    });
  };

  const trigger = (
    <span
      className="inline-flex flex-none"
      tabIndex={nothingToEvaluate ? 0 : undefined}
    >
      <Button
        type="button"
        size="sm"
        className="disabled:cursor-not-allowed"
        disabled={!autoEvaluateActionable}
        onClick={handleRunEvaluation}
      >
        {evaluatingCount > 0 ? (
          <EvaluatingGavel />
        ) : postPredictions.isPending ? (
          <Loader2 aria-hidden className="size-4 animate-spin" />
        ) : (
          <Sparkle aria-hidden className="size-4" />
        )}
        {evaluatingCount > 0 ? (
          <span>
            Evaluating {evaluatingCount}{" "}
            {evaluatingCount === 1 ? "item" : "items"}
          </span>
        ) : (
          <span className="@max-[420px]/review-card:sr-only">
            Run AI Evaluator
          </span>
        )}
      </Button>
    </span>
  );

  return (
    <AutoEvaluateOnboardingCoachmark
      open={showOnboarding}
      onDismiss={dismissOnboarding}
      anchor={
        <AutoEvaluateHoverPopover
          trigger={trigger}
          account={account}
          open={showHover}
          onOpenChange={setHoverOpen}
          nothingToEvaluate={nothingToEvaluate}
          unavailable={postPredictions.isError}
        />
      }
    />
  );
}
