import { useState } from "react";
import { Sparkles } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { ProgressBar, type ProgressBarTone } from "@/components/ui/progress-bar";
import { ActionPanel } from "@/components/ui/status-panel";
import { useBillingSpend, useBillingStatus } from "@/api/queries/billing";
import type { BillingSpend } from "@/lib/api";
import { EmptyState, LoadError, Unavailable } from "@/components/settings/SettingsShared";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Skeleton } from "@/components/ui/skeleton";
import { ManageLimitsDialog } from "@/components/settings/ManageLimitsDialog";
import { formatMoney, thresholdDollars } from "@/lib/billing-balances";
import { DEFAULT_CURRENCY } from "@/lib/billing-provider";
import { formatShortDate } from "@/lib/date-utils";
import { canManageAccountBilling } from "@/lib/billing-copy";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

// Recedes from the card; surface sits below card in dark, so it reads the same.
const SPEND_BAND_TINT = "bg-[#FCFCFD] dark:bg-surface";

function NoCardBanner({ message, onAddPayment }: { message: string; onAddPayment: () => void }) {
  return (
    <ActionPanel
      tone="warning"
      title="No payment method on file"
      primaryLabel="Add payment method"
      onPrimary={onAddPayment}
    >
      {message}
    </ActionPanel>
  );
}

function CardSkeleton() {
  return (
    <Card className="flex flex-col gap-4 p-5">
      <div className="flex items-center justify-between gap-3">
        <Skeleton className="h-5 w-32" />
        <Skeleton className="h-8 w-28" />
      </div>
      <Skeleton className="h-4 w-28" />
      <Skeleton className="h-9 w-40" />
      <Skeleton className="h-1.5 w-full" />
      <Skeleton className="h-9 w-full" />
    </Card>
  );
}

// Disabled instead of dead-ending: raising the limit needs a card and
// permission, so a live link would open a dialog that can't act.
function ResumeLink({
  enabled,
  disabledReason,
  onOpen,
}: {
  enabled: boolean;
  disabledReason: string;
  onOpen: () => void;
}) {
  const link = (
    <button
      type="button"
      disabled={!enabled}
      className={cn(
        "underline hover:no-underline",
        !enabled && "cursor-not-allowed decoration-dotted opacity-70 hover:underline",
      )}
      onClick={onOpen}
    >
      Increase limit to resume now
    </button>
  );

  if (enabled) return link;
  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span>{link}</span>
        </TooltipTrigger>
        <TooltipContent>{disabledReason}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

interface CreditPosition {
  granted: number;
  applied: number;
  remaining: number;
}

interface LimitState {
  spendLimitReached: boolean;
  limitReached: boolean;
  warningPassed: boolean;
  limitAmount?: number;
}

// Granted is not reported; what is left plus what was drawn reconstructs it.
function creditPosition(spend: BillingSpend, usageSpend: number, netSpend: number): CreditPosition {
  const applied = Math.max(usageSpend - netSpend, 0);
  const remaining = spend.has_credit ? Math.max(spend.credit_remaining, 0) : 0;
  return { granted: remaining + applied, applied, remaining };
}

function limitState(spend: BillingSpend): LimitState {
  const spendLimitReached = !!spend.limit?.in_alarm;
  // Per-metric caps aren't self-serve, but a provider- or admin-set one can
  // still pause the account; the bar only tracks spend, so this iterates every
  // metric rather than naming the two that exist today, and still catches a
  // pause the bar itself wouldn't show if a third one ever ships.
  const usageLimitReached = Object.values(spend.usage ?? {}).some((u) => u.limit?.in_alarm);
  return {
    spendLimitReached,
    limitReached: spendLimitReached || usageLimitReached,
    warningPassed: !!spend.warning?.in_alarm && !spendLimitReached,
    limitAmount: thresholdDollars(spend.limit?.amount),
  };
}

// Untouched credit, partly spent, and exhausted are three different messages.
function spendBadge(
  limits: LimitState,
  credit: CreditPosition,
  currency: string,
): { label: string; color: StatusBadgeColor } | null {
  if (limits.limitReached) return { label: "Agents paused", color: "destructive" };
  if (credit.granted <= 0) return null;
  if (credit.remaining > 0 && credit.applied === 0) {
    return { label: `${formatMoney(credit.granted, currency)} free credit ready`, color: "success" };
  }
  if (credit.remaining > 0) {
    return { label: `${formatMoney(credit.remaining, currency)} free credit left`, color: "success" };
  }
  return { label: `${formatMoney(credit.applied, currency)} credit applied`, color: "success" };
}

function meterTone(limits: LimitState): ProgressBarTone {
  if (limits.limitReached) return "destructive";
  if (limits.warningPassed) return "warning";
  return limits.limitAmount != null ? "primary" : "muted";
}

// About the card only. A limit being reached is a separate notice.
function noCardMessage(hasCard: boolean, creditRemaining: number, currency: string): string | null {
  if (hasCard) return null;
  if (creditRemaining > 0) {
    return `Your agents will be paused when your ${formatMoney(creditRemaining, currency)} of free credit runs out.`;
  }
  return "Your agents are paused. Add a payment method to keep them running.";
}

function resumeDisabledReason(canManage: boolean): string {
  return canManage
    ? "Add a payment method to increase your spend limit."
    : "Only owners and admins can change limits.";
}

// Spend/limit/credit summary from useBillingSpend. No per-credit ledger
// (see the old Credits tab); this answers "can agents keep running".
export function PayAsYouGoCard({
  account,
  onAddPayment,
  onViewInvoices,
}: {
  account: string;
  onAddPayment: () => void;
  onViewInvoices: () => void;
}) {
  const { data, isLoading, isLoadingError, refetch } = useBillingSpend(account);
  const { data: status, isLoadingError: statusError, refetch: refetchStatus } = useBillingStatus(account);
  const { role, personalAccount } = useAuth();
  const canManage = canManageAccountBilling(account, personalAccount?.name, role);
  const [limitsOpen, setLimitsOpen] = useState(false);

  if (isLoading) return <CardSkeleton />;
  // isLoadingError, not isError: a background refetch failure keeps existing
  // data and shouldn't tear down a card that's already showing real numbers.
  if (isLoadingError) return <LoadError onRetry={() => refetch()} />;
  if (!data?.available || !data.data) return <Unavailable />;

  const spend = data.data;
  if (!spend.plan) return <EmptyState message="No plan on this account yet." />;

  if (spend.plan === "unlimited") {
    return (
      <Card className="flex flex-wrap items-center justify-between gap-3 p-5">
        <div className="flex items-center gap-2">
          <h3 className="text-heading-4 text-foreground">Pay-as-you-go</h3>
          <StatusBadge color="primary" size="sm" className="gap-1">
            <Sparkles className="size-3" aria-hidden />
            Unlimited
          </StatusBadge>
        </div>
        <p className="text-body-sm text-muted-foreground">Usage is rated at zero, so nothing is ever owed.</p>
      </Card>
    );
  }

  const currency = spend.currency ?? DEFAULT_CURRENCY;
  const usageSpend = spend.has_usage_spend ? spend.usage_spend : 0;
  const netSpend = spend.has_current_spend ? spend.current_spend : usageSpend;
  // Defaults true until status resolves, so a loading/erroring query can't
  // flash the paused banner for an account that has a card. A confirmed
  // isLoadingError gets its own retry notice below instead of this default.
  const hasCard = status ? !!status.has_payment_method : true;

  const credit = creditPosition(spend, usageSpend, netSpend);
  const limits = limitState(spend);
  const { limitAmount, limitReached, spendLimitReached, warningPassed } = limits;
  const warningAmount = thresholdDollars(spend.warning?.amount);
  const badge = spendBadge(limits, credit, currency);
  const barTone = meterTone(limits);
  const cardMessage = noCardMessage(hasCard, credit.remaining, currency);
  const periodEnd = spend.current_period_end;

  return (
    <div className="flex flex-col gap-3">
      {statusError && (
        <ActionPanel
          tone="warning"
          title="Couldn't check your payment method"
          primaryLabel="Retry"
          onPrimary={() => refetchStatus()}
        >
          We can't confirm whether a card is on file, so the paused-agent notice below may not be accurate.
        </ActionPanel>
      )}
      {cardMessage && <NoCardBanner message={cardMessage} onAddPayment={onAddPayment} />}

      <Card className="flex flex-col overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
          <h3 className="text-heading-4 text-foreground">Pay-as-you-go</h3>
          <Button size="sm" variant="outline" onClick={() => setLimitsOpen(true)}>
            Manage limits
          </Button>
        </div>

        <div className={cn("flex flex-col gap-2 px-5 py-4", SPEND_BAND_TINT)}>
          <p className="text-body-sm text-muted-foreground">Spend this period</p>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-heading-2 tabular-nums text-foreground">
              {formatMoney(usageSpend, currency)}
            </span>
            {badge && (
              <StatusBadge color={badge.color} size="sm">
                {badge.label}
              </StatusBadge>
            )}
          </div>

          {limitAmount != null ? (
            // Stops short of the card edge so the limit ends the meter.
            <div className="flex items-center gap-3">
              <ProgressBar
                aria-label="Spend this period"
                className="w-[55%] max-w-[55%]"
                value={usageSpend}
                max={limitAmount}
                tone={barTone}
              />
              <span className="shrink-0 text-body-sm text-muted-foreground tabular-nums">
                {formatMoney(limitAmount, currency)} spend limit
              </span>
            </div>
          ) : (
            // Saying "no spend limit" while also saying agents are paused
            // below reads as contradictory when a per-metric cap (not
            // shown here) is what's actually pausing the account.
            !limitReached && (
              <p className="text-body-sm text-muted-foreground">
                No spend limit set. Manage limits to cap what this account can be billed.
              </p>
            )
          )}

          {warningPassed && warningAmount != null && (
            <p className="text-body-sm text-warning">
              Alert threshold of {formatMoney(warningAmount, currency)} passed.
            </p>
          )}
          {limitReached && (
            <p className="text-body-sm text-destructive">
              {spendLimitReached ? "Spend limit" : "Usage limit"} reached and agents will resume{" "}
              {periodEnd ? `on ${formatShortDate(periodEnd)}` : "when the billing period resets"}.
              {/* Manage limits only edits the spend limit, so the resume link only
                  helps when spend is what's gating the account: raising it can't
                  lift a cap the dialog doesn't expose or write. */}
              {spendLimitReached && (
                <>
                  {" "}
                  <ResumeLink
                    enabled={hasCard && canManage}
                    disabledReason={resumeDisabledReason(canManage)}
                    onOpen={() => setLimitsOpen(true)}
                  />
                </>
              )}
            </p>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border/60 px-5 py-4">
          <div className="flex flex-col gap-1 text-body-sm tabular-nums">
            {credit.applied > 0 && (
              <div className="flex items-baseline justify-between gap-6 text-muted-foreground">
                <span>Credit applied</span>
                <span>−{formatMoney(credit.applied, currency)}</span>
              </div>
            )}
            <div className="flex items-baseline justify-between gap-6">
              <span className="font-medium text-foreground">Upcoming invoice</span>
              <span className="text-foreground">
                <span className="font-medium">{formatMoney(netSpend, currency)}</span>
                {periodEnd ? ` on ${formatShortDate(periodEnd)}` : ""}
              </span>
            </div>
          </div>
          {spend.has_last_invoice && (
            <Button size="sm" variant="outline" onClick={onViewInvoices}>
              View invoice
            </Button>
          )}
        </div>
      </Card>

      <ManageLimitsDialog account={account} open={limitsOpen} onOpenChange={setLimitsOpen} />
    </div>
  );
}
