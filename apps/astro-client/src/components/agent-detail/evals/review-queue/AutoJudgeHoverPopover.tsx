import type { ReactNode } from "react";
import { ArrowUpRight } from "lucide-react";
import { Link } from "react-router";
import { CoachmarkSurface } from "@/components/ui/coachmark";
import {
  Tooltip,
  TooltipContent,
  TooltipPositioner,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/lib/auth";
import { accountSettingsPath } from "@/lib/settings-paths";

const AUTO_JUDGE_CREDIT_ESTIMATE = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
}).format(500);

export function AutoJudgeHoverPopover({
  trigger,
  account,
  open,
  onOpenChange,
  nothingToJudge,
  unavailable,
}: {
  trigger: ReactNode;
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  nothingToJudge: boolean;
  unavailable: boolean;
}) {
  const { accounts } = useAuth();
  const billingPath = accountSettingsPath(accounts, account, "billing");

  return (
    <TooltipProvider delayDuration={0}>
      <Tooltip open={open} onOpenChange={onOpenChange}>
        <TooltipTrigger asChild>{trigger}</TooltipTrigger>
        {nothingToJudge ? (
          <TooltipContent
            side="bottom"
            align="start"
            sideOffset={8}
            className="max-w-xs"
          >
            <span>
              Every trace is already judged.
            </span>
          </TooltipContent>
        ) : (
          <TooltipPositioner
            side="bottom"
            align="start"
            sideOffset={8}
            className="w-[calc(100vw-2rem)] max-w-2xs"
          >
            <CoachmarkSurface className="rounded-2xl p-4 shadow-xl">
              <div className="flex flex-col gap-2">
                <h2 className="text-heading-4 font-semibold text-foreground">
                  Automatically judge traces
                </h2>
                <p className="text-body-sm text-muted-foreground">
                  The judge will score up to 50 of the most recent unjudged
                  traces. Use the results while deciding which traces belong in the dataset.
                </p>
              </div>

              <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
                <div className="text-body-sm font-semibold text-foreground">
                  Estimated ~{AUTO_JUDGE_CREDIT_ESTIMATE} credits
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
            </CoachmarkSurface>
          </TooltipPositioner>
        )}
      </Tooltip>
    </TooltipProvider>
  );
}
