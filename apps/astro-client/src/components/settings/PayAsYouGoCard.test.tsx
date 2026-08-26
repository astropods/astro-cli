import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { BillingSpend } from "@/lib/api";
import { buildSpendResponse } from "@/api/queries/billing.fixtures";
import { PayAsYouGoCard } from "./PayAsYouGoCard";

const mockSpend = vi.fn();
const mockStatus = vi.fn();
const mockRole = vi.fn<() => string | null>(() => "owner");
const mockPersonalAccount = vi.fn<() => { name: string } | undefined>(() => undefined);

vi.mock("@/lib/auth", () => ({ useAuth: () => ({ role: mockRole(), personalAccount: mockPersonalAccount() }) }));
vi.mock("@/api/queries/billing", () => ({
  useBillingSpend: (account: string) => mockSpend(account),
  useBillingStatus: (account: string) => mockStatus(account),
  useSetBillingSpendThresholds: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

function billingStatus(partial: { has_payment_method?: boolean } = {}) {
  return { data: { has_payment_method: false, gated: false, ...partial } };
}

function spendResponse(partial: Partial<BillingSpend> = {}) {
  return {
    data: buildSpendResponse({ plan: "credit", current_period_end: futureISO(2), ...partial }),
    isLoading: false,
  };
}

const onAddPayment = vi.fn();
const onViewInvoices = vi.fn();

function renderCard() {
  return render(
    <PayAsYouGoCard
      account="acme"
      onAddPayment={onAddPayment}
      onViewInvoices={onViewInvoices}
    />,
  );
}

/** The chip's own element, so a test can assert its tone as well as its copy. */
function creditChip(): HTMLElement | null {
  return screen.queryByText(/free credit|credit applied|Agents paused/)?.closest("span") ?? null;
}

/** The filled portion of the bar carries the tone class; the track never does. */
function barIndicator(): Element {
  const bar = screen.getByRole("progressbar");
  if (!bar.firstElementChild) throw new Error("progress bar has no indicator");
  return bar.firstElementChild;
}

beforeEach(() => {
  onAddPayment.mockReset();
  onViewInvoices.mockReset();
  mockSpend.mockReset();
  mockStatus.mockReset().mockReturnValue(billingStatus({ has_payment_method: true }));
  mockRole.mockReset().mockReturnValue("owner");
  mockPersonalAccount.mockReset().mockReturnValue(undefined);
});

describe("PayAsYouGoCard free credit chip", () => {
  it("shows the full grant as ready when nothing has been spent", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 0, credit_remaining: 10, has_credit: true }),
    );
    renderCard();

    // Usage this period and the upcoming invoice are both zero.
    expect(screen.getAllByText("$0.00")).toHaveLength(2);
    expect(screen.getByText("$10.00 free credit ready")).toBeInTheDocument();
  });

  it("shows what's left once some credit has been drawn down", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.getByText("$6.40")).toBeInTheDocument();
    expect(screen.getByText("$3.60 free credit left")).toBeInTheDocument();
  });

  it("tracks the remaining grant down as usage consumes it", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 9.5, credit_remaining: 0.5, has_credit: true }),
    );
    const { rerender } = renderCard();
    expect(screen.getByText("$0.50 free credit left")).toBeInTheDocument();

    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 2, credit_remaining: 8, has_credit: true }),
    );
    rerender(
      <PayAsYouGoCard account="acme" onAddPayment={onAddPayment} onViewInvoices={onViewInvoices} />,
    );
    expect(screen.getByText("$8.00 free credit left")).toBeInTheDocument();
  });

  it("shows the applied amount once the credit is gone and billing has started", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 45.4, usage_spend: 55.4, credit_remaining: 0, has_credit: true }),
    );
    renderCard();

    expect(screen.getByText("$55.40")).toBeInTheDocument();
    expect(screen.getByText("$10.00 credit applied")).toBeInTheDocument();
    // The invoice line spells out the arithmetic: usage, credit, then total.
    expect(screen.getByText("−$10.00")).toBeInTheDocument();
    expect(screen.getByText("Upcoming invoice")).toBeInTheDocument();
    expect(screen.getByText("$45.40")).toBeInTheDocument();
    expect(screen.getByText(/^on /)).toBeInTheDocument();
  });

  it("puts the date after the amount, not the label", () => {
    mockSpend.mockReturnValue(spendResponse({ current_spend: 45.4, usage_spend: 62.1 }));
    renderCard();

    const row = screen.getByText("Upcoming invoice").closest("div")!;
    expect(row).toHaveTextContent(/Upcoming invoice\s*\$45\.40\s*on/);
  });

  it("tints the credit chip with the success token, not a raw color", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 0, credit_remaining: 10, has_credit: true }),
    );
    renderCard();

    expect(creditChip()!.getAttribute("style")).toContain("var(--success)");
  });

  it("drops the chip entirely when the account never had credit", () => {
    mockSpend.mockReturnValue(spendResponse({ current_spend: 45.02, usage_spend: 45.02 }));
    renderCard();

    // Nothing was covered by credit, so usage and the invoice agree.
    expect(screen.getAllByText("$45.02")).toHaveLength(2);
    expect(creditChip()).toBeNull();
  });
});

describe("PayAsYouGoCard alert threshold", () => {
  it("turns the bar warning and names the threshold that was passed", () => {
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 52,
        usage_spend: 52,
        warning: { amount: 5000, in_alarm: true },
        limit: { amount: 10000, in_alarm: false },
      }),
    );
    renderCard();

    expect(screen.getByText(/Alert threshold of \$50\.00 passed\./)).toBeInTheDocument();
    expect(barIndicator()).toHaveClass("bg-warning");
  });

  it("leaves the bar at its default tone before the threshold is reached", () => {
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 20,
        usage_spend: 20,
        warning: { amount: 5000, in_alarm: false },
        limit: { amount: 10000, in_alarm: false },
      }),
    );
    renderCard();

    expect(screen.queryByText(/Alert threshold/)).not.toBeInTheDocument();
    expect(barIndicator()).toHaveClass("bg-primary");
  });
});

describe("PayAsYouGoCard spend limit reached", () => {
  beforeEach(() => {
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 60,
        usage_spend: 60,
        current_period_end: "2026-08-23T12:00:00Z",
        limit: { amount: 6000, in_alarm: true },
      }),
    );
  });

  it("turns the bar destructive and says when agents resume", () => {
    renderCard();

    expect(screen.getByText("Agents paused")).toBeInTheDocument();
    expect(screen.getByText(/Spend limit reached and agents will resume on Aug 23, 2026/)).toBeInTheDocument();
    expect(screen.getByText(/\$60\.00 spend limit/)).toBeInTheDocument();
    expect(barIndicator()).toHaveClass("bg-destructive");
  });

  it("tints the paused chip with the destructive token", () => {
    renderCard();
    expect(creditChip()!.getAttribute("style")).toContain("var(--destructive)");
  });

  it("opens the limits dialog from the resume link", async () => {
    renderCard();
    await userEvent.click(screen.getByRole("button", { name: "Increase limit to resume now" }));

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("Spend limit")).toBeInTheDocument();
  });
});

describe("PayAsYouGoCard when a limit not shown in the UI pauses the account", () => {
  // Compute/AI Gateway caps aren't self-serve here, but the account can
  // still hold one from before this change (or set outside the UI), and it
  // independently pauses the account. The bar only tracks spend, so this is
  // the only thing that would otherwise miss the pause entirely. The message
  // stays generic ("Usage limit"), not naming Compute or AI Gateway, since
  // this dialog never exposes those as a concept.
  it("shows the paused badge, even with no spend limit set", () => {
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 12.5,
        usage_spend: 12.5,
        current_period_end: "2026-08-23T12:00:00Z",
        usage: {
          compute: { unit: "CU-hours", limit: { amount: 500, in_alarm: true } },
        },
      }),
    );
    renderCard();

    expect(screen.getByText("Agents paused")).toBeInTheDocument();
    expect(screen.getByText(/Usage limit reached and agents will resume on Aug 23, 2026/)).toBeInTheDocument();
    expect(screen.queryByText(/Compute/)).not.toBeInTheDocument();
    // Manage limits only edits spend, so it can't lift a per-metric cap: a
    // resume link here would raise the wrong number and dead-end the account.
    expect(screen.queryByRole("button", { name: "Increase limit to resume now" })).not.toBeInTheDocument();
    // "No spend limit set" next to "reached and agents will resume" would
    // read as contradictory: the pause message alone is the clear signal.
    expect(screen.queryByText(/No spend limit set/)).not.toBeInTheDocument();
  });

  it("catches a pause on a metric this UI has never heard of", () => {
    // spend.usage is a Record<string, UsageThresholds>; a metric added on the
    // provider side tomorrow must still surface here without a client change.
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 12.5,
        usage_spend: 12.5,
        current_period_end: "2026-08-23T12:00:00Z",
        usage: {
          storage: { unit: "GB-hours", limit: { amount: 500, in_alarm: true } },
        },
      }),
    );
    renderCard();

    expect(screen.getByText("Agents paused")).toBeInTheDocument();
    expect(screen.getByText(/Usage limit reached and agents will resume on Aug 23, 2026/)).toBeInTheDocument();
  });

  it("falls back to a dateless message when the period end is missing", () => {
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 60,
        usage_spend: 60,
        current_period_end: undefined,
        limit: { amount: 6000, in_alarm: true },
      }),
    );
    renderCard();

    expect(
      screen.getByText(/Spend limit reached and agents will resume when the billing period resets/),
    ).toBeInTheDocument();
  });
});

describe("PayAsYouGoCard without a spend limit", () => {
  it("renders no bar at all, since a bar with no cap reads as a full one", () => {
    mockSpend.mockReturnValue(spendResponse({ current_spend: 12.5, usage_spend: 12.5 }));
    renderCard();

    expect(screen.getByText(/No spend limit set\./)).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard without a payment method", () => {
  it("warns above the card, naming what is left before agents pause", () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.getByText("No payment method on file")).toBeInTheDocument();
    expect(screen.getByText(/will be paused when your \$3\.60 of free credit runs out/)).toBeInTheDocument();
  });

  it("routes the banner button to the payment section", async () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();
    await userEvent.click(screen.getByRole("button", { name: "Add payment method" }));

    expect(onAddPayment).toHaveBeenCalledTimes(1);
  });

  it("says agents are already paused once there is nothing left to fall back on", () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(spendResponse({ current_spend: 0, usage_spend: 0 }));
    renderCard();

    expect(screen.getByText(/Your agents are paused\. Add a payment method/)).toBeInTheDocument();
  });

  it("keeps the banner up when the spend limit is also stopping agents", () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 60, usage_spend: 60, limit: { amount: 6000, in_alarm: true } }),
    );
    renderCard();

    expect(screen.getByText("No payment method on file")).toBeInTheDocument();
    expect(screen.getByText(/Spend limit reached/)).toBeInTheDocument();
  });

  it("makes the resume link inert, since a higher limit bills nothing without a card", async () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 60, usage_spend: 60, limit: { amount: 6000, in_alarm: true } }),
    );
    renderCard();

    const resume = screen.getByRole("button", { name: "Increase limit to resume now" });
    expect(resume).toBeDisabled();

    await userEvent.click(resume);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("explains on hover why the resume link is inert", async () => {
    mockStatus.mockReturnValue(billingStatus({ has_payment_method: false }));
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 60, usage_spend: 60, limit: { amount: 6000, in_alarm: true } }),
    );
    renderCard();

    // The disabled button takes no pointer events, so the wrapper is the trigger.
    const trigger = screen.getByRole("button", { name: "Increase limit to resume now" }).parentElement!;
    await userEvent.hover(trigger);

    await waitFor(() =>
      expect(trigger).toHaveAccessibleDescription(
        "Add a payment method to increase your spend limit.",
      ),
    );
  });

  it("keeps the banner hidden once a card is on file", () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.queryByText("No payment method on file")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard invoices", () => {
  it("routes to the invoices section once one has been issued", async () => {
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 45.02, usage_spend: 45.02, has_last_invoice: true }),
    );
    renderCard();
    await userEvent.click(screen.getByRole("button", { name: "View invoice" }));

    expect(onViewInvoices).toHaveBeenCalledTimes(1);
  });

  it("offers no link before the first invoice, which would land on a draft", () => {
    mockSpend.mockReturnValue(spendResponse({ current_spend: 45.02, usage_spend: 45.02 }));
    renderCard();

    expect(screen.queryByRole("button", { name: /View invoice/ })).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard on the unlimited plan", () => {
  it("states the mode and nothing else", () => {
    mockSpend.mockReturnValue(spendResponse({ plan: "unlimited" }));
    renderCard();

    expect(screen.getByText("Unlimited")).toBeInTheDocument();
    expect(screen.queryByText("Manage limits")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard with no billing backend", () => {
  it("renders the unavailable state", () => {
    mockSpend.mockReturnValue({ data: { available: false }, isLoading: false });
    renderCard();
    expect(
      screen.getByText("Billing isn't available for this account yet. Data appears here once billing is enabled."),
    ).toBeInTheDocument();
  });
});

describe("PayAsYouGoCard when the spend query has never loaded successfully", () => {
  it("shows a retry state instead of the unavailable message", () => {
    mockSpend.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: vi.fn() });
    renderCard();

    expect(screen.getByText("Couldn't load this.")).toBeInTheDocument();
    expect(
      screen.queryByText("Billing isn't available for this account yet. Data appears here once billing is enabled."),
    ).not.toBeInTheDocument();
  });

  it("retries the query on click", async () => {
    const refetch = vi.fn();
    mockSpend.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch });
    renderCard();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

describe("PayAsYouGoCard when a background refetch fails after loading", () => {
  it("keeps rendering the last known-good data instead of an error banner", () => {
    // React Query sets isError on any failed fetch, including a refetch of a
    // query that already has data; isLoadingError is the narrower "never had
    // data" case this component actually wants to treat as a failure.
    mockSpend.mockReturnValue({
      ...spendResponse({ current_spend: 12.5, usage_spend: 12.5 }),
      isError: true,
      isLoadingError: false,
    });
    renderCard();

    expect(screen.getByText("Pay-as-you-go")).toBeInTheDocument();
    expect(screen.getAllByText("$12.50")).toHaveLength(2);
    expect(screen.queryByText("Couldn't load this.")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard while payment status is still loading", () => {
  it("assumes a card exists rather than flashing the no-card banner", () => {
    mockStatus.mockReturnValue({ data: undefined, isLoading: true });
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.queryByText("No payment method on file")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard when payment status has never loaded successfully", () => {
  it("assumes a card exists rather than flashing the no-card banner", () => {
    mockStatus.mockReturnValue({ data: undefined, isLoading: false, isError: true });
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.queryByText("No payment method on file")).not.toBeInTheDocument();
  });
});

describe("PayAsYouGoCard when payment status refetch fails after loading", () => {
  it("keeps reading the last known status instead of assuming a card exists", () => {
    mockStatus.mockReturnValue({ data: { has_payment_method: false }, isLoading: false, isError: true });
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.getByText("No payment method on file")).toBeInTheDocument();
  });
});

describe("PayAsYouGoCard when payment status permanently fails to load", () => {
  it("shows a retry notice instead of silently assuming a card exists", async () => {
    const refetchStatus = vi.fn();
    mockStatus.mockReturnValue({ data: undefined, isLoading: false, isLoadingError: true, refetch: refetchStatus });
    mockSpend.mockReturnValue(
      spendResponse({ current_spend: 0, usage_spend: 6.4, credit_remaining: 3.6, has_credit: true }),
    );
    renderCard();

    expect(screen.getByText("Couldn't check your payment method")).toBeInTheDocument();
    expect(screen.queryByText("No payment method on file")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetchStatus).toHaveBeenCalledTimes(1);
  });
});

describe("PayAsYouGoCard resume link without manage permission", () => {
  beforeEach(() => {
    mockRole.mockReturnValue("member");
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 60,
        usage_spend: 60,
        current_period_end: "2026-08-23T12:00:00Z",
        limit: { amount: 6000, in_alarm: true },
      }),
    );
  });

  it("disables the link even though a card is on file", async () => {
    renderCard();

    const resume = screen.getByRole("button", { name: "Increase limit to resume now" });
    expect(resume).toBeDisabled();

    await userEvent.click(resume);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("explains the permission, not the missing card, on hover", async () => {
    renderCard();

    const trigger = screen.getByRole("button", { name: "Increase limit to resume now" }).parentElement!;
    await userEvent.hover(trigger);

    await waitFor(() =>
      expect(trigger).toHaveAccessibleDescription("Only owners and admins can change limits."),
    );
  });
});

describe("PayAsYouGoCard on the account's own personal account", () => {
  // `role` is null on a personal account; canManageBilling(role) alone would deny its owner.
  it("enables the resume link for the owner regardless of the session's org role", () => {
    mockRole.mockReturnValue(null);
    mockPersonalAccount.mockReturnValue({ name: "acme" });
    mockSpend.mockReturnValue(
      spendResponse({
        current_spend: 60,
        usage_spend: 60,
        current_period_end: "2026-08-23T12:00:00Z",
        limit: { amount: 6000, in_alarm: true },
      }),
    );
    renderCard();

    expect(screen.getByRole("button", { name: "Increase limit to resume now" })).not.toBeDisabled();
  });
});

function futureISO(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString();
}
