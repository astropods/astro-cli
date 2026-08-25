import { useState } from "react";
import { Sparkles } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { InlineBadge } from "@/components/InlineBadge";
import { ProgressBar, type ProgressBarTone } from "@/components/ui/progress-bar";
import { ActionPanel } from "@/components/ui/status-panel";
import { useBillingSpend, useBillingStatus } from "@/api/queries/billing";
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
import { formatShortDate } from "@/lib/date-utils";
import { canManageAccountBilling } from "@/lib/billing-copy";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";

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
          <InlineBadge variant="soft" className="gap-1">
            <Sparkles className="size-3" aria-hidden />
            Unlimited
          </InlineBadge>
        </div>
        <p className="text-body-sm text-muted-foreground">Usage is rated at zero, so nothing is ever owed.</p>
      </Card>
    );
  }

  const currency = spend.currency ?? "USD";
  const usageSpend = spend.has_usage_spend ? spend.usage_spend : 0;
  const netSpend = spend.has_current_spend ? spend.current_spend : usageSpend;
  // Gap between usage and net spend is what credit covered this period.
  const creditApplied = Math.max(usageSpend - netSpend, 0);
  const creditRemaining = spend.has_credit ? Math.max(spend.credit_remaining, 0) : 0;
  const creditGranted = creditRemaining + creditApplied;
  // Defaults true until status resolves, so a loading/erroring query can't
  // flash the paused banner for an account that has a card. A confirmed
  // isLoadingError gets its own retry notice below instead of this default.
  const hasCard = status ? !!status.has_payment_method : true;

  const limitAmount = thresholdDollars(spend.limit?.amount);
  const warningAmount = thresholdDollars(spend.warning?.amount);
  const spendLimitReached = !!spend.limit?.in_alarm;
  // Per-metric caps aren't self-serve, but a provider- or admin-set one can
  // still pause the account; the bar only tracks spend, so this iterates
  // every metric rather than naming the two that exist today, and still
  // catches a pause the bar itself wouldn't show if a third one ever ships.
  const usageLimitReached = Object.values(spend.usage ?? {}).some((u) => u.limit?.in_alarm);
  const limitReached = spendLimitReached || usageLimitReached;
  const warningPassed = !!spend.warning?.in_alarm && !spendLimitReached;

  let badge: { label: string; tone: "success" | "destructive" } | null = null;
  if (limitReached) {
    badge = { label: "Agents paused", tone: "destructive" };
  } else if (creditGranted > 0) {
    if (creditRemaining > 0 && creditApplied === 0) {
      badge = { label: `${formatMoney(creditGranted, currency)} free credit ready`, tone: "success" };
    } else if (creditRemaining > 0) {
      badge = { label: `${formatMoney(creditRemaining, currency)} free credit left`, tone: "success" };
    } else {
      badge = { label: `${formatMoney(creditApplied, currency)} credit applied`, tone: "success" };
    }
  }

  const barTone: ProgressBarTone = limitReached
    ? "destructive"
    : warningPassed
      ? "warning"
      : limitAmount != null
        ? "primary"
        : "muted";

  const noCardMessage = !hasCard
    ? creditRemaining > 0
      ? `Your agents will be paused when your ${formatMoney(creditRemaining, currency)} of free credit runs out.`
      : "Your agents are paused. Add a payment method to keep them running."
    : null;

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
      {noCardMessage && <NoCardBanner message={noCardMessage} onAddPayment={onAddPayment} />}

      <Card className="flex flex-col gap-4 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-heading-4 text-foreground">Pay-as-you-go</h3>
          <Button size="sm" variant="outline" onClick={() => setLimitsOpen(true)}>
            Manage limits
          </Button>
        </div>

        <div className="flex flex-col gap-2">
          <p className="text-body-sm text-muted-foreground">Usage this period</p>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-heading-2 tabular-nums text-foreground">
              {formatMoney(usageSpend, currency)}
            </span>
            {badge && (
              <InlineBadge
                variant="soft"
                className={cn(
                  "gap-1",
                  badge.tone === "success"
                    ? "bg-success/10 text-success"
                    : "bg-destructive/10 text-destructive",
                )}
              >
                {badge.label}
              </InlineBadge>
            )}
          </div>

          {limitAmount != null ? (
            <div className="flex items-center gap-3">
              <ProgressBar
                aria-label="Usage this period"
                className="flex-1"
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
                    disabledReason={
                      !canManage
                        ? "Only owners and admins can change limits."
                        : "Add a payment method to increase your spend limit."
                    }
                    onOpen={() => setLimitsOpen(true)}
                  />
                </>
              )}
            </p>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border/60 pt-4">
          <div className="flex flex-col gap-1 text-body-sm tabular-nums">
            {creditApplied > 0 && (
              <div className="flex items-baseline justify-between gap-6 text-muted-foreground">
                <span>Credit applied</span>
                <span>−{formatMoney(creditApplied, currency)}</span>
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
              View invoices
            </Button>
          )}
        </div>
      </Card>

      <ManageLimitsDialog account={account} open={limitsOpen} onOpenChange={setLimitsOpen} />
    </div>
  );
}
