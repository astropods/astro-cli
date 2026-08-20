import { Card } from "@/components/ui/card";
import { InlineBadge } from "@/components/InlineBadge";
import { useBillingSpend } from "@/api/queries/billing";

const PLANS: Record<string, { label: string; detail: string }> = {
  unlimited: {
    label: "Unlimited",
    detail: "Internal account. Usage is metered and rated at zero, so nothing is ever owed.",
  },
  credit: {
    label: "Signup credit",
    detail: "Usage draws down the signup credit first, then bills to the card on file.",
  },
  no_credit: {
    label: "Pay as you go",
    detail: "The signup credit is already spent, so usage bills to the card on file.",
  },
};

function formatMoney(amount: number, currency?: string): string {
  const value = amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return currency?.toUpperCase().includes("USD") === false ? `${value} ${currency}` : `$${value}`;
}

export function PlanSummary({ account }: { account: string }) {
  const { data, isLoading } = useBillingSpend(account);

  const spend = data?.available ? data.data : undefined;
  if (isLoading || !spend) return null;

  const plan = spend.plan ? PLANS[spend.plan] : undefined;
  // An uncovered account cannot reach this: the spend endpoint reports it as an
  // error rather than as data.
  if (!plan) return null;
  const { label, detail } = plan;

  return (
    <Card className="mb-6 flex flex-col gap-2 p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-heading-4 text-foreground">Plan</h3>
        <div className="flex items-center gap-2">
          {spend.plan === "credit" && spend.has_credit && (
            <span className="text-body-sm text-muted-foreground">
              {formatMoney(spend.credit_remaining, spend.currency)} credit left
            </span>
          )}
          <InlineBadge variant="soft">{label}</InlineBadge>
        </div>
      </div>
      <p className="text-body-sm text-muted-foreground">{detail}</p>
    </Card>
  );
}
